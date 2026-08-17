package drive

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/create_permission_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/drive_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/types/permission"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/update_permission_config"
)

func testServer(t *testing.T, handler http.HandlerFunc, options ...drive_config.Option) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return NewClient(append([]drive_config.Option{drive_config.WithBaseUrl(u)}, options...)...)
}

func TestCreatePermission(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/drive/v3/files/file-1/permissions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if sendNotificationEmail := r.URL.Query().Get("sendNotificationEmail"); sendNotificationEmail != "false" {
			t.Errorf("sendNotificationEmail = %q, want %q", sendNotificationEmail, "false")
		}

		var requestPermission permission.Permission
		if err := json.UnmarshalRead(r.Body, &requestPermission); err != nil {
			t.Errorf("unmarshal request: %v", err)
		}
		if requestPermission.Type != permission.TypeUser {
			t.Errorf("Type = %q, want %q", requestPermission.Type, permission.TypeUser)
		}
		if requestPermission.EmailAddress != "sa@project.iam.gserviceaccount.com" {
			t.Errorf("EmailAddress = %q", requestPermission.EmailAddress)
		}
		if requestPermission.Role != permission.RoleReader {
			t.Errorf("Role = %q, want %q", requestPermission.Role, permission.RoleReader)
		}

		w.Header().Set("Content-Type", "application/json")
		requestPermission.Id = "permission-1"
		if err := json.MarshalWrite(w, &requestPermission); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	result, err := client.CreatePermission(
		context.Background(),
		"file-1",
		&permission.Permission{
			Type:         permission.TypeUser,
			EmailAddress: "sa@project.iam.gserviceaccount.com",
			Role:         permission.RoleReader,
		},
		create_permission_config.WithSendNotificationEmail(false),
	)
	if err != nil {
		t.Fatalf("create permission: %v", err)
	}
	if result.Id != "permission-1" {
		t.Errorf("Id = %q, want %q", result.Id, "permission-1")
	}
}

func TestCreatePermissionSupportsAllDrives(t *testing.T) {
	t.Parallel()

	client := testServer(
		t,
		func(w http.ResponseWriter, r *http.Request) {
			if supportsAllDrives := r.URL.Query().Get("supportsAllDrives"); supportsAllDrives != "true" {
				t.Errorf("supportsAllDrives = %q, want %q", supportsAllDrives, "true")
			}

			w.Header().Set("Content-Type", "application/json")
			if err := json.MarshalWrite(w, &permission.Permission{Id: "permission-1"}); err != nil {
				t.Errorf("marshal: %v", err)
			}
		},
		drive_config.WithSupportsAllDrives(true),
	)

	if _, err := client.CreatePermission(
		context.Background(),
		"file-1",
		&permission.Permission{Type: permission.TypeAnyone, Role: permission.RoleReader},
	); err != nil {
		t.Fatalf("create permission: %v", err)
	}
}

func TestGetPermission(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/drive/v3/files/file-1/permissions/permission-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &permission.Permission{
			Id:           "permission-1",
			Type:         permission.TypeUser,
			EmailAddress: "sa@project.iam.gserviceaccount.com",
			Role:         permission.RoleReader,
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	result, err := client.GetPermission(context.Background(), "file-1", "permission-1")
	if err != nil {
		t.Fatalf("get permission: %v", err)
	}
	if result.Role != permission.RoleReader {
		t.Errorf("Role = %q, want %q", result.Role, permission.RoleReader)
	}
}

func TestListPermissionsPaginated(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var response any
		if r.URL.Query().Get("pageToken") == "" {
			response = map[string]any{
				"permissions":   []*permission.Permission{{Id: "permission-1"}},
				"nextPageToken": "token-1",
			}
		} else {
			response = map[string]any{
				"permissions": []*permission.Permission{{Id: "permission-2"}},
			}
		}
		if err := json.MarshalWrite(w, response); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	result, err := client.ListPermissions(context.Background(), "file-1")
	if err != nil {
		t.Fatalf("list permissions: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0].Id != "permission-1" || result[1].Id != "permission-2" {
		t.Errorf("ids = %q, %q", result[0].Id, result[1].Id)
	}
}

func TestUpdatePermission(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/drive/v3/files/file-1/permissions/permission-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if removeExpiration := r.URL.Query().Get("removeExpiration"); removeExpiration != "true" {
			t.Errorf("removeExpiration = %q, want %q", removeExpiration, "true")
		}

		var requestPermission permission.Permission
		if err := json.UnmarshalRead(r.Body, &requestPermission); err != nil {
			t.Errorf("unmarshal request: %v", err)
		}
		if requestPermission.Role != permission.RoleWriter {
			t.Errorf("Role = %q, want %q", requestPermission.Role, permission.RoleWriter)
		}

		w.Header().Set("Content-Type", "application/json")
		requestPermission.Id = "permission-1"
		if err := json.MarshalWrite(w, &requestPermission); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	result, err := client.UpdatePermission(
		context.Background(),
		"file-1",
		"permission-1",
		&permission.Permission{Role: permission.RoleWriter},
		update_permission_config.WithRemoveExpiration(true),
	)
	if err != nil {
		t.Fatalf("update permission: %v", err)
	}
	if result.Role != permission.RoleWriter {
		t.Errorf("Role = %q, want %q", result.Role, permission.RoleWriter)
	}
}

func TestDeletePermission(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/drive/v3/files/file-1/permissions/permission-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	if err := client.DeletePermission(context.Background(), "file-1", "permission-1"); err != nil {
		t.Fatalf("delete permission: %v", err)
	}
}

func TestEmptyArgumentErrors(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx := context.Background()

	testCases := []struct {
		name string
		call func() error
	}{
		{
			name: "create permission empty file id",
			call: func() error {
				_, err := client.CreatePermission(ctx, "", &permission.Permission{})
				return err
			},
		},
		{
			name: "create permission nil permission",
			call: func() error {
				_, err := client.CreatePermission(ctx, "file-1", nil)
				return err
			},
		},
		{
			name: "get permission empty file id",
			call: func() error {
				_, err := client.GetPermission(ctx, "", "permission-1")
				return err
			},
		},
		{
			name: "get permission empty permission id",
			call: func() error {
				_, err := client.GetPermission(ctx, "file-1", "")
				return err
			},
		},
		{
			name: "list permissions empty file id",
			call: func() error {
				_, err := client.ListPermissions(ctx, "")
				return err
			},
		},
		{
			name: "update permission empty file id",
			call: func() error {
				_, err := client.UpdatePermission(ctx, "", "permission-1", &permission.Permission{})
				return err
			},
		},
		{
			name: "update permission empty permission id",
			call: func() error {
				_, err := client.UpdatePermission(ctx, "file-1", "", &permission.Permission{})
				return err
			},
		},
		{
			name: "update permission nil permission",
			call: func() error {
				_, err := client.UpdatePermission(ctx, "file-1", "permission-1", nil)
				return err
			},
		},
		{
			name: "delete permission empty file id",
			call: func() error {
				return client.DeletePermission(ctx, "", "permission-1")
			},
		},
		{
			name: "delete permission empty permission id",
			call: func() error {
				return client.DeletePermission(ctx, "file-1", "")
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if err := testCase.call(); err == nil {
				t.Error("expected an error")
			}
		})
	}
}
