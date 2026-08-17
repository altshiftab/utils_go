package context

import (
	"context"
	motmedelTlsTypes "github.com/altshiftab/utils_go/pkg/tls/types"
)

type tlsContextType struct{}

var TlsContextKey tlsContextType

func WithTlsContextValue(parent context.Context, tlsContext *motmedelTlsTypes.TlsContext) context.Context {
	return context.WithValue(parent, TlsContextKey, tlsContext)
}

func WithTlsContext(parent context.Context) context.Context {
	return WithTlsContextValue(parent, &motmedelTlsTypes.TlsContext{})
}
