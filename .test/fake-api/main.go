package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	webhookrelay "github.com/webhookrelay/webhookrelay-go"
)

type apiServer struct {
	mu        sync.Mutex
	nextID    int
	buckets   map[string]*webhookrelay.Bucket
	mutations map[string]int
}

func newAPIServer() *apiServer {
	return &apiServer{
		nextID:    1,
		buckets:   make(map[string]*webhookrelay.Bucket),
		mutations: make(map[string]int),
	}
}

func (s *apiServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.URL.Path == "/v1/state" {
		s.writeState(w)
		return
	}
	if r.Header.Get("Authorization") == "" {
		http.Error(w, "authorization required", http.StatusUnauthorized)
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "v1" || parts[1] != "buckets" {
		http.NotFound(w, r)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.routeBuckets(w, r, parts[2:])
}

func (s *apiServer) routeBuckets(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 0 {
		s.handleBucketCollection(w, r)
		return
	}
	bucket := s.buckets[parts[0]]
	if bucket == nil {
		http.Error(w, "bucket not found", http.StatusNotFound)
		return
	}
	if len(parts) == 1 {
		s.handleBucket(w, r, bucket)
		return
	}
	if len(parts) >= 2 && parts[1] == "inputs" {
		s.handleInputs(w, r, bucket, parts[2:])
		return
	}
	if len(parts) >= 2 && parts[1] == "outputs" {
		s.handleOutputs(w, r, bucket, parts[2:])
		return
	}
	http.NotFound(w, r)
}

func (s *apiServer) handleBucketCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		buckets := make([]*webhookrelay.Bucket, 0, len(s.buckets))
		for _, bucket := range s.buckets {
			buckets = append(buckets, bucket)
		}
		s.writeJSON(w, buckets)
	case http.MethodPost:
		var options webhookrelay.BucketCreateOptions
		if !s.decodeJSON(w, r, &options) {
			return
		}
		if strings.Contains(options.Name, "force-error") {
			http.Error(w, "fixture rejected bucket", http.StatusInternalServerError)
			return
		}
		now := time.Now().UTC()
		bucket := &webhookrelay.Bucket{
			ID:          s.id(),
			Name:        options.Name,
			Description: options.Description,
			CreatedAt:   now,
			UpdatedAt:   now,
			Inputs:      []*webhookrelay.Input{},
			Outputs:     []*webhookrelay.Output{},
		}
		s.buckets[bucket.ID] = bucket
		s.mutations["createBucket"]++
		s.writeJSON(w, bucket)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *apiServer) handleBucket(w http.ResponseWriter, r *http.Request, bucket *webhookrelay.Bucket) {
	switch r.Method {
	case http.MethodGet:
		s.writeJSON(w, bucket)
	case http.MethodPut:
		var updated webhookrelay.Bucket
		if !s.decodeJSON(w, r, &updated) {
			return
		}
		updated.ID = bucket.ID
		updated.CreatedAt = bucket.CreatedAt
		updated.UpdatedAt = time.Now().UTC()
		updated.Inputs = bucket.Inputs
		updated.Outputs = bucket.Outputs
		s.buckets[bucket.ID] = &updated
		s.mutations["updateBucket"]++
		s.writeJSON(w, &updated)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *apiServer) handleInputs(w http.ResponseWriter, r *http.Request, bucket *webhookrelay.Bucket, parts []string) {
	switch r.Method {
	case http.MethodPost:
		var input webhookrelay.Input
		if !s.decodeJSON(w, r, &input) {
			return
		}
		input.ID = s.id()
		input.BucketID = bucket.ID
		input.CreatedAt = time.Now().UTC()
		input.UpdatedAt = input.CreatedAt
		if input.CustomDomain == "" {
			input.CustomDomain = "fake.invalid"
		}
		bucket.Inputs = append(bucket.Inputs, &input)
		s.mutations["createInput"]++
		s.writeJSON(w, &input)
	case http.MethodPut:
		if len(parts) != 1 {
			http.NotFound(w, r)
			return
		}
		var input webhookrelay.Input
		if !s.decodeJSON(w, r, &input) {
			return
		}
		for i := range bucket.Inputs {
			if bucket.Inputs[i].ID == parts[0] {
				input.ID = parts[0]
				input.BucketID = bucket.ID
				input.CreatedAt = bucket.Inputs[i].CreatedAt
				input.UpdatedAt = time.Now().UTC()
				bucket.Inputs[i] = &input
				s.mutations["updateInput"]++
				s.writeJSON(w, &input)
				return
			}
		}
		http.Error(w, "input not found", http.StatusNotFound)
	case http.MethodDelete:
		if len(parts) != 1 {
			http.NotFound(w, r)
			return
		}
		for i := range bucket.Inputs {
			if bucket.Inputs[i].ID == parts[0] {
				bucket.Inputs = append(bucket.Inputs[:i], bucket.Inputs[i+1:]...)
				s.mutations["deleteInput"]++
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		http.Error(w, "input not found", http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *apiServer) handleOutputs(w http.ResponseWriter, r *http.Request, bucket *webhookrelay.Bucket, parts []string) {
	switch r.Method {
	case http.MethodPost:
		var output webhookrelay.Output
		if !s.decodeJSON(w, r, &output) {
			return
		}
		output.ID = s.id()
		output.BucketID = bucket.ID
		output.CreatedAt = time.Now().UTC()
		output.UpdatedAt = output.CreatedAt
		bucket.Outputs = append(bucket.Outputs, &output)
		s.mutations["createOutput"]++
		s.writeJSON(w, &output)
	case http.MethodPut:
		if len(parts) != 1 {
			http.NotFound(w, r)
			return
		}
		var output webhookrelay.Output
		if !s.decodeJSON(w, r, &output) {
			return
		}
		for i := range bucket.Outputs {
			if bucket.Outputs[i].ID == parts[0] {
				output.ID = parts[0]
				output.BucketID = bucket.ID
				output.CreatedAt = bucket.Outputs[i].CreatedAt
				output.UpdatedAt = time.Now().UTC()
				bucket.Outputs[i] = &output
				s.mutations["updateOutput"]++
				s.writeJSON(w, &output)
				return
			}
		}
		http.Error(w, "output not found", http.StatusNotFound)
	case http.MethodDelete:
		if len(parts) != 1 {
			http.NotFound(w, r)
			return
		}
		for i := range bucket.Outputs {
			if bucket.Outputs[i].ID == parts[0] {
				bucket.Outputs = append(bucket.Outputs[:i], bucket.Outputs[i+1:]...)
				s.mutations["deleteOutput"]++
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		http.Error(w, "output not found", http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *apiServer) writeState(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeJSON(w, struct {
		Buckets   map[string]*webhookrelay.Bucket `json:"buckets"`
		Mutations map[string]int                  `json:"mutations"`
	}{s.buckets, s.mutations})
}

func (s *apiServer) id() string {
	id := fmt.Sprintf("00000000-0000-4000-8000-%012x", s.nextID)
	s.nextID++
	return id
}

func (s *apiServer) decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return false
	}
	return true
}

func (s *apiServer) writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
	}
}

func main() {
	server := &http.Server{
		Addr:              ":8080",
		Handler:           newAPIServer(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("fake Relay API listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
