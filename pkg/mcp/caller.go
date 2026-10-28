package mcp

import (
	"context"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

// Caller is whom a tool call is for: the account whose data it reaches, and the client asking, as
// the request's access token names them.
type Caller struct {
	Account  string
	ClientId string
}

type callerKey struct{}

// WithCaller is the context of a call for the caller, which the endpoint sets before handing the
// request to the server, and a tool reads with CallerOf.
func WithCaller(ctx context.Context, caller *Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, caller)
}

// CallerOf is the caller the context carries, an error when it carries none with an account: a tool
// is never run for no one.
func CallerOf(ctx context.Context) (*Caller, error) {
	caller, ok := ctx.Value(callerKey{}).(*Caller)
	if !ok || caller == nil || caller.Account == "" {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("caller"))
	}
	return caller, nil
}
