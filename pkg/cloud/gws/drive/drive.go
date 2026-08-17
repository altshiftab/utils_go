package drive

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

	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/create_permission_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/drive_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/types/permission"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/update_permission_config"
)

const Domain = "www.googleapis.com"

const (
	ScopeDrive         = "https://www.googleapis.com/auth/drive"
	ScopeDriveFile     = "https://www.googleapis.com/auth/drive.file"
	ScopeDriveReadonly = "https://www.googleapis.com/auth/drive.readonly"
)

var defaultBaseUrl = &url.URL{
	Scheme: "https",
	Host:   Domain,
}

type Client struct {
	baseUrl *url.URL
	config  *drive_config.Config
}

func NewClient(options ...drive_config.Option) *Client {
	config := drive_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = defaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/drive/v3/"
	return &Client{baseUrl: &u, config: config}
}

func (c *Client) urlString(path string, query url.Values) string {
	urlObj := *c.baseUrl
	urlObj.Path += path
	if len(query) != 0 {
		urlObj.RawQuery = query.Encode()
	}
	return urlObj.String()
}

// newQuery seeds the query parameters every permissions call shares.
func (c *Client) newQuery() url.Values {
	query := url.Values{}
	if c.config.SupportsAllDrives {
		query.Set("supportsAllDrives", "true")
	}
	return query
}

func (c *Client) fetchOptions(options []fetch_config.Option) []fetch_config.Option {
	return append(c.config.FetchOptions, options...)
}

func permissionsPath(fileId string) string {
	return "files/" + url.PathEscape(fileId) + "/permissions"
}

func permissionPath(fileId string, permissionId string) string {
	return permissionsPath(fileId) + "/" + url.PathEscape(permissionId)
}

// Permission operations

// CreatePermission grants a user, group, domain, or anyone access to the file
// identified by fileId.
func (c *Client) CreatePermission(
	ctx context.Context,
	fileId string,
	p *permission.Permission,
	options ...create_permission_config.Option,
) (*permission.Permission, error) {
	if fileId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("file id"))
	}
	if p == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("permission"))
	}

	createPermissionConfig := create_permission_config.New(options...)

	query := c.newQuery()
	if sendNotificationEmail := createPermissionConfig.SendNotificationEmail; sendNotificationEmail != nil {
		query.Set("sendNotificationEmail", strconv.FormatBool(*sendNotificationEmail))
	}
	if emailMessage := createPermissionConfig.EmailMessage; emailMessage != "" {
		query.Set("emailMessage", emailMessage)
	}
	if createPermissionConfig.TransferOwnership {
		query.Set("transferOwnership", "true")
	}

	return rest.SendJson[permission.Permission](
		ctx,
		http.MethodPost,
		c.urlString(permissionsPath(fileId), query),
		p,
		c.fetchOptions(createPermissionConfig.FetchOptions),
	)
}

// GetPermission retrieves the permission identified by permissionId on the file
// identified by fileId.
func (c *Client) GetPermission(
	ctx context.Context,
	fileId string,
	permissionId string,
	options ...fetch_config.Option,
) (*permission.Permission, error) {
	if fileId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("file id"))
	}
	if permissionId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("permission id"))
	}

	return rest.GetJson[permission.Permission](
		ctx,
		c.urlString(permissionPath(fileId, permissionId), c.newQuery()),
		c.fetchOptions(options),
	)
}

type listPermissionsResponse struct {
	Permissions   []*permission.Permission `json:"permissions"`
	NextPageToken string                   `json:"nextPageToken"`
}

// ListPermissions retrieves all permissions on the file identified by fileId.
func (c *Client) ListPermissions(
	ctx context.Context,
	fileId string,
	options ...fetch_config.Option,
) ([]*permission.Permission, error) {
	if fileId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("file id"))
	}

	return rest.ListPaginated(
		ctx,
		func(pageToken string) string {
			query := c.newQuery()
			if pageToken != "" {
				query.Set("pageToken", pageToken)
			}
			return c.urlString(permissionsPath(fileId), query)
		},
		func(response *listPermissionsResponse) ([]*permission.Permission, string) {
			return response.Permissions, response.NextPageToken
		},
		c.fetchOptions(options),
	)
}

// UpdatePermission patches the permission identified by permissionId on the
// file identified by fileId. Only the writable fields (role, allowFileDiscovery,
// expirationTime) may be set in p.
func (c *Client) UpdatePermission(
	ctx context.Context,
	fileId string,
	permissionId string,
	p *permission.Permission,
	options ...update_permission_config.Option,
) (*permission.Permission, error) {
	if fileId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("file id"))
	}
	if permissionId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("permission id"))
	}
	if p == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("permission"))
	}

	updatePermissionConfig := update_permission_config.New(options...)

	query := c.newQuery()
	if updatePermissionConfig.TransferOwnership {
		query.Set("transferOwnership", "true")
	}
	if updatePermissionConfig.RemoveExpiration {
		query.Set("removeExpiration", "true")
	}

	return rest.SendJson[permission.Permission](
		ctx,
		http.MethodPatch,
		c.urlString(permissionPath(fileId, permissionId), query),
		p,
		c.fetchOptions(updatePermissionConfig.FetchOptions),
	)
}

// DeletePermission removes the permission identified by permissionId from the
// file identified by fileId.
func (c *Client) DeletePermission(
	ctx context.Context,
	fileId string,
	permissionId string,
	options ...fetch_config.Option,
) error {
	if fileId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("file id"))
	}
	if permissionId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("permission id"))
	}

	return rest.Do(
		ctx,
		http.MethodDelete,
		c.urlString(permissionPath(fileId, permissionId), c.newQuery()),
		c.fetchOptions(options),
	)
}
