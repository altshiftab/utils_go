package gmail

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/get_message_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/gmail_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/list_history_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/list_messages_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/filter"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/history"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/label"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/modify_request"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/send_as"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/thread"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/watch_request"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/watch_response"
)

const Domain = "gmail.googleapis.com"

var defaultBaseUrl = &url.URL{
	Scheme: "https",
	Host:   Domain,
}

type Client struct {
	baseUrl *url.URL
	config  *gmail_config.Config
}

func NewClient(options ...gmail_config.Option) *Client {
	config := gmail_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = defaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/gmail/v1/users/"
	return &Client{baseUrl: &u, config: config}
}

func (c *Client) messagesUrl(userId string, messageId string) string {
	u := *c.baseUrl
	u.Path += url.PathEscape(userId) + "/messages"
	if messageId != "" {
		u.Path += "/" + url.PathEscape(messageId)
	}
	return u.String()
}

func (c *Client) sendUrl(userId string) string {
	u := *c.baseUrl
	u.Path += url.PathEscape(userId) + "/messages/send"
	return u.String()
}

func (c *Client) watchUrl(userId string) string {
	u := *c.baseUrl
	u.Path += url.PathEscape(userId) + "/watch"
	return u.String()
}

func (c *Client) historyUrl(userId string) string {
	u := *c.baseUrl
	u.Path += url.PathEscape(userId) + "/history"
	return u.String()
}

func (c *Client) sendAsUrl(userId string, sendAsEmail string) string {
	u := *c.baseUrl
	u.Path += url.PathEscape(userId) + "/settings/sendAs"
	if sendAsEmail != "" {
		u.Path += "/" + url.PathEscape(sendAsEmail)
	}
	return u.String()
}

func (c *Client) threadsUrl(userId string, threadId string) string {
	u := *c.baseUrl
	u.Path += url.PathEscape(userId) + "/threads"
	if threadId != "" {
		u.Path += "/" + url.PathEscape(threadId)
	}
	return u.String()
}

func (c *Client) labelsUrl(userId string, labelId string) string {
	u := *c.baseUrl
	u.Path += url.PathEscape(userId) + "/labels"
	if labelId != "" {
		u.Path += "/" + url.PathEscape(labelId)
	}
	return u.String()
}

func (c *Client) filtersUrl(userId string, filterId string) string {
	u := *c.baseUrl
	u.Path += url.PathEscape(userId) + "/settings/filters"
	if filterId != "" {
		u.Path += "/" + url.PathEscape(filterId)
	}
	return u.String()
}

func (c *Client) fetchOptions(options []fetch_config.Option) []fetch_config.Option {
	return append(c.config.FetchOptions, options...)
}

func withQuery(urlString string, query url.Values) string {
	if encoded := query.Encode(); encoded != "" {
		return urlString + "?" + encoded
	}
	return urlString
}

// Send sends the specified message to the recipients in the To, Cc, and Bcc headers.
// The message should have its Raw field set to a base64url-encoded RFC 2822 email.
func (c *Client) Send(ctx context.Context, userId string, msg *message.Message, options ...fetch_config.Option) (*message.Message, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if msg == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("message"))
	}

	return rest.SendJson[message.Message](ctx, http.MethodPost, c.sendUrl(userId), msg, c.fetchOptions(options))
}

// Watch sets up or renews a push notification watch on the given user's mailbox.
// Notifications are delivered to the Cloud Pub/Sub topic specified in the request's TopicName.
func (c *Client) Watch(ctx context.Context, userId string, request *watch_request.WatchRequest, options ...fetch_config.Option) (*watch_response.WatchResponse, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if request == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("request"))
	}

	return rest.SendJson[watch_response.WatchResponse](
		ctx,
		http.MethodPost,
		c.watchUrl(userId),
		request,
		c.fetchOptions(options),
	)
}

type listHistoryResponse struct {
	History       []*history.Record `json:"history"`
	NextPageToken string            `json:"nextPageToken"`
	HistoryId     string            `json:"historyId"`
}

// ListHistory retrieves all history records for the given user after the specified startHistoryId.
func (c *Client) ListHistory(ctx context.Context, userId string, startHistoryId string, options ...list_history_config.Option) ([]*history.Record, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if startHistoryId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("start history id"))
	}

	listHistoryConfig := list_history_config.New(options...)

	return rest.ListPaginated(
		ctx,
		func(pageToken string) string {
			query := url.Values{}
			query.Set("startHistoryId", startHistoryId)
			for _, historyType := range listHistoryConfig.HistoryTypes {
				query.Add("historyTypes", string(historyType))
			}
			if listHistoryConfig.LabelId != "" {
				query.Set("labelId", listHistoryConfig.LabelId)
			}
			if pageToken != "" {
				query.Set("pageToken", pageToken)
			}
			return withQuery(c.historyUrl(userId), query)
		},
		func(response *listHistoryResponse) ([]*history.Record, string) {
			return response.History, response.NextPageToken
		},
		c.fetchOptions(listHistoryConfig.FetchOptions),
	)
}

type listMessagesResponse struct {
	Messages           []*message.Message `json:"messages"`
	NextPageToken      string             `json:"nextPageToken"`
	ResultSizeEstimate int                `json:"resultSizeEstimate"`
}

// ListMessages retrieves all messages for the given user matching the optional query,
// following the paging to the end.
// Only message IDs and thread IDs are populated; use GetMessage to retrieve the full message.
//
// Spam and trash are left out unless WithIncludeSpamTrash asks for them, as Gmail
// leaves them out: a caller taking stock of everything a mailbox holds wants them,
// and one reading the inbox does not.
func (c *Client) ListMessages(ctx context.Context, userId string, options ...list_messages_config.Option) ([]*message.Message, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}

	listMessagesConfig := list_messages_config.New(options...)

	return rest.ListPaginated(
		ctx,
		func(pageToken string) string {
			query := url.Values{}
			if listMessagesConfig.Query != "" {
				query.Set("q", listMessagesConfig.Query)
			}
			for _, labelId := range listMessagesConfig.LabelIds {
				query.Add("labelIds", labelId)
			}
			if listMessagesConfig.IncludeSpamTrash {
				query.Set("includeSpamTrash", "true")
			}
			if listMessagesConfig.MaxResults > 0 {
				query.Set("maxResults", strconv.Itoa(listMessagesConfig.MaxResults))
			}
			if pageToken != "" {
				query.Set("pageToken", pageToken)
			}
			return withQuery(c.messagesUrl(userId, ""), query)
		},
		func(response *listMessagesResponse) ([]*message.Message, string) {
			return response.Messages, response.NextPageToken
		},
		c.fetchOptions(listMessagesConfig.FetchOptions),
	)
}

// GetMessage retrieves a message identified by messageId for the given user.
func (c *Client) GetMessage(ctx context.Context, userId string, messageId string, options ...get_message_config.Option) (*message.Message, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if messageId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("message id"))
	}

	getMessageConfig := get_message_config.New(options...)

	query := url.Values{}
	if getMessageConfig.Format != "" {
		query.Set("format", string(getMessageConfig.Format))
	}
	for _, header := range getMessageConfig.MetadataHeaders {
		query.Add("metadataHeaders", header)
	}

	return rest.GetJson[message.Message](
		ctx,
		withQuery(c.messagesUrl(userId, messageId), query),
		c.fetchOptions(getMessageConfig.FetchOptions),
	)
}

// Trash moves the given message to the user's trash. Requires the gmail.modify scope (or wider).
func (c *Client) Trash(ctx context.Context, userId string, messageId string, options ...fetch_config.Option) (*message.Message, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if messageId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("message id"))
	}

	return rest.SendJson[message.Message, any](
		ctx,
		http.MethodPost,
		c.messagesUrl(userId, messageId)+"/trash",
		nil,
		c.fetchOptions(options),
	)
}

// Modify changes which labels a message carries. Requires the gmail.modify
// scope (or wider).
//
// It is how a service marks what it has done with a message, which is the one
// record the mailbox keeps and a database cannot: a database knows what it
// ingested, and says nothing about what arrived and was never read.
//
// A request naming no label is refused. Gmail accepts it and changes nothing,
// so the call would succeed while doing none of what the caller meant.
func (c *Client) Modify(
	ctx context.Context,
	userId string,
	messageId string,
	request *modify_request.Request,
	options ...fetch_config.Option,
) (*message.Message, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if messageId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("message id"))
	}
	if request == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("modify request"))
	}
	if len(request.AddLabelIds) == 0 && len(request.RemoveLabelIds) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("label ids"))
	}

	return rest.SendJson[message.Message](
		ctx,
		http.MethodPost,
		c.messagesUrl(userId, messageId)+"/modify",
		request,
		c.fetchOptions(options),
	)
}

// ModifyThread changes which labels every message in a thread carries.
//
// The same call as Modify against the conversation rather than one message,
// which is what to use when what was decided applies to the exchange and not
// to a single arrival.
func (c *Client) ModifyThread(
	ctx context.Context,
	userId string,
	threadId string,
	request *modify_request.Request,
	options ...fetch_config.Option,
) (*thread.Thread, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if threadId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("thread id"))
	}
	if request == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("modify request"))
	}
	if len(request.AddLabelIds) == 0 && len(request.RemoveLabelIds) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("label ids"))
	}

	return rest.SendJson[thread.Thread](
		ctx,
		http.MethodPost,
		c.threadsUrl(userId, threadId)+"/modify",
		request,
		c.fetchOptions(options),
	)
}

type listLabelsResponse struct {
	Labels []*label.Label `json:"labels,omitzero"`
}

// ListLabels returns every label in the mailbox.
//
// A label is applied by id, and a user-created label's id is opaque, so this
// is how a caller finds the id of a label it knows only by name.
func (c *Client) ListLabels(ctx context.Context, userId string, options ...fetch_config.Option) ([]*label.Label, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}

	response, err := rest.SendJson[listLabelsResponse, any](
		ctx,
		http.MethodGet,
		c.labelsUrl(userId, ""),
		nil,
		c.fetchOptions(options),
	)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, nil
	}

	return response.Labels, nil
}

// CreateLabel adds a label to the mailbox and returns it with its id.
//
// Creating one that already exists is answered 409 by Gmail rather than
// returning the existing label, so a caller that wants "ensure it is there"
// lists first.
func (c *Client) CreateLabel(
	ctx context.Context,
	userId string,
	l *label.Label,
	options ...fetch_config.Option,
) (*label.Label, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if l == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("label"))
	}
	if l.Name == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("label name"))
	}

	return rest.SendJson[label.Label](
		ctx,
		http.MethodPost,
		c.labelsUrl(userId, ""),
		l,
		c.fetchOptions(options),
	)
}

// CreateSendAs creates a custom "from" send-as alias for the given user.
func (c *Client) CreateSendAs(ctx context.Context, userId string, s *send_as.SendAs, options ...fetch_config.Option) (*send_as.SendAs, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if s == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("send as"))
	}

	return rest.SendJson[send_as.SendAs](ctx, http.MethodPost, c.sendAsUrl(userId, ""), s, c.fetchOptions(options))
}

// GetSendAs retrieves a send-as alias identified by sendAsEmail for the given user.
func (c *Client) GetSendAs(ctx context.Context, userId string, sendAsEmail string, options ...fetch_config.Option) (*send_as.SendAs, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if sendAsEmail == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("send-as email"))
	}

	return rest.GetJson[send_as.SendAs](ctx, c.sendAsUrl(userId, sendAsEmail), c.fetchOptions(options))
}

// UpdateSendAs updates a send-as alias identified by sendAsEmail for the given user.
func (c *Client) UpdateSendAs(ctx context.Context, userId string, sendAsEmail string, s *send_as.SendAs, options ...fetch_config.Option) (*send_as.SendAs, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if sendAsEmail == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("send-as email"))
	}
	if s == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("send as"))
	}

	return rest.SendJson[send_as.SendAs](
		ctx,
		http.MethodPut,
		c.sendAsUrl(userId, sendAsEmail),
		s,
		c.fetchOptions(options),
	)
}

// DeleteSendAs deletes a send-as alias identified by sendAsEmail for the given user.
func (c *Client) DeleteSendAs(ctx context.Context, userId string, sendAsEmail string, options ...fetch_config.Option) error {
	if userId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if sendAsEmail == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("send-as email"))
	}

	return rest.Do(ctx, http.MethodDelete, c.sendAsUrl(userId, sendAsEmail), c.fetchOptions(options))
}

type listFiltersResponse struct {
	Filter []*filter.Filter `json:"filter"`
}

// CreateFilter creates a filter for the given user.
func (c *Client) CreateFilter(ctx context.Context, userId string, f *filter.Filter, options ...fetch_config.Option) (*filter.Filter, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if f == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("filter"))
	}

	return rest.SendJson[filter.Filter](ctx, http.MethodPost, c.filtersUrl(userId, ""), f, c.fetchOptions(options))
}

// GetFilter retrieves a filter identified by filterId for the given user.
func (c *Client) GetFilter(ctx context.Context, userId string, filterId string, options ...fetch_config.Option) (*filter.Filter, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if filterId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("filter id"))
	}

	return rest.GetJson[filter.Filter](ctx, c.filtersUrl(userId, filterId), c.fetchOptions(options))
}

// ListFilters retrieves all filters for the given user.
func (c *Client) ListFilters(ctx context.Context, userId string, options ...fetch_config.Option) ([]*filter.Filter, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}

	response, err := rest.GetJson[listFiltersResponse](ctx, c.filtersUrl(userId, ""), c.fetchOptions(options))
	if err != nil {
		return nil, err
	}

	return response.Filter, nil
}

// DeleteFilter deletes a filter identified by filterId for the given user.
func (c *Client) DeleteFilter(ctx context.Context, userId string, filterId string, options ...fetch_config.Option) error {
	if userId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if filterId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("filter id"))
	}

	return rest.Do(ctx, http.MethodDelete, c.filtersUrl(userId, filterId), c.fetchOptions(options))
}
