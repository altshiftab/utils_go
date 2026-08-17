package groups_settings

import (
	"context"
	"net/http"
	"net/url"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/groups_settings/groups_settings_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/groups_settings/types/group"
)

const Domain = "www.googleapis.com"

var defaultBaseUrl = &url.URL{
	Scheme: "https",
	Host:   Domain,
}

type Client struct {
	baseUrl *url.URL
	config  *groups_settings_config.Config
}

func NewClient(options ...groups_settings_config.Option) *Client {
	config := groups_settings_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = defaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/groups/v1/groups/"
	return &Client{baseUrl: &u, config: config}
}

func (c *Client) groupUrl(groupEmail string) string {
	u := *c.baseUrl
	u.Path += url.PathEscape(groupEmail)
	// The Groups Settings API defaults to Atom XML; request JSON explicitly.
	u.RawQuery = "alt=json"
	return u.String()
}

func (c *Client) fetchOptions(options []fetch_config.Option) []fetch_config.Option {
	return append(c.config.FetchOptions, options...)
}

// Get retrieves a group's settings identified by the group email address.
func (c *Client) Get(ctx context.Context, groupEmail string, options ...fetch_config.Option) (*group.Group, error) {
	if groupEmail == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("group email"))
	}

	return rest.GetJson[group.Group](ctx, c.groupUrl(groupEmail), c.fetchOptions(options))
}

// Update updates an existing group's settings identified by the group email address.
func (c *Client) Update(ctx context.Context, groupEmail string, groupSettings *group.Group, options ...fetch_config.Option) (*group.Group, error) {
	if groupEmail == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("group email"))
	}

	return rest.SendJson[group.Group](ctx, http.MethodPut, c.groupUrl(groupEmail), groupSettings, c.fetchOptions(options))
}

// Patch updates an existing group's settings using patch semantics.
func (c *Client) Patch(ctx context.Context, groupEmail string, groupSettings *group.Group, options ...fetch_config.Option) (*group.Group, error) {
	if groupEmail == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("group email"))
	}

	return rest.SendJson[group.Group](ctx, http.MethodPatch, c.groupUrl(groupEmail), groupSettings, c.fetchOptions(options))
}
