package config

type (
	// Config stores the configuration settings.
	Config struct {
		Image string `default:"webhookrelay/webhookrelayd-ubi8:latest"`

		// Relay allows setting up relay token key & secret on the operator itself
		// rather than using per CR key & secret
		Relay struct {
			Key    string `envconfig:"RELAY_KEY"`
			Secret string `envconfig:"RELAY_SECRET"`
		}

		HTTPPRoxy  string `envconfig:"HTTP_PROXY"`
		HTTPSPRoxy string `envconfig:"HTTPS_PROXY"`
	}
)
