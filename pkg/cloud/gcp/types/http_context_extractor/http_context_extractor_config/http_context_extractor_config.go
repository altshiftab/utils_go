package http_context_extractor_config

type Config struct {
	ProjectId string
	MaskUrl   func(string) string
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{}
	for _, option := range options {
		if option != nil {
			option(config)
		}
	}

	return config
}

func WithProjectId(projectId string) Option {
	return func(config *Config) {
		config.ProjectId = projectId
	}
}

// WithMaskUrl gives the extractor the masking to apply to the URLs it writes.
//
// Cloud Logging reads the referrer from a field of its own, which no other masking reaches: a
// service that declares a query parameter secret and logs through this extractor would otherwise
// publish it under httpRequest.referer on every request made from the page that carries it.
func WithMaskUrl(maskUrl func(string) string) Option {
	return func(config *Config) {
		config.MaskUrl = maskUrl
	}
}
