package errors

import (
	"errors"
	"strconv"
)

var (
	ErrNon2xxStatusCode             = errors.New("non-2xx status code")
	ErrReattemptFailedError         = errors.New("reattempt failed")
	ErrBadIfModifiedSinceTimestamp  = errors.New("bad If-Modified-Since timestamp")
	ErrMissingHeader                = errors.New("missing header")
	ErrMultipleHeaderValues         = errors.New("multiple header values")
	ErrMissingQueryParameter        = errors.New("missing query parameter")
	ErrMultipleQueryParameterValues = errors.New("multiple query parameter values")
	ErrResponseBodyTooLarge         = errors.New("response body too large")
)

type Non2xxStatusCodeError struct {
	StatusCode int
}

func (non2xxStatusCodeError *Non2xxStatusCodeError) Is(target error) bool {
	return target == ErrNon2xxStatusCode
}

func (non2xxStatusCodeError *Non2xxStatusCodeError) Error() string {
	return ErrNon2xxStatusCode.Error()
}

func (non2xxStatusCodeError *Non2xxStatusCodeError) GetInput() any {
	return non2xxStatusCodeError.StatusCode
}

func (non2xxStatusCodeError *Non2xxStatusCodeError) GetCode() string {
	if non2xxStatusCodeError.StatusCode == 0 {
		return ""
	}
	return strconv.Itoa(non2xxStatusCodeError.StatusCode)
}

type ReattemptFailedError struct {
	Attempt int
	Cause   error
}

func (reattemptFailedError *ReattemptFailedError) Is(target error) bool {
	return target == ErrReattemptFailedError
}

func (reattemptFailedError *ReattemptFailedError) Error() string {
	return ErrReattemptFailedError.Error()
}

func (reattemptFailedError *ReattemptFailedError) GetCause() error {
	return reattemptFailedError.Cause
}

func (reattemptFailedError *ReattemptFailedError) Unwrap() error {
	return reattemptFailedError.Cause
}
