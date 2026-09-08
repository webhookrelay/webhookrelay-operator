package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
)

const maxBodyBytes = 1 << 20

type recordedRequest struct {
	Method         string      `json:"method"`
	Path           string      `json:"path"`
	RawQuery       string      `json:"rawQuery"`
	Headers        http.Header `json:"headers"`
	Body           string      `json:"body"`
	Nonce          string      `json:"nonce"`
	OverrideHeader string      `json:"overrideHeader"`
	FunctionHeader string      `json:"functionHeader"`
}

type recorder struct {
	requests sync.Map
}

func (r *recorder) serveHTTP(w http.ResponseWriter, request *http.Request) {
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/healthz":
		w.WriteHeader(http.StatusNoContent)
		return
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/requests/"):
		nonce := strings.TrimPrefix(request.URL.Path, "/requests/")
		value, found := r.requests.Load(nonce)
		if !found {
			http.NotFound(w, request)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(value); err != nil {
			log.Printf("encode request: %v", err)
		}
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, request.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	nonce := request.Header.Get("X-WHR-E2E-Nonce")
	if nonce == "" {
		http.Error(w, "missing test nonce", http.StatusBadRequest)
		return
	}
	r.requests.Store(nonce, recordedRequest{
		Method:         request.Method,
		Path:           request.URL.Path,
		RawQuery:       request.URL.RawQuery,
		Headers:        request.Header.Clone(),
		Body:           string(body),
		Nonce:          nonce,
		OverrideHeader: request.Header.Get("X-WHR-E2E-Override"),
		FunctionHeader: request.Header.Get("X-WHR-E2E-Function"),
	})

	w.Header().Set("X-WHR-E2E-Receiver", "observed")
	w.WriteHeader(http.StatusCreated)
	_, _ = fmt.Fprintf(w, "receiver-response:%s", nonce)
}

func main() {
	recorder := &recorder{}
	server := &http.Server{
		Addr:    ":8080",
		Handler: http.HandlerFunc(recorder.serveHTTP),
	}
	log.Printf("recording receiver listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
