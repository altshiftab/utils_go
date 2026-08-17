package gemini

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"

	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/gemini_config"
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/generate_content_request"
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/generate_content_response"
)

const Domain = "generativelanguage.googleapis.com"

const apiKeyHeaderName = "x-goog-api-key" //nolint:gosec // G101: HTTP header name, not a credential

var defaultBaseUrl = &url.URL{
	Scheme: "https",
	Host:   Domain,
}

type Client struct {
	baseUrl *url.URL
	config  *gemini_config.Config
}

func NewClient(options ...gemini_config.Option) *Client {
	config := gemini_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = defaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/v1beta/models/"
	return &Client{baseUrl: &u, config: config}
}

func (c *Client) generateContentUrl(model string) string {
	u := *c.baseUrl
	u.Path += url.PathEscape(model) + ":generateContent"
	return u.String()
}

// apiKeyFetchOption merges the API key header into the fetch configuration without
// replacing headers set by other options.
func (c *Client) apiKeyFetchOption() fetch_config.Option {
	apiKey := c.config.ApiKey
	if apiKey == "" {
		return nil
	}

	return func(fetchConfig *fetch_config.Config) {
		if fetchConfig.Headers == nil {
			fetchConfig.Headers = make(map[string]string)
		}
		fetchConfig.Headers[apiKeyHeaderName] = apiKey
	}
}

// GenerateContent generates a model response for the given request.
// https://ai.google.dev/api/generate-content
func (c *Client) GenerateContent(
	ctx context.Context,
	model string,
	request *generate_content_request.GenerateContentRequest,
	options ...fetch_config.Option,
) (*generate_content_response.GenerateContentResponse, error) {
	if model == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("model"))
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	if request == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("request"))
	}

	urlString := c.generateContentUrl(model)
	options = append(
		append(c.config.FetchOptions, options...),
		fetch_config.WithMethod(http.MethodPost),
		c.apiKeyFetchOption(),
	)
	_, response, err := altshiftHttpUtils.FetchJsonWithBody[*generate_content_response.GenerateContentResponse](
		ctx,
		urlString,
		request,
		options...,
	)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json with body: %w", err), urlString)
	}

	return response, nil
}
