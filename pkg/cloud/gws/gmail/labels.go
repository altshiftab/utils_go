package gmail

import (
	"context"
	"net/http"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/label"
)

// GetLabel retrieves one label, with the counts of the messages and threads
// carrying it, which a listing of every label leaves out.
func (c *Client) GetLabel(ctx context.Context, userId string, labelId string, options ...fetch_config.Option) (*label.Label, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if labelId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("label id"))
	}

	return rest.GetJson[label.Label](ctx, c.labelsUrl(userId, labelId), c.fetchOptions(options))
}

// UpdateLabel changes the fields of a label that l sets, leaving the rest; it is
// how a label is renamed. A system label is refused by Gmail.
func (c *Client) UpdateLabel(
	ctx context.Context,
	userId string,
	labelId string,
	l *label.Label,
	options ...fetch_config.Option,
) (*label.Label, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if labelId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("label id"))
	}
	if l == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("label"))
	}

	return rest.SendJson[label.Label](ctx, http.MethodPatch, c.labelsUrl(userId, labelId), l, c.fetchOptions(options))
}

// DeleteLabel deletes a label and takes it off every message carrying it; the
// messages themselves stay. A system label is refused by Gmail.
func (c *Client) DeleteLabel(ctx context.Context, userId string, labelId string, options ...fetch_config.Option) error {
	if userId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}
	if labelId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("label id"))
	}

	return rest.Do(ctx, http.MethodDelete, c.labelsUrl(userId, labelId), c.fetchOptions(options))
}
