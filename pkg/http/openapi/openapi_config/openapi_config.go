package openapi_config

import (
	openapiTypes "github.com/altshiftab/utils_go/pkg/http/openapi/types"
)

// Version32 is the specification version the generator writes by default. It is the first with a
// field for the HTTP QUERY method, and is otherwise a document 3.1 readers accept.
const Version32 = "3.2.0"

// Version31 is the earlier version, for a reader that does not know 3.2 yet. A document targeting
// it cannot hold a QUERY operation.
const Version31 = "3.1.1"

// DefaultPath is where a served document is offered when nothing says otherwise.
const DefaultPath = "/openapi.json"

// ContentType is what a served document is written as. The specification registers it; a reader
// that knows only application/json still reads the body, the suffix saying it is JSON.
const ContentType = "application/openapi+json"

type Config struct {
	// Version is the specification version the document declares and is written against.
	Version string

	// Info identifies the API. Title and Version are required of a document and cannot be derived
	// from any endpoint.
	Info *openapiTypes.Info

	// Servers are the base URLs the API is served at.
	Servers []*openapiTypes.Server

	// SecuritySchemes are the ways a request may prove who is making it, by the name an operation
	// refers to them by. Every scheme declared here is offered as an alternative on every endpoint
	// that is not public.
	SecuritySchemes map[string]*openapiTypes.SecurityScheme

	// Tags describe the groups operations name through Hint.Tags.
	Tags []*openapiTypes.Tag

	// Path is where a served document is offered.
	Path string

	// Public says whether the served document may be read without a session.
	//
	// It is false by default, unlike most things that are served. The document names every
	// operation, its inputs and its outputs, including those that exist for one customer's
	// administrators; handing that to an unauthenticated reader is a decision to be made on
	// purpose, not by leaving a field alone. A document meant for a public API says so here.
	Public bool

	// CacheControl is what a served document is cached under. Empty means a value derived from
	// Public: a gated document is private and revalidated, a public one may be held briefly.
	CacheControl string
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{Version: Version32, Path: DefaultPath}
	for _, option := range options {
		option(config)
	}

	return config
}

// WithVersion sets the specification version. Version32 and Version31 are the ones understood.
func WithVersion(version string) Option {
	return func(config *Config) {
		config.Version = version
	}
}

// WithInfo sets what identifies the API. Title and version are required of a document.
func WithInfo(title string, version string) Option {
	return func(config *Config) {
		if config.Info == nil {
			config.Info = &openapiTypes.Info{}
		}
		config.Info.Title = title
		config.Info.Version = version
	}
}

// WithInfoDescription sets the prose shown above the operations, in CommonMark.
func WithInfoDescription(description string) Option {
	return func(config *Config) {
		if config.Info == nil {
			config.Info = &openapiTypes.Info{}
		}
		config.Info.Description = description
	}
}

// WithServer adds a base URL the API is served at. It may be given more than once.
func WithServer(url string, description string) Option {
	return func(config *Config) {
		config.Servers = append(config.Servers, &openapiTypes.Server{Url: url, Description: description})
	}
}

// WithSecurityScheme declares a way a request may prove who is making it, under the name operations
// refer to it by. It may be given more than once, each scheme being an alternative to the others.
func WithSecurityScheme(name string, scheme *openapiTypes.SecurityScheme) Option {
	return func(config *Config) {
		if config.SecuritySchemes == nil {
			config.SecuritySchemes = make(map[string]*openapiTypes.SecurityScheme)
		}
		config.SecuritySchemes[name] = scheme
	}
}

// WithTag describes a group operations name through Hint.Tags.
func WithTag(name string, description string) Option {
	return func(config *Config) {
		config.Tags = append(config.Tags, &openapiTypes.Tag{Name: name, Description: description})
	}
}

// WithPath sets where a served document is offered.
func WithPath(path string) Option {
	return func(config *Config) {
		config.Path = path
	}
}

// WithPublic says whether a served document may be read without a session. See Config.Public for
// why it is false by default.
func WithPublic(public bool) Option {
	return func(config *Config) {
		config.Public = public
	}
}

// WithCacheControl sets what a served document is cached under, in place of the value derived from
// whether it is public.
func WithCacheControl(cacheControl string) Option {
	return func(config *Config) {
		config.CacheControl = cacheControl
	}
}
