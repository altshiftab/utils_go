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

type bodiesOmittedContextType struct{}

var bodiesOmittedContextKey = &bodiesOmittedContextType{}

// WithBodiesOmitted marks a record's context as one whose HTTP context is to be written without the
// bodies of its request and response: the summary of a request served, which says who asked for
// what and how it ended, and has no need of what was sent.
func WithBodiesOmitted(parent context.Context) context.Context {
	return context.WithValue(parent, bodiesOmittedContextKey, true)
}

// BodiesOmitted reports whether the context was marked by WithBodiesOmitted.
func BodiesOmitted(ctx context.Context) bool {
	if ctx == nil {
		return false
	}

	omitted, _ := ctx.Value(bodiesOmittedContextKey).(bool)

	return omitted
}
