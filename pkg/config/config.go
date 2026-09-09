package config

type (
	// Config stores the configuration settings.
	Config struct {
		Image string `default:"webhookrelay/webhookrelayd-ubi8:1.37.0"`
		// APIEndpointURL is an operator-admin-controlled Webhook Relay API base
		// URL. It is intentionally not configurable from a custom resource.
		APIEndpointURL string `envconfig:"API_ENDPOINT_URL" default:"https://my.webhookrelay.com/v1"`

		// Relay allows setting up relay token key & secret on the operator itself
		// rather than using per CR key & secret
		Relay struct {
			Key    string `envconfig:"RELAY_KEY"`
			Secret string `envconfig:"RELAY_SECRET"`
		}
		// HTTPS proxy variable.
		// Note: not using standard HTTPS_PROXY so Kubernetes clients do not use it.
		HTTPSPRoxy string `envconfig:"CLIENT_HTTPS_PROXY"`
	}
)
