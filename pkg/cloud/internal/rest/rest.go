// Package rest provides shared request helpers for the cloud API wrapper
// packages: context checking, JSON fetching, response nil-checking, and
// nextPageToken pagination.
package rest

import (
	"context"
	"fmt"
	"slices"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	motmedelHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

// GetJson performs a GET request and returns the decoded, nil-checked response.
func GetJson[T any](ctx context.Context, urlString string, options []fetch_config.Option) (*T, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	_, value, err := motmedelHttpUtils.FetchJson[*T](ctx, urlString, options...)
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
	}

	if value == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("response value"))
	}

	return value, nil
}

// SendJson performs a request with the given method and JSON body and returns
// the decoded, nil-checked response.
func SendJson[T any, B any](
	ctx context.Context,
	method string,
	urlString string,
	body B,
	options []fetch_config.Option,
) (*T, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	options = append(slices.Clip(options), fetch_config.WithMethod(method))
	_, value, err := motmedelHttpUtils.FetchJsonWithBody[*T](ctx, urlString, body, options...)
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("fetch json with body: %w", err), urlString)
	}

	if value == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("response value"))
	}

	return value, nil
}

// Do performs a request with the given method, ignoring the response body.
func Do(ctx context.Context, method string, urlString string, options []fetch_config.Option) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context err: %w", err)
	}

	options = append(slices.Clip(options), fetch_config.WithMethod(method))
	if _, _, err := motmedelHttpUtils.Fetch(ctx, urlString, options...); err != nil {
		return motmedelErrors.New(fmt.Errorf("fetch: %w", err), urlString)
	}

	return nil
}

// DoWithBody performs a request with the given method and JSON body, ignoring
// the response body.
func DoWithBody[B any](
	ctx context.Context,
	method string,
	urlString string,
	body B,
	options []fetch_config.Option,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context err: %w", err)
	}

	options = append(slices.Clip(options), fetch_config.WithMethod(method))
	if _, _, err := motmedelHttpUtils.FetchJsonWithBody[*struct{}](ctx, urlString, body, options...); err != nil {
		return motmedelErrors.New(fmt.Errorf("fetch json with body: %w", err), urlString)
	}

	return nil
}

// ListPaginated repeatedly performs GET requests, following the page token
// returned by extract until it is empty, and concatenates the extracted items.
func ListPaginated[R any, T any](
	ctx context.Context,
	makeUrlString func(pageToken string) string,
	extract func(response *R) (items []T, nextPageToken string),
	options []fetch_config.Option,
) ([]T, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	var all []T
	pageToken := ""

	for {
		urlString := makeUrlString(pageToken)

		_, response, err := motmedelHttpUtils.FetchJson[*R](ctx, urlString, options...)
		if err != nil {
			return nil, motmedelErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
		}
		if response == nil {
			break
		}

		items, nextPageToken := extract(response)
		all = append(all, items...)

		if nextPageToken == "" {
			break
		}
		pageToken = nextPageToken
	}

	return all, nil
}
