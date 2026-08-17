package asset_list

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_asset_inventory/types/asset"
)

type AssetList struct {
	ReadTime      string         `json:"readTime,omitempty"`
	Assets        []*asset.Asset `json:"assets,omitempty"`
	NextPageToken string         `json:"nextPageToken,omitempty"`
}
