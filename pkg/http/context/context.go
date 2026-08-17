package context

import (
	"context"

	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

type requestIdContextType struct{}

var RequestIdContextKey = &requestIdContextType{}

type httpContextContextType struct{}

var HttpContextContextKey httpContextContextType

func WithHttpContextValue(parent context.Context, httpContext *altshiftHttpTypes.HttpContext) context.Context {
	return context.WithValue(parent, HttpContextContextKey, httpContext)
}

func WithHttpContext(parent context.Context) context.Context {
	return WithHttpContextValue(parent, &altshiftHttpTypes.HttpContext{})
}
