package list_blobs_config

type Config struct {
	// Prefix lists only the blobs whose names start with it.
	Prefix string
	// IncludeVersions lists every version of each blob, the current one marked, rather than the
	// current versions alone. It needs versioning on the account.
	IncludeVersions bool
	// IncludeMetadata lists each blob's user metadata.
	IncludeMetadata bool
	// Marker continues a listing from the NextMarker of the page before.
	Marker string
	// MaxResults bounds a page; zero leaves it to the service, which returns at most 5000.
	MaxResults int
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

func WithPrefix(prefix string) Option {
	return func(config *Config) {
		config.Prefix = prefix
	}
}

func WithIncludeVersions(includeVersions bool) Option {
	return func(config *Config) {
		config.IncludeVersions = includeVersions
	}
}

func WithIncludeMetadata(includeMetadata bool) Option {
	return func(config *Config) {
		config.IncludeMetadata = includeMetadata
	}
}

func WithMarker(marker string) Option {
	return func(config *Config) {
		config.Marker = marker
	}
}

func WithMaxResults(maxResults int) Option {
	return func(config *Config) {
		config.MaxResults = maxResults
	}
}
