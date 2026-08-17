package pubsub

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/pubsub_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/types/publish_request"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/types/publish_response"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

const Domain = "pubsub.googleapis.com"

var defaultBaseUrl = &url.URL{
	Scheme: "https",
	Host:   Domain,
}

type Client struct {
	baseUrl *url.URL
	config  *pubsub_config.Config
}

func NewClient(options ...pubsub_config.Option) *Client {
	config := pubsub_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = defaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/v1/"

	return &Client{baseUrl: &u, config: config}
}

// Publish publishes the messages in the request to the given topic and returns the
// server-assigned message ids. The runtime identity must have the pubsub.publisher
// role on the topic.
func (c *Client) Publish(
	ctx context.Context,
	project string,
	topic string,
	request *publish_request.Request,
	options ...fetch_config.Option,
) (*publish_response.Response, error) {
	if project == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("project"))
	}
	if topic == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("topic"))
	}
	if request == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("publish request"))
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	// The ":publish" custom verb is appended literally; only the per-request
	// project and topic identifiers are path-segment escaped.
	u := *c.baseUrl
	u.RawPath = u.Path + "projects/" + url.PathEscape(project) + "/topics/" + url.PathEscape(topic) + ":publish"
	u.Path += "projects/" + project + "/topics/" + topic + ":publish"
	urlString := u.String()

	options = append(append(c.config.FetchOptions, options...), fetch_config.WithMethod(http.MethodPost))
	_, response, err := altshiftHttpUtils.FetchJsonWithBody[*publish_response.Response](ctx, urlString, request, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json with body: %w", err), urlString)
	}
	if response == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("publish response"))
	}

	if response == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("response"))
	}

	return response, nil
}
