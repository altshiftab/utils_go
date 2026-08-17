package resource_search_result_list

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_asset_inventory/types/resource_search_result"
)

type ResourceSearchResultList struct {
	Results       []*resource_search_result.ResourceSearchResult `json:"results,omitempty"`
	NextPageToken string                                         `json:"nextPageToken,omitempty"`
}
