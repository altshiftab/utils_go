package asset

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_asset_inventory/types/resource"
)

type Asset struct {
	Name       string             `json:"name,omitempty"`
	AssetType  string             `json:"assetType,omitempty"`
	Resource   *resource.Resource `json:"resource,omitempty"`
	Ancestors  []string           `json:"ancestors,omitempty"`
	UpdateTime string             `json:"updateTime,omitempty"`
}
