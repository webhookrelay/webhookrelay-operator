package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecorderStoresRequestAndReturnsDynamicResponse(t *testing.T) {
	recorder := &recorder{}
	nonce := "test-nonce"
	request := httptest.NewRequest(http.MethodPost, "/hooks/base?ignored=true", strings.NewReader(`{"nonce":"test-nonce"}`))
	request.Header.Set("X-WHR-E2E-Nonce", nonce)
	request.Header.Set("X-WHR-E2E-Override", "baseline")
	response := httptest.NewRecorder()

	recorder.serveHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusCreated)
	}
	if got := response.Header().Get("X-WHR-E2E-Receiver"); got != "observed" {
		t.Fatalf("receiver header = %q, want observed", got)
	}
	if got := response.Body.String(); got != "receiver-response:"+nonce {
		t.Fatalf("body = %q, want receiver response", got)
	}

	lookup := httptest.NewRequest(http.MethodGet, "/requests/"+nonce, nil)
	lookupResponse := httptest.NewRecorder()
	recorder.serveHTTP(lookupResponse, lookup)
	if lookupResponse.Code != http.StatusOK {
		t.Fatalf("lookup status = %d, want %d", lookupResponse.Code, http.StatusOK)
	}
	var recorded recordedRequest
	if err := json.NewDecoder(lookupResponse.Body).Decode(&recorded); err != nil {
		t.Fatalf("decode recorded request: %v", err)
	}
	if recorded.Method != http.MethodPost || recorded.Path != "/hooks/base" || recorded.Nonce != nonce {
		t.Fatalf("unexpected recorded request: %+v", recorded)
	}
	if recorded.OverrideHeader != "baseline" {
		t.Fatalf("override header = %q, want baseline", recorded.OverrideHeader)
	}
}

func TestRecorderRejectsRequestWithoutNonce(t *testing.T) {
	recorder := &recorder{}
	request := httptest.NewRequest(http.MethodPost, "/hooks/base", strings.NewReader("{}"))
	response := httptest.NewRecorder()

	recorder.serveHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
