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
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/list_drafts_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/draft"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/page"
)

// checkDraftMessage refuses a draft with no message to save, which Gmail answers
// with a 400 that says less.
func checkDraftMessage(d *draft.Draft) error {
	if d == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("draft"))
	}
	if d.Message == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("draft message"))
	}
	if d.Message.Raw == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("draft message raw"))
	}
	return nil
}

// CreateDraft saves a message as a draft. The message's Raw is a base64url-encoded
// RFC 5322 message, as Send takes; a ThreadId puts the draft in that thread,
// which Gmail does only when the message's headers say it is a reply in it too.
// Requires the gmail.compose scope (or wider).
func (c *Client) CreateDraft(ctx context.Context, userId string, d *draft.Draft, options ...fetch_config.Option) (*draft.Draft, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if err := checkDraftMessage(d); err != nil {
		return nil, err
	}

	return rest.SendJson[draft.Draft](ctx, http.MethodPost, c.draftsUrl(userId, "", nil), d, c.fetchOptions(options))
}

// GetDraft retrieves a draft and its message, in the format the options ask for.
func (c *Client) GetDraft(
	ctx context.Context,
	userId string,
	draftId string,
	options ...get_message_config.Option,
) (*draft.Draft, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if draftId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("draft id"))
	}

	getMessageConfig := get_message_config.New(options...)

	query := url.Values{}
	if getMessageConfig.Format != "" {
		query.Set("format", string(getMessageConfig.Format))
	}

	return rest.GetJson[draft.Draft](ctx, c.draftsUrl(userId, draftId, query), c.fetchOptions(getMessageConfig.FetchOptions))
}

type listDraftsResponse struct {
	Drafts             []*draft.Draft `json:"drafts"`
	NextPageToken      string         `json:"nextPageToken"`
	ResultSizeEstimate int            `json:"resultSizeEstimate"`
}

// ListDraftsPage retrieves one page of the user's drafts, beginning at the
// options' page token. Each carries its id and its message's ids; GetDraft reads
// one.
func (c *Client) ListDraftsPage(
	ctx context.Context,
	userId string,
	options ...list_drafts_config.Option,
) (*page.Page[*draft.Draft], error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}

	listDraftsConfig := list_drafts_config.New(options...)

	query := url.Values{}
	if listDraftsConfig.Query != "" {
		query.Set("q", listDraftsConfig.Query)
	}
	if listDraftsConfig.IncludeSpamTrash {
		query.Set("includeSpamTrash", "true")
	}
	if listDraftsConfig.MaxResults > 0 {
		query.Set("maxResults", strconv.Itoa(listDraftsConfig.MaxResults))
	}
	if listDraftsConfig.PageToken != "" {
		query.Set("pageToken", listDraftsConfig.PageToken)
	}

	response, err := rest.GetJson[listDraftsResponse](
		ctx,
		c.draftsUrl(userId, "", query),
		c.fetchOptions(listDraftsConfig.FetchOptions),
	)
	if err != nil {
		return nil, err
	}

	return &page.Page[*draft.Draft]{
		Items:              response.Drafts,
		NextPageToken:      response.NextPageToken,
		ResultSizeEstimate: response.ResultSizeEstimate,
	}, nil
}

// UpdateDraft replaces a draft's message with the one given, keeping the draft's
// id. Requires the gmail.compose scope (or wider).
func (c *Client) UpdateDraft(
	ctx context.Context,
	userId string,
	draftId string,
	d *draft.Draft,
	options ...fetch_config.Option,
) (*draft.Draft, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if draftId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("draft id"))
	}
	if err := checkDraftMessage(d); err != nil {
		return nil, err
	}

	return rest.SendJson[draft.Draft](ctx, http.MethodPut, c.draftsUrl(userId, draftId, nil), d, c.fetchOptions(options))
}

// DeleteDraft deletes a draft outright; it does not go to the trash. Requires
// the gmail.compose scope (or wider).
func (c *Client) DeleteDraft(ctx context.Context, userId string, draftId string, options ...fetch_config.Option) error {
	if userId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if draftId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("draft id"))
	}

	return rest.Do(ctx, http.MethodDelete, c.draftsUrl(userId, draftId, nil), c.fetchOptions(options))
}

// SendDraft sends a saved draft as it stands, and returns the message sent.
// Requires the gmail.compose scope (or wider).
//
// It names the draft by id alone, so what goes out is what was saved, rather
// than whatever a caller sends along with the id.
func (c *Client) SendDraft(ctx context.Context, userId string, draftId string, options ...fetch_config.Option) (*message.Message, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if draftId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("draft id"))
	}

	return rest.SendJson[message.Message](
		ctx,
		http.MethodPost,
		c.draftsUrl(userId, "send", nil),
		&draft.Draft{Id: draftId},
		c.fetchOptions(options),
	)
}
