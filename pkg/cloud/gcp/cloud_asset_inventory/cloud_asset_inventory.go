package cloud_asset_inventory

import (
	"context"
	"net/url"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_asset_inventory/cloud_asset_inventory_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_asset_inventory/types/asset_list"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_asset_inventory/types/resource_search_result_list"
	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

const Domain = "cloudasset.googleapis.com"

var defaultBaseUrl = &url.URL{
	Scheme: "https",
	Host:   Domain,
}

type Client struct {
	baseUrl *url.URL
	config  *cloud_asset_inventory_config.Config
}

func NewClient(options ...cloud_asset_inventory_config.Option) *Client {
	config := cloud_asset_inventory_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = defaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/v1/"

	return &Client{baseUrl: &u, config: config}
}

func (c *Client) urlString(path string, query url.Values) string {
	u := *c.baseUrl
	u.Path += path
	if query != nil {
		u.RawQuery = query.Encode()
	}
	return u.String()
}

// ListAssets lists assets under the specified parent (e.g. "organizations/123456", "projects/my-project", or "folders/123456").
// Use the query parameter to specify assetTypes, contentType, pageSize, pageToken, and other query parameters.
func (c *Client) ListAssets(ctx context.Context, parent string, query url.Values, options ...fetch_config.Option) (*asset_list.AssetList, error) {
	if parent == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("parent"))
	}

	return rest.GetJson[asset_list.AssetList](
		ctx,
		c.urlString(parent+"/assets", query),
		append(c.config.FetchOptions, options...),
	)
}

// SearchAllResources searches all resources within the specified scope (e.g. "organizations/123456", "projects/my-project", or "folders/123456").
// Use the query parameter to specify query, assetTypes, pageSize, pageToken, orderBy, readMask, and other query parameters.
func (c *Client) SearchAllResources(ctx context.Context, scope string, query url.Values, options ...fetch_config.Option) (*resource_search_result_list.ResourceSearchResultList, error) {
	if scope == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("scope"))
	}

	return rest.GetJson[resource_search_result_list.ResourceSearchResultList](
		ctx,
		c.urlString(scope+":searchAllResources", query),
		append(c.config.FetchOptions, options...),
	)
}
