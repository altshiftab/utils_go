// Package response_checker decides, from what an attempt came back with,
// whether it is worth making again.
//
// The body is passed alongside the response because a status code is not always
// enough to tell a refusal that will stand from one that will not: Google's APIs
// refuse a caller who asks too often with 403 and a reason in the body, which
// reads exactly like a refusal for want of permission until the body is looked
// at. It is the same reason RetryAfterFunc is given the body.
package response_checker

import (
	"net/http"
)

type ResponseChecker interface {
	Check(*http.Response, []byte, error) bool
}

type ResponseCheckerFunction func(*http.Response, []byte, error) bool

func (f ResponseCheckerFunction) Check(response *http.Response, responseBody []byte, err error) bool {
	return f(response, responseBody, err)
}

func New(f func(*http.Response, []byte, error) bool) ResponseChecker {
	return ResponseCheckerFunction(f)
}
