package cloud_asset_inventory

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_asset_inventory/cloud_asset_inventory_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_asset_inventory/types/asset"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_asset_inventory/types/asset_list"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_asset_inventory/types/resource"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_asset_inventory/types/resource_search_result"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_asset_inventory/types/resource_search_result_list"
)

func testServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return NewClient(cloud_asset_inventory_config.WithBaseUrl(u))
}

func TestListAssets(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1/organizations/123456/assets") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("assetTypes") != "cloudresourcemanager.googleapis.com/Project" {
			t.Errorf("unexpected assetTypes: %s", r.URL.Query().Get("assetTypes"))
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &asset_list.AssetList{
			ReadTime: "2024-01-01T00:00:00Z",
			Assets: []*asset.Asset{
				{
					Name:      "//cloudresourcemanager.googleapis.com/projects/my-project",
					AssetType: "cloudresourcemanager.googleapis.com/Project",
					Resource: &resource.Resource{
						Version:       "v3",
						ResourceUrl:   "//cloudresourcemanager.googleapis.com/projects/my-project",
						DiscoveryName: "Project",
					},
					Ancestors: []string{"projects/my-project", "organizations/123456"},
				},
			},
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	list, err := client.ListAssets(
		context.Background(),
		"organizations/123456",
		url.Values{"assetTypes": {"cloudresourcemanager.googleapis.com/Project"}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list.Assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(list.Assets))
	}
	if list.Assets[0].AssetType != "cloudresourcemanager.googleapis.com/Project" {
		t.Errorf("unexpected asset type: %s", list.Assets[0].AssetType)
	}
	if list.Assets[0].Resource == nil {
		t.Fatal("expected non-nil resource")
	}
	if list.Assets[0].Resource.Version != "v3" {
		t.Errorf("expected version 'v3', got %q", list.Assets[0].Resource.Version)
	}
}

func TestListAssets_EmptyParent(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.ListAssets(context.Background(), "", nil)
	if err == nil {
		t.Fatal("expected error for empty parent")
	}
}

func TestListAssets_NilQuery(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &asset_list.AssetList{ReadTime: "2024-01-01T00:00:00Z"}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	_, err := client.ListAssets(context.Background(), "projects/my-project", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListAssets_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.ListAssets(ctx, "organizations/123456", nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestSearchAllResources(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1/organizations/123456:searchAllResources") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("assetTypes") != "run.googleapis.com/Service" {
			t.Errorf("unexpected assetTypes: %s", r.URL.Query().Get("assetTypes"))
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &resource_search_result_list.ResourceSearchResultList{
			Results: []*resource_search_result.ResourceSearchResult{
				{
					Name:        "//run.googleapis.com/projects/my-project/locations/us-central1/services/my-service",
					AssetType:   "run.googleapis.com/Service",
					Project:     "projects/my-project",
					DisplayName: "my-service",
					Location:    "us-central1",
					State:       "ACTIVE",
				},
			},
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	list, err := client.SearchAllResources(
		context.Background(),
		"organizations/123456",
		url.Values{"assetTypes": {"run.googleapis.com/Service"}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(list.Results))
	}
	if list.Results[0].DisplayName != "my-service" {
		t.Errorf("expected display name 'my-service', got %q", list.Results[0].DisplayName)
	}
	if list.Results[0].Location != "us-central1" {
		t.Errorf("expected location 'us-central1', got %q", list.Results[0].Location)
	}
}

func TestSearchAllResources_EmptyScope(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.SearchAllResources(context.Background(), "", nil)
	if err == nil {
		t.Fatal("expected error for empty scope")
	}
}

func TestSearchAllResources_NilQuery(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &resource_search_result_list.ResourceSearchResultList{}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	_, err := client.SearchAllResources(context.Background(), "projects/my-project", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchAllResources_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.SearchAllResources(ctx, "organizations/123456", nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
