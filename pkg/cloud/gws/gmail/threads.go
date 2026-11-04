package gmail

import (
	"context"
	"net/http"
	"net/url"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/get_message_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/list_messages_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/page"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/thread"
)

type listThreadsResponse struct {
	Threads            []*thread.Thread `json:"threads"`
	NextPageToken      string           `json:"nextPageToken"`
	ResultSizeEstimate int              `json:"resultSizeEstimate"`
}

// ListThreadsPage retrieves one page of the threads matching the options,
// beginning at the options' page token.
//
// A thread listing takes the parameters a message listing does, which is why it
// is given the same options. What it returns carries each thread's id and
// snippet and no messages: GetThread reads one.
func (c *Client) ListThreadsPage(
	ctx context.Context,
	userId string,
	options ...list_messages_config.Option,
) (*page.Page[*thread.Thread], error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}

	listConfig := list_messages_config.New(options...)

	response, err := rest.GetJson[listThreadsResponse](
		ctx,
		c.threadsUrl(userId, "", "", listQuery(listConfig, listConfig.PageToken)),
		c.fetchOptions(listConfig.FetchOptions),
	)
	if err != nil {
		return nil, err
	}

	return &page.Page[*thread.Thread]{
		Items:              response.Threads,
		NextPageToken:      response.NextPageToken,
		ResultSizeEstimate: response.ResultSizeEstimate,
	}, nil
}

// GetThread retrieves a thread and its messages.
//
// It takes a message read's options, since a thread is read message by message
// in the format asked for; the raw format is the one a thread refuses.
func (c *Client) GetThread(
	ctx context.Context,
	userId string,
	threadId string,
	options ...get_message_config.Option,
) (*thread.Thread, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if threadId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("thread id"))
	}

	getMessageConfig := get_message_config.New(options...)

	query := url.Values{}
	if getMessageConfig.Format != "" {
		query.Set("format", string(getMessageConfig.Format))
	}
	for _, header := range getMessageConfig.MetadataHeaders {
		query.Add("metadataHeaders", header)
	}

	return rest.GetJson[thread.Thread](
		ctx,
		c.threadsUrl(userId, threadId, "", query),
		c.fetchOptions(getMessageConfig.FetchOptions),
	)
}

// TrashThread moves every message in a thread to the user's trash. Requires the
// gmail.modify scope (or wider).
func (c *Client) TrashThread(ctx context.Context, userId string, threadId string, options ...fetch_config.Option) (*thread.Thread, error) {
	return c.threadVerb(ctx, userId, threadId, "trash", options)
}

// UntrashThread takes every message in a thread out of the user's trash.
// Requires the gmail.modify scope (or wider).
func (c *Client) UntrashThread(ctx context.Context, userId string, threadId string, options ...fetch_config.Option) (*thread.Thread, error) {
	return c.threadVerb(ctx, userId, threadId, "untrash", options)
}

func (c *Client) threadVerb(
	ctx context.Context,
	userId string,
	threadId string,
	verb string,
	options []fetch_config.Option,
) (*thread.Thread, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if threadId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("thread id"))
	}

	return rest.SendJson[thread.Thread, any](
		ctx,
		http.MethodPost,
		c.threadsUrl(userId, threadId, verb, nil),
		nil,
		c.fetchOptions(options),
	)
}
