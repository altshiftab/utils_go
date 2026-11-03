package errors

import (
	"errors"
	"fmt"
	"strconv"
)

var (
	ErrRetrieveToken = errors.New("retrieve token")
)

type RetrieveError struct {
	StatusCode       int
	Body             []byte
	ErrorCode        string
	ErrorDescription string
	ErrorURI         string
}

func (e *RetrieveError) Is(target error) bool {
	return target == ErrRetrieveToken
}

func (e *RetrieveError) Error() string {
	if e.ErrorCode != "" {
		s := fmt.Sprintf("%q", e.ErrorCode)
		if e.ErrorDescription != "" {
			s += fmt.Sprintf(" %q", e.ErrorDescription)
		}
		if e.ErrorURI != "" {
			s += fmt.Sprintf(" %q", e.ErrorURI)
		}
		return s
	}

	return strconv.Itoa(e.StatusCode)
}
