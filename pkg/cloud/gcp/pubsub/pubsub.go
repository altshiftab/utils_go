package pubsub

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/pubsub_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/types/acknowledge_request"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/types/publish_request"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/types/publish_response"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/types/pull_request"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/types/pull_response"
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

// subscriptionUrl is the URL of a custom verb on a subscription. Only the
// per-request identifiers are path-segment escaped; the verb is appended
// literally, as in Publish.
func (c *Client) subscriptionUrl(project string, subscription string, verb string) string {
	u := *c.baseUrl
	u.RawPath = u.Path + "projects/" + url.PathEscape(project) +
		"/subscriptions/" + url.PathEscape(subscription) + ":" + verb
	u.Path += "projects/" + project + "/subscriptions/" + subscription + ":" + verb

	return u.String()
}

// Pull asks for whatever is waiting on a subscription. The runtime identity must
// have the pubsub.subscriber role on it.
//
// Nothing is acknowledged. That is the caller's to decide and it is the decision
// that matters: a message stays outstanding until acknowledged and is delivered
// again if the deadline passes, so acknowledging before the work is done trades
// duplicates for loss. Which way to trade depends on what the message carries. A
// signal that something should be refreshed can be acknowledged at once, because
// whoever wanted the refresh will ask again; a log line cannot, because nothing
// will ever ask for it again. Consumers of the second kind acknowledge after the
// message is somewhere durable, and rely on being able to recognise a duplicate.
//
// An empty response is the ordinary idle case, not a failure: the server waits a
// short while for messages and then answers with none.
func (c *Client) Pull(
	ctx context.Context,
	project string,
	subscription string,
	maxMessages int,
	options ...fetch_config.Option,
) (*pull_response.Response, error) {
	if project == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("project"))
	}
	if subscription == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("subscription"))
	}
	if maxMessages <= 0 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: max messages must be positive: %d", altshiftErrors.ErrValidationError, maxMessages),
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	urlString := c.subscriptionUrl(project, subscription, "pull")

	options = append(append(c.config.FetchOptions, options...), fetch_config.WithMethod(http.MethodPost))
	_, response, err := altshiftHttpUtils.FetchJsonWithBody[*pull_response.Response](
		ctx,
		urlString,
		&pull_request.Request{MaxMessages: maxMessages},
		options...,
	)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json with body: %w", err), urlString)
	}
	if response == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("pull response"))
	}

	return response, nil
}

// Acknowledge tells the server that the given messages have been dealt with, so
// that they are not delivered again. Acknowledging nothing is not a request.
func (c *Client) Acknowledge(
	ctx context.Context,
	project string,
	subscription string,
	ackIds []string,
	options ...fetch_config.Option,
) error {
	if project == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("project"))
	}
	if subscription == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("subscription"))
	}
	if len(ackIds) == 0 {
		return nil
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context err: %w", err)
	}

	urlString := c.subscriptionUrl(project, subscription, "acknowledge")

	// The response is an empty object; there is nothing to read and nothing to
	// decode into.
	options = append(
		append(c.config.FetchOptions, options...),
		fetch_config.WithMethod(http.MethodPost),
		fetch_config.WithSkipReadResponseBody(true),
	)
	if _, _, err := altshiftHttpUtils.FetchJsonWithBody[*struct{}](
		ctx,
		urlString,
		&acknowledge_request.Request{AckIds: ackIds},
		options...,
	); err != nil {
		return altshiftErrors.New(fmt.Errorf("fetch json with body: %w", err), urlString)
	}

	return nil
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
