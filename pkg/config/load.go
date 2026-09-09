package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/kelseyhightower/envconfig"
)

// Load loads the configuration from the environment.
func Load() (Config, error) {
	config := Config{}
	if err := envconfig.Process("WHR", &config); err != nil {
		return Config{}, err
	}
	endpoint, err := normalizeAPIEndpointURL(config.APIEndpointURL)
	if err != nil {
		return Config{}, err
	}
	config.APIEndpointURL = endpoint
	return config, nil
}

func normalizeAPIEndpointURL(value string) (string, error) {
	endpoint, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid Relay API endpoint URL: %w", err)
	}
	if (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return "", fmt.Errorf("invalid Relay API endpoint URL: must be an absolute HTTP(S) URL")
	}
	if endpoint.User != nil {
		return "", fmt.Errorf("invalid Relay API endpoint URL: user information is not allowed")
	}
	if endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return "", fmt.Errorf("invalid Relay API endpoint URL: query parameters and fragments are not allowed")
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/")
	return endpoint.String(), nil
}

// MustLoad loads the configuration from the environment
// and panics if an error is encountered.
func MustLoad() Config {
	config, err := Load()
	if err != nil {
		panic(err)
	}
	return config
}
