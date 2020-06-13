package config

type (
	// Config stores the configuration settings.
	Config struct {
		Image string `default:"webhookrelay/webhookrelayd:latest"`

		// Token allows setting up relay token key & secret on the operator itself
		// rather than using per CR key & secret
		Token struct {
			Key    string `envconfig:"WHR_TOKEN_KEY"`
			Secret string `envconfig:"WHR_TOKEN_SECRET"`
		}
	}
)
