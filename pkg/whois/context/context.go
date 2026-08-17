package context

import (
	"context"

	"github.com/altshiftab/utils_go/pkg/whois/types"
)

type contextType struct{}

var Key contextType

func WithContextValue(parent context.Context, whoisContext *types.WhoisContext) context.Context {
	return context.WithValue(parent, Key, whoisContext)
}

func WithWhoisContext(parent context.Context) context.Context {
	return WithContextValue(parent, &types.WhoisContext{})
}
