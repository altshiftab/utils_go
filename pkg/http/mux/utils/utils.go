package utils

import (
	"context"
	"fmt"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type parsedRequestUrlContextType struct{}
type parsedRequestHeaderContextType struct{}
type parsedRequestBodyContextType struct{}
type parsedRequestAuthenticationContextType struct{}

var ParsedRequestUrlContextKey = parsedRequestUrlContextType{}
var ParsedRequestHeaderContextKey = parsedRequestHeaderContextType{}
var ParsedRequestBodyContextKey = parsedRequestBodyContextType{}
var ParsedRequestAuthenticationContextKey = parsedRequestAuthenticationContextType{}

func getParsed[T any](ctx context.Context, key any) (T, error) {
	value, err := utils.GetContextValue[T](ctx, key)
	if err != nil {
		return value, fmt.Errorf("get context value: %w", err)
	}

	return value, nil
}

func getNonZeroParsed[T comparable](ctx context.Context, key any) (T, error) {
	value, err := utils.GetNonZeroContextValue[T](ctx, key)
	if err != nil {
		return value, fmt.Errorf("get non zero context value: %w", err)
	}

	return value, nil
}

func GetServerNonZeroContextValue[T comparable](ctx context.Context, key any) (T, *response_error.ResponseError) {
	value, err := getNonZeroParsed[T](ctx, key)
	if err != nil {
		var zero T
		return zero, &response_error.ResponseError{
			ServerError: fmt.Errorf("get non zero context value: %w", err),
		}
	}

	return value, nil
}

func GetServerContextValue[T any](ctx context.Context, key any) (T, *response_error.ResponseError) {
	value, err := getParsed[T](ctx, key)
	if err != nil {
		var zero T
		return zero, &response_error.ResponseError{
			ServerError: fmt.Errorf("get context value: %w", err),
		}
	}

	return value, nil
}

func GetParsedRequestBody[T any](ctx context.Context) (T, error) {
	return getParsed[T](ctx, ParsedRequestBodyContextKey)
}

func GetServerParsedRequestBody[T any](ctx context.Context) (T, *response_error.ResponseError) {
	return GetServerContextValue[T](ctx, ParsedRequestBodyContextKey)
}

func GetNonZeroParsedRequestBody[T comparable](ctx context.Context) (T, error) {
	return getNonZeroParsed[T](ctx, ParsedRequestBodyContextKey)
}

func GetServerNonZeroParsedRequestBody[T comparable](ctx context.Context) (T, *response_error.ResponseError) {
	return GetServerNonZeroContextValue[T](ctx, ParsedRequestBodyContextKey)
}

func GetParsedRequestHeaders[T any](ctx context.Context) (T, error) {
	return getParsed[T](ctx, ParsedRequestHeaderContextKey)
}

func GetNonZeroParsedRequestHeaders[T comparable](ctx context.Context) (T, error) {
	return getNonZeroParsed[T](ctx, ParsedRequestHeaderContextKey)
}

func GetServerParsedRequestHeaders[T any](ctx context.Context) (T, *response_error.ResponseError) {
	return GetServerContextValue[T](ctx, ParsedRequestHeaderContextKey)
}

func GetServerNonZeroParsedRequestHeaders[T comparable](ctx context.Context) (T, *response_error.ResponseError) {
	return GetServerNonZeroContextValue[T](ctx, ParsedRequestHeaderContextKey)
}

func GetParsedRequestUrl[T any](ctx context.Context) (T, error) {
	return getParsed[T](ctx, ParsedRequestUrlContextKey)
}

func GetServerNonZeroParsedRequestUrl[T comparable](ctx context.Context) (T, *response_error.ResponseError) {
	return GetServerNonZeroContextValue[T](ctx, ParsedRequestUrlContextKey)
}

func GetNonZeroParsedRequestUrl[T comparable](ctx context.Context) (T, error) {
	return getNonZeroParsed[T](ctx, ParsedRequestUrlContextKey)
}

func GetParsedRequestAuthentication[T any](ctx context.Context) (T, error) {
	return getParsed[T](ctx, ParsedRequestAuthenticationContextKey)
}

func GetServerNonZeroParsedRequestAuthentication[T comparable](ctx context.Context) (T, *response_error.ResponseError) {
	return GetServerNonZeroContextValue[T](ctx, ParsedRequestAuthenticationContextKey)
}

func MakeStaticContentHeaders(contentType, cacheControl, etag, lastModified string) []*response.HeaderEntry {
	var entries []*response.HeaderEntry

	if contentType != "" {
		entries = append(entries, &response.HeaderEntry{Name: "Content-Type", Value: contentType})
	}
	if etag != "" {
		entries = append(entries, &response.HeaderEntry{Name: "ETag", Value: etag})
	}

	if lastModified != "" {
		entries = append(entries, &response.HeaderEntry{Name: "Last-Modified", Value: lastModified})
	}

	if cacheControl != "" {
		entries = append(entries, &response.HeaderEntry{Name: "Cache-Control", Value: cacheControl, Overwrite: true})
	}

	return entries
}
