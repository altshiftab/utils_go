package gmail

import (
	"context"
	"net/http"
	"net/url"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/list_messages_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message/message_part/body"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/page"
)

// ListMessagesPage retrieves one page of the messages matching the options,
// beginning at the options' page token.
//
// It is ListMessages for a caller that shows what it finds rather than taking
// stock of it: a search of a large mailbox matches more than anyone reads, and
// following the paging to the end would fetch all of it to show the first few.
func (c *Client) ListMessagesPage(
	ctx context.Context,
	userId string,
	options ...list_messages_config.Option,
) (*page.Page[*message.Message], error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}

	listMessagesConfig := list_messages_config.New(options...)

	response, err := rest.GetJson[listMessagesResponse](
		ctx,
		c.messagesUrl(userId, "", "", listQuery(listMessagesConfig, listMessagesConfig.PageToken)),
		c.fetchOptions(listMessagesConfig.FetchOptions),
	)
	if err != nil {
		return nil, err
	}

	return &page.Page[*message.Message]{
		Items:              response.Messages,
		NextPageToken:      response.NextPageToken,
		ResultSizeEstimate: response.ResultSizeEstimate,
	}, nil
}

// Untrash takes the given message out of the user's trash. Requires the
// gmail.modify scope (or wider).
func (c *Client) Untrash(ctx context.Context, userId string, messageId string, options ...fetch_config.Option) (*message.Message, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if messageId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("message id"))
	}

	return rest.SendJson[message.Message, any](
		ctx,
		http.MethodPost,
		c.messagesUrl(userId, messageId, "untrash", nil),
		nil,
		c.fetchOptions(options),
	)
}

// GetAttachment retrieves the content of one of a message's attachments.
//
// A message read in full names its attachments by an id in each part's body and
// leaves their content out, which is fetched here. The content comes back in
// Data, base64url-encoded as every body in the API is. The id is not stable: two
// reads of the same message may name the same attachment by different ids, so it
// is to be used from the read that gave it, not stored.
func (c *Client) GetAttachment(
	ctx context.Context,
	userId string,
	messageId string,
	attachmentId string,
	options ...fetch_config.Option,
) (*body.Body, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if messageId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("message id"))
	}
	if attachmentId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("attachment id"))
	}

	return rest.GetJson[body.Body](
		ctx,
		c.messagesUrl(userId, messageId, "attachments/"+url.PathEscape(attachmentId), nil),
		c.fetchOptions(options),
	)
}
