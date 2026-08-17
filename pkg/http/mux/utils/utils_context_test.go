package utils

import (
	"context"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
)

func TestGetServerContextValue(t *testing.T) {
	t.Parallel()

	t.Run("present value", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(t.Context(), ParsedRequestBodyContextKey, "value")
		value, responseError := GetServerContextValue[string](ctx, ParsedRequestBodyContextKey)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if value != "value" {
			t.Fatalf("got %q, want value", value)
		}
	})

	t.Run("missing value is a server error", func(t *testing.T) {
		t.Parallel()
		_, responseError := GetServerContextValue[string](t.Context(), ParsedRequestBodyContextKey)
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})
}

func TestGetServerNonZeroContextValue(t *testing.T) {
	t.Parallel()

	t.Run("non-zero value", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(t.Context(), ParsedRequestBodyContextKey, "value")
		value, responseError := GetServerNonZeroContextValue[string](ctx, ParsedRequestBodyContextKey)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if value != "value" {
			t.Fatalf("got %q, want value", value)
		}
	})

	t.Run("zero value is a server error", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(t.Context(), ParsedRequestBodyContextKey, "")
		_, responseError := GetServerNonZeroContextValue[string](ctx, ParsedRequestBodyContextKey)
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("missing value is a server error", func(t *testing.T) {
		t.Parallel()
		_, responseError := GetServerNonZeroContextValue[string](t.Context(), ParsedRequestBodyContextKey)
		if responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})
}

func TestParsedRequestGetters(t *testing.T) {
	t.Parallel()

	bodyCtx := context.WithValue(t.Context(), ParsedRequestBodyContextKey, "body")
	headerCtx := context.WithValue(t.Context(), ParsedRequestHeaderContextKey, "header")
	urlCtx := context.WithValue(t.Context(), ParsedRequestUrlContextKey, "url")
	authCtx := context.WithValue(t.Context(), ParsedRequestAuthenticationContextKey, "auth")

	errGetters := []struct {
		name string
		want string
		get  func() (string, error)
	}{
		{"GetParsedRequestBody", "body", func() (string, error) { return GetParsedRequestBody[string](bodyCtx) }},
		{"GetNonZeroParsedRequestBody", "body", func() (string, error) { return GetNonZeroParsedRequestBody[string](bodyCtx) }},
		{"GetParsedRequestHeaders", "header", func() (string, error) { return GetParsedRequestHeaders[string](headerCtx) }},
		{"GetNonZeroParsedRequestHeaders", "header", func() (string, error) { return GetNonZeroParsedRequestHeaders[string](headerCtx) }},
		{"GetParsedRequestUrl", "url", func() (string, error) { return GetParsedRequestUrl[string](urlCtx) }},
		{"GetNonZeroParsedRequestUrl", "url", func() (string, error) { return GetNonZeroParsedRequestUrl[string](urlCtx) }},
		{"GetParsedRequestAuthentication", "auth", func() (string, error) { return GetParsedRequestAuthentication[string](authCtx) }},
	}
	for _, getter := range errGetters {
		value, err := getter.get()
		if err != nil {
			t.Errorf("%s: unexpected error: %v", getter.name, err)
		}
		if value != getter.want {
			t.Errorf("%s: got %q, want %q", getter.name, value, getter.want)
		}
	}

	serverGetters := []struct {
		name string
		want string
		get  func() (string, *response_error.ResponseError)
	}{
		{"GetServerParsedRequestBody", "body", func() (string, *response_error.ResponseError) { return GetServerParsedRequestBody[string](bodyCtx) }},
		{"GetServerNonZeroParsedRequestBody", "body", func() (string, *response_error.ResponseError) {
			return GetServerNonZeroParsedRequestBody[string](bodyCtx)
		}},
		{"GetServerParsedRequestHeaders", "header", func() (string, *response_error.ResponseError) {
			return GetServerParsedRequestHeaders[string](headerCtx)
		}},
		{"GetServerNonZeroParsedRequestHeaders", "header", func() (string, *response_error.ResponseError) {
			return GetServerNonZeroParsedRequestHeaders[string](headerCtx)
		}},
		{"GetServerNonZeroParsedRequestUrl", "url", func() (string, *response_error.ResponseError) {
			return GetServerNonZeroParsedRequestUrl[string](urlCtx)
		}},
		{"GetServerNonZeroParsedRequestAuthentication", "auth", func() (string, *response_error.ResponseError) {
			return GetServerNonZeroParsedRequestAuthentication[string](authCtx)
		}},
	}
	for _, getter := range serverGetters {
		value, responseError := getter.get()
		if responseError != nil {
			t.Errorf("%s: unexpected error: %#v", getter.name, responseError)
		}
		if value != getter.want {
			t.Errorf("%s: got %q, want %q", getter.name, value, getter.want)
		}
	}
}
