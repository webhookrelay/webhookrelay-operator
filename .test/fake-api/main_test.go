package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIRequiresAuthorizationWithoutReflectingIt(t *testing.T) {
	server := newAPIServer()
	request := httptest.NewRequest(http.MethodGet, "/v1/buckets", nil)
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.NotContains(t, response.Body.String(), "Authorization")
}

func TestAPICreatesAndReturnsState(t *testing.T) {
	server := newAPIServer()
	body := bytes.NewBufferString(`{"name":"example","description":"owned by test"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/buckets", body)
	request.Header.Set("Authorization", "Basic secret-value")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	stateRequest := httptest.NewRequest(http.MethodGet, "/v1/state", nil)
	stateResponse := httptest.NewRecorder()
	server.ServeHTTP(stateResponse, stateRequest)
	var state struct {
		Mutations map[string]int `json:"mutations"`
	}
	require.NoError(t, json.Unmarshal(stateResponse.Body.Bytes(), &state))
	assert.Equal(t, 1, state.Mutations["createBucket"])
	assert.NotContains(t, stateResponse.Body.String(), "secret-value")
}

func TestAPIProvidesDeterministicFailureFixture(t *testing.T) {
	server := newAPIServer()
	body := bytes.NewBufferString(`{"name":"force-error"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/buckets", body)
	request.Header.Set("Authorization", "Basic test")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	assert.Equal(t, http.StatusInternalServerError, response.Code)
}
