package list_messages_config

import (
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

type Config struct {
	// FolderId lists one folder's messages: an id, or a well-known name such as "inbox". Empty
	// lists the whole mailbox, Deleted Items and Junk Email included.
	FolderId string
	// Search is a KQL query ("from:a@example.com budget"). Graph serves a search newest first and
	// takes neither Filter nor OrderBy beside it.
	Search string
	// Filter is an OData filter. Every property OrderBy names must come first in it, in the same
	// order, or Graph refuses the request as inefficient.
	Filter  string
	OrderBy []string
	// Select names the properties served; empty serves Graph's default set.
	Select []string
	// Expand is an OData expansion, such as "attachments($select=id,name,size)".
	Expand string
	// Top is how many are asked for per page; zero takes Graph's default of ten.
	Top int
	// BodyContentType asks for bodies as item_body.ContentTypeText or ContentTypeHtml; empty serves
	// them as they are kept.
	BodyContentType string
	// NextLink asks for the page after the one that returned it, in place of everything above. It
	// must be a link of the client's own Graph for the same mailbox, or it is refused.
	NextLink     string
	FetchOptions []fetch_config.Option
}

type Option func(*Config)

func New(options ...Option) *Config {
	config := &Config{}
	for _, option := range options {
		option(config)
	}

	return config
}

func WithFolderId(folderId string) Option {
	return func(config *Config) {
		config.FolderId = folderId
	}
}

func WithSearch(search string) Option {
	return func(config *Config) {
		config.Search = search
	}
}

func WithFilter(filter string) Option {
	return func(config *Config) {
		config.Filter = filter
	}
}

func WithOrderBy(orderBy ...string) Option {
	return func(config *Config) {
		config.OrderBy = append(config.OrderBy, orderBy...)
	}
}

func WithSelect(properties ...string) Option {
	return func(config *Config) {
		config.Select = append(config.Select, properties...)
	}
}

func WithExpand(expand string) Option {
	return func(config *Config) {
		config.Expand = expand
	}
}

func WithTop(top int) Option {
	return func(config *Config) {
		config.Top = top
	}
}

func WithBodyContentType(bodyContentType string) Option {
	return func(config *Config) {
		config.BodyContentType = bodyContentType
	}
}

func WithNextLink(nextLink string) Option {
	return func(config *Config) {
		config.NextLink = nextLink
	}
}

func WithFetchOptions(fetchOptions ...fetch_config.Option) Option {
	return func(config *Config) {
		config.FetchOptions = append(config.FetchOptions, fetchOptions...)
	}
}
