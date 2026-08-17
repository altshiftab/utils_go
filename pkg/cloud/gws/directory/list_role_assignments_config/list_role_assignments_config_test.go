package list_role_assignments_config

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.UserKey != "" {
		t.Errorf("default UserKey = %q, want empty", defaults.UserKey)
	}
	if defaults.RoleId != "" {
		t.Errorf("default RoleId = %q, want empty", defaults.RoleId)
	}
	if len(defaults.FetchOptions) != 0 {
		t.Errorf("default FetchOptions len = %d, want 0", len(defaults.FetchOptions))
	}

	config := New(
		WithUserKey("user@example.com"),
		WithRoleId("role-123"),
		WithFetchOptions(fetch_config.WithMethod(http.MethodPost)),
	)
	if config.UserKey != "user@example.com" {
		t.Errorf("UserKey = %q, want %q", config.UserKey, "user@example.com")
	}
	if config.RoleId != "role-123" {
		t.Errorf("RoleId = %q, want %q", config.RoleId, "role-123")
	}
	if len(config.FetchOptions) != 1 {
		t.Fatalf("FetchOptions len = %d, want 1", len(config.FetchOptions))
	}
	if applied := fetch_config.New(config.FetchOptions...); applied.Method != http.MethodPost {
		t.Errorf("applied fetch Method = %q, want %q", applied.Method, http.MethodPost)
	}
}
