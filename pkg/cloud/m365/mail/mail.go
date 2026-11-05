// Package mail is a client for the Microsoft Graph mail API of Exchange Online mailboxes: messages,
// their attachments, folders and the mailbox's categories. Every call addresses a mailbox by its
// user's id -- an object id, or a user principal name -- since an app's own token has no "me".
//
// Every request asks for immutable ids, so that a message keeps its id when it is moved between
// folders, archiving included.
package mail

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/internal/graph"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"

	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/get_attachment_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/get_message_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/list_messages_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/mail_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/types/attachment_content"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/types/mail_folder"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/types/message"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/types/message_update"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/types/outlook_category"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/mail/types/page"
)

// The well-known names of a mailbox's folders, taken wherever a folder id is.
const (
	FolderInbox        = "inbox"
	FolderArchive      = "archive"
	FolderSentItems    = "sentitems"
	FolderDrafts       = "drafts"
	FolderDeletedItems = "deleteditems"
	FolderJunkEmail    = "junkemail"
)

const (
	preferHeaderName = "Prefer"
	immutableIdPref  = `IdType="ImmutableId"`
)

// The OData query options, written as Graph documents them.
const (
	queryOptionSearch  = "$search"
	queryOptionFilter  = "$filter"
	queryOptionOrderBy = "$orderby"
	queryOptionSelect  = "$select"
	queryOptionExpand  = "$expand"
	queryOptionTop     = "$top"
)

var (
	// ErrForeignNextLink is a next link that is not one of the client's Graph for the mailbox asked
	// about. A next link is a whole URL, which the client sends its credential to: one from anywhere
	// else is refused.
	ErrForeignNextLink = errors.New("next link not of this mailbox")
	// ErrSearchWithFilter is a search given a filter or an order as well, which Graph does not take
	// together for messages.
	ErrSearchWithFilter = errors.New("a search takes no filter or order")
)

type Client struct {
	baseUrl *url.URL
	config  *mail_config.Config
}

func NewClient(options ...mail_config.Option) *Client {
	config := mail_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = graph.DefaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/v1.0"
	u.RawPath = ""
	return &Client{baseUrl: &u, config: config}
}

// fetchOptions are the client's options, the call's, and the Prefer header, last so that no option
// takes the immutable ids away.
func (c *Client) fetchOptions(bodyContentType string, options []fetch_config.Option) []fetch_config.Option {
	preferences := []string{immutableIdPref}
	if bodyContentType != "" {
		preferences = append(preferences, fmt.Sprintf("outlook.body-content-type=%q", bodyContentType))
	}

	return append(
		append(slices.Clip(c.config.FetchOptions), options...),
		fetch_config.WithHeaders(map[string]string{preferHeaderName: strings.Join(preferences, ", ")}),
	)
}

func userPath(userId string) *graph.PathBuilder {
	return new(graph.PathBuilder).Literal("/users/").Escaped(userId)
}

// messagesPath is the messages of the folder, or of the whole mailbox where folderId is empty.
func messagesPath(userId string, folderId string) *graph.PathBuilder {
	builder := userPath(userId)
	if folderId != "" {
		builder.Literal("/mailFolders/").Escaped(folderId)
	}
	return builder.Literal("/messages")
}

func messagePath(userId string, messageId string) *graph.PathBuilder {
	return userPath(userId).Literal("/messages/").Escaped(messageId)
}

func attachmentPath(userId string, messageId string, attachmentId string) *graph.PathBuilder {
	return messagePath(userId, messageId).Literal("/attachments/").Escaped(attachmentId)
}

type queryParameter struct {
	name  string
	value string
}

// encodeQuery is a query of the parameters with a value, in order. The names are written as they
// are, "$" and all; a value's spaces are "%20", since Graph does not read "+" as one everywhere.
func encodeQuery(parameters ...queryParameter) string {
	var parts []string
	for _, parameter := range parameters {
		if parameter.value == "" {
			continue
		}
		parts = append(parts, parameter.name+"="+strings.ReplaceAll(url.QueryEscape(parameter.value), "+", "%20"))
	}
	return strings.Join(parts, "&")
}

func (c *Client) urlWithQuery(builder *graph.PathBuilder, parameters ...queryParameter) string {
	return builder.UrlWithRawQuery(c.baseUrl, encodeQuery(parameters...))
}

// searchValue is a KQL query as $search takes it: in double quotes, its own escaped.
func searchValue(search string) string {
	if search == "" {
		return ""
	}
	return `"` + strings.ReplaceAll(strings.ReplaceAll(search, `\`, `\\`), `"`, `\"`) + `"`
}

// CheckNextLink refuses a next link that is not of the client's Graph, under its version, for the
// mailbox of userId: Graph writes a mailbox into a link as /users/<id>/ or /users('<id>')/.
func (c *Client) CheckNextLink(userId string, nextLink string) error {
	refuse := func() error {
		return altshiftErrors.NewWithTrace(fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, ErrForeignNextLink))
	}

	if userId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	u, err := url.Parse(nextLink)
	if err != nil {
		return refuse()
	}
	if u.Scheme != c.baseUrl.Scheme || !strings.EqualFold(u.Host, c.baseUrl.Host) || u.User != nil || u.Fragment != "" {
		return refuse()
	}
	// A path with a dot segment could climb out of the mailbox once Graph resolves it.
	if u.Path == "" || path.Clean(u.Path) != u.Path {
		return refuse()
	}

	lowered := strings.ToLower(u.Path)
	for _, prefix := range []string{
		c.baseUrl.Path + "/users/" + userId + "/",
		c.baseUrl.Path + "/users('" + userId + "')/",
	} {
		if strings.HasPrefix(lowered, strings.ToLower(prefix)) {
			return nil
		}
	}
	return refuse()
}

// ListMessagesPage lists one page of the mailbox's messages, as list_messages_config says. A next
// link from an earlier page asks for the one after it; Select and the rest are carried in the link.
func (c *Client) ListMessagesPage(
	ctx context.Context,
	userId string,
	options ...list_messages_config.Option,
) (*page.Page[*message.Message], error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}

	config := list_messages_config.New(options...)

	urlString := config.NextLink
	if urlString != "" {
		if err := c.CheckNextLink(userId, urlString); err != nil {
			return nil, err
		}
	} else {
		if config.Search != "" && (config.Filter != "" || len(config.OrderBy) != 0) {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, ErrSearchWithFilter))
		}
		if config.Top < 0 {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: top %d", altshiftErrors.ErrValidationError, config.Top))
		}

		top := ""
		if config.Top > 0 {
			top = strconv.Itoa(config.Top)
		}
		urlString = c.urlWithQuery(
			messagesPath(userId, config.FolderId),
			queryParameter{name: queryOptionSearch, value: searchValue(config.Search)},
			queryParameter{name: queryOptionFilter, value: config.Filter},
			queryParameter{name: queryOptionOrderBy, value: strings.Join(config.OrderBy, ",")},
			queryParameter{name: queryOptionSelect, value: strings.Join(config.Select, ",")},
			queryParameter{name: queryOptionExpand, value: config.Expand},
			queryParameter{name: queryOptionTop, value: top},
		)
	}

	response, err := rest.GetJson[graph.ListResponse[*message.Message]](
		ctx,
		urlString,
		c.fetchOptions(config.BodyContentType, config.FetchOptions),
	)
	if err != nil {
		return nil, err
	}

	return &page.Page[*message.Message]{Items: response.Value, NextLink: response.NextLink}, nil
}

// GetMessage retrieves the message identified by messageId.
func (c *Client) GetMessage(
	ctx context.Context,
	userId string,
	messageId string,
	options ...get_message_config.Option,
) (*message.Message, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if messageId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("message id"))
	}

	config := get_message_config.New(options...)
	urlString := c.urlWithQuery(
		messagePath(userId, messageId),
		queryParameter{name: queryOptionSelect, value: strings.Join(config.Select, ",")},
		queryParameter{name: queryOptionExpand, value: config.Expand},
	)

	return rest.GetJson[message.Message](ctx, urlString, c.fetchOptions(config.BodyContentType, config.FetchOptions))
}

// UpdateMessage changes what the update names of the message, and returns the message as it is now.
func (c *Client) UpdateMessage(
	ctx context.Context,
	userId string,
	messageId string,
	update *message_update.Update,
	options ...fetch_config.Option,
) (*message.Message, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if messageId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("message id"))
	}
	if update == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("message update"))
	}

	return rest.SendJson[message.Message](
		ctx,
		http.MethodPatch,
		messagePath(userId, messageId).Url(c.baseUrl, nil),
		update,
		c.fetchOptions("", options),
	)
}

type moveRequest struct {
	DestinationId string `json:"destinationId"`
}

// MoveMessage moves the message into the folder destinationId, an id or a well-known name
// (FolderArchive archives it), and returns it as it is there. With immutable ids, its id stays.
func (c *Client) MoveMessage(
	ctx context.Context,
	userId string,
	messageId string,
	destinationId string,
	options ...fetch_config.Option,
) (*message.Message, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if messageId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("message id"))
	}
	if destinationId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("destination id"))
	}

	return rest.SendJson[message.Message](
		ctx,
		http.MethodPost,
		messagePath(userId, messageId).Literal("/move").Url(c.baseUrl, nil),
		&moveRequest{DestinationId: destinationId},
		c.fetchOptions("", options),
	)
}

// GetAttachment retrieves the attachment's properties, and a file's content unless Select leaves
// it out.
func (c *Client) GetAttachment(
	ctx context.Context,
	userId string,
	messageId string,
	attachmentId string,
	options ...get_attachment_config.Option,
) (*message.Attachment, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if messageId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("message id"))
	}
	if attachmentId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("attachment id"))
	}

	config := get_attachment_config.New(options...)
	urlString := c.urlWithQuery(
		attachmentPath(userId, messageId, attachmentId),
		queryParameter{name: queryOptionSelect, value: strings.Join(config.Select, ",")},
		queryParameter{name: queryOptionExpand, value: config.Expand},
	)

	return rest.GetJson[message.Attachment](ctx, urlString, c.fetchOptions("", config.FetchOptions))
}

// OpenAttachmentContent opens the attachment's content, unread: a file's bytes, or an item's MIME.
// The caller reads the content's body and closes it. A reference attachment has none, and Graph
// refuses it.
func (c *Client) OpenAttachmentContent(
	ctx context.Context,
	userId string,
	messageId string,
	attachmentId string,
	options ...fetch_config.Option,
) (*attachment_content.Content, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if messageId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("message id"))
	}
	if attachmentId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("attachment id"))
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	urlString := attachmentPath(userId, messageId, attachmentId).Literal("/$value").Url(c.baseUrl, nil)
	response, _, err := altshiftHttpUtils.Fetch(
		ctx,
		urlString,
		append(c.fetchOptions("", options), fetch_config.WithSkipReadResponseBody(true))...,
	)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch: %w", err), urlString)
	}
	if response == nil || response.Body == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("response"))
	}

	return &attachment_content.Content{
		Body:        response.Body,
		ContentType: response.Header.Get("Content-Type"),
		Length:      response.ContentLength,
	}, nil
}

// ListMasterCategories lists the mailbox's categories: the ones Outlook offers, with their colours.
// A message may carry others, which Outlook shows uncoloured.
func (c *Client) ListMasterCategories(
	ctx context.Context,
	userId string,
	options ...fetch_config.Option,
) ([]*outlook_category.Category, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}

	firstUrl := userPath(userId).Literal("/outlook/masterCategories").Url(c.baseUrl, nil)
	return rest.ListPaginated(
		ctx,
		graph.NextLinkOr(firstUrl),
		graph.Extract[*outlook_category.Category],
		c.fetchOptions("", options),
	)
}

// GetMailFolder retrieves the folder folderId, an id or a well-known name.
func (c *Client) GetMailFolder(
	ctx context.Context,
	userId string,
	folderId string,
	options ...fetch_config.Option,
) (*mail_folder.MailFolder, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if folderId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("folder id"))
	}

	urlString := userPath(userId).Literal("/mailFolders/").Escaped(folderId).Url(c.baseUrl, nil)
	return rest.GetJson[mail_folder.MailFolder](ctx, urlString, c.fetchOptions("", options))
}
