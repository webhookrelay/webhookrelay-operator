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
	tests := []string{
		"relay-api.test/v1",
		"ftp://relay-api.test/v1",
		"https://key:secret@relay-api.test/v1",
		"https://relay-api.test/v1?token=secret",
		"https://relay-api.test/v1#fragment",
	}
	for _, endpoint := range tests {
		t.Run(endpoint, func(t *testing.T) {
			t.Setenv("WHR_API_ENDPOINT_URL", endpoint)

			_, err := Load()

			require.Error(t, err)
			assert.NotContains(t, err.Error(), "key:secret")
			assert.False(t, strings.Contains(err.Error(), "token=secret"))
		})
	}
}
