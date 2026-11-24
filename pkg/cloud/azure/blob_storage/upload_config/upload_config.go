package upload_config

const (
	// DefaultBlockSize is what one Put Block carries.
	DefaultBlockSize = 8 << 20
	// MaxBlockSize is the largest block the service takes. Typed, since it is larger than an int
	// holds on 32-bit platforms.
	MaxBlockSize int64 = 4000 << 20
	// MaxBlocks is the most blocks one blob can be committed from.
	MaxBlocks = 50_000
)

type Config struct {
	// IfAbsent commits the blob only where no current version holds the name, and reports a
	// taken name as blob_storage.ErrBlobExists. In an account with versioning, the earlier
	// versions of a deleted blob do not hold its name.
	IfAbsent bool
	// BlockSize is how many bytes one block carries, and how many the upload holds in memory.
	BlockSize int
	// ContentType is the blob's Content-Type.
	ContentType string
	// Metadata is the blob's user metadata, set once at commit. Names must be valid C#
	// identifiers; values are sent as they are, so they must be header-safe ASCII.
	Metadata map[string]string
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{BlockSize: DefaultBlockSize}
	for _, option := range options {
		if option != nil {
			option(config)
		}
	}

	return config
}

func WithIfAbsent(ifAbsent bool) Option {
	return func(config *Config) {
		config.IfAbsent = ifAbsent
	}
}

func WithBlockSize(blockSize int) Option {
	return func(config *Config) {
		config.BlockSize = blockSize
	}
}

func WithContentType(contentType string) Option {
	return func(config *Config) {
		config.ContentType = contentType
	}
}

func WithMetadata(metadata map[string]string) Option {
	return func(config *Config) {
		config.Metadata = metadata
	}
}
