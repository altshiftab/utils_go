package errors

import (
	"errors"
	"fmt"
)

var (
	ErrNoResponseWritten           = errors.New("no response was written")
	ErrNoResponseWriterFlusher     = errors.New("no response writer flusher")
	ErrTransferEncodingAlreadySet  = errors.New("transfer encoding already set")
	ErrUnusableMuxSpecification    = errors.New("unusable mux specification")
	ErrCouldNotObtainHttpContext   = errors.New("could not obtain http context")
	ErrContentEncodingToDataNotOk  = errors.New("content encoding to data not ok")
	ErrUnusableResponseError       = errors.New("unusable response error")
	ErrMultipleResponseErrorErrors = fmt.Errorf("%w: multiple response error errors", ErrUnusableMuxSpecification)
	ErrUnexpectedResponseErrorType = fmt.Errorf("%w: unexpected response error type", ErrUnusableResponseError)
	ErrUnexpectedContentEncoding   = errors.New("unexpected content encoding")
	ErrUnsupportedFileExtension    = errors.New("unsupported file extension")
	ErrMisdirectedRequest          = errors.New("misdirected request")
)
