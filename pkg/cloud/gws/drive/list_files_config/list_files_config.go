package list_files_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

// Corpora are the bodies of files a listing searches.
const (
	// CorporaUser is the files the user owns or has been given, which is Drive's
	// default.
	CorporaUser = "user"
	// CorporaAllDrives adds every shared drive the user is a member of.
	CorporaAllDrives = "allDrives"
)

type Config struct {
	// Query narrows what is listed, in Drive's query language
	// ("name contains 'invoice' and trashed = false"). Empty lists everything,
	// the trash included.
	Query string
	// PageSize is how many are asked for per page. Zero takes Drive's own
	// default, which is a hundred; the most it serves is a thousand.
	PageSize  int
	PageToken string
	// OrderBy sorts the listing ("modifiedTime desc", "name"). Drive refuses
	// it together with a fullText query.
	OrderBy string
	// Fields names what each page returns, as "nextPageToken, files(id, name)".
	// Empty takes Drive's default, which gives each file its id, name and MIME
	// type and nothing else.
	Fields  string
	Corpora string
	// IncludeItemsFromAllDrives lists what is in shared drives too; Drive
	// requires it with CorporaAllDrives, along with the client's
	// supportsAllDrives.
	IncludeItemsFromAllDrives bool
	FetchOptions              []fetch_config.Option
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithQuery(query string) Option {
	return func(config *Config) {
		config.Query = query
	}
}

func WithPageSize(pageSize int) Option {
	return func(config *Config) {
		config.PageSize = pageSize
	}
}

func WithPageToken(pageToken string) Option {
	return func(config *Config) {
		config.PageToken = pageToken
	}
}

func WithOrderBy(orderBy string) Option {
	return func(config *Config) {
		config.OrderBy = orderBy
	}
}

func WithFields(fields string) Option {
	return func(config *Config) {
		config.Fields = fields
	}
}

func WithCorpora(corpora string) Option {
	return func(config *Config) {
		config.Corpora = corpora
	}
}

func WithIncludeItemsFromAllDrives(includeItemsFromAllDrives bool) Option {
	return func(config *Config) {
		config.IncludeItemsFromAllDrives = includeItemsFromAllDrives
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
