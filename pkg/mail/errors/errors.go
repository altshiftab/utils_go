package errors

import "errors"

var ErrBadFromAddress = errors.New("bad from address")

// ErrHeaderInjection is a header name or value that would end its line, and so write headers, or a body, no one
// asked for.
var ErrHeaderInjection = errors.New("header injection")
