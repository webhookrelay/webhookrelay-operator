package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaultsAPIEndpoint(t *testing.T) {
	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "https://my.webhookrelay.com/v1", cfg.APIEndpointURL)
}

func TestLoadNormalizesAPIEndpoint(t *testing.T) {
	t.Setenv("WHR_API_ENDPOINT_URL", "http://relay-api.test:8080/v1///")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "http://relay-api.test:8080/v1", cfg.APIEndpointURL)
}

func TestLoadRejectsUnsafeAPIEndpoints(t *testing.T) {
	credentialPair := strings.Join([]string{"key", "secret"}, ":")
	tests := []struct {
		name     string
		endpoint string
	}{
		{name: "relative", endpoint: "relay-api.test/v1"},
		{name: "unsupported scheme", endpoint: "ftp://relay-api.test/v1"},
		{name: "user information", endpoint: "https://" + credentialPair + "@relay-api.test/v1"},
		{name: "malformed user information", endpoint: "https://" + credentialPair + "@%zz/v1"},
		{name: "query", endpoint: "https://relay-api.test/v1?token=secret"},
		{name: "fragment", endpoint: "https://relay-api.test/v1#fragment"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("WHR_API_ENDPOINT_URL", test.endpoint)

			_, err := Load()

			require.Error(t, err)
			assert.NotContains(t, err.Error(), credentialPair)
			assert.False(t, strings.Contains(err.Error(), "token=secret"))
		})
	}
}
