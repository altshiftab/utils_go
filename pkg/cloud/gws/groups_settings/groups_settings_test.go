package groups_settings

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/groups_settings/groups_settings_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/groups_settings/types/group"
)

func testServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return NewClient(groups_settings_config.WithBaseUrl(u))
}

func TestGet(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/groups/v1/groups/group@example.com") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &group.Group{
			Kind:              "groupsSettings#groups",
			Email:             "group@example.com",
			Name:              "Test Group",
			WhoCanPostMessage: "ALL_MEMBERS_CAN_POST",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	g, err := client.Get(context.Background(), "group@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Email != "group@example.com" {
		t.Errorf("expected email 'group@example.com', got %q", g.Email)
	}
	if g.WhoCanPostMessage != "ALL_MEMBERS_CAN_POST" {
		t.Errorf("expected 'ALL_MEMBERS_CAN_POST', got %q", g.WhoCanPostMessage)
	}
}

func TestGet_EmptyEmail(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.Get(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty group email")
	}
}

func TestGet_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Get(ctx, "group@example.com")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestUpdate(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}

		var input group.Group
		if err := json.UnmarshalRead(r.Body, &input); err != nil {
			t.Errorf("decode: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &group.Group{
			Email:             "group@example.com",
			WhoCanPostMessage: input.WhoCanPostMessage,
			AllowWebPosting:   input.AllowWebPosting,
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	g, err := client.Update(context.Background(), "group@example.com", &group.Group{
		WhoCanPostMessage: "ANYONE_CAN_POST",
		AllowWebPosting:   "true",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.WhoCanPostMessage != "ANYONE_CAN_POST" {
		t.Errorf("expected 'ANYONE_CAN_POST', got %q", g.WhoCanPostMessage)
	}
}

func TestUpdate_EmptyEmail(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.Update(context.Background(), "", &group.Group{})
	if err == nil {
		t.Fatal("expected error for empty group email")
	}
}

func TestUpdate_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Update(ctx, "group@example.com", &group.Group{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestPatch(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}

		var input group.Group
		if err := json.UnmarshalRead(r.Body, &input); err != nil {
			t.Errorf("decode: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &group.Group{
			Email:                  "group@example.com",
			MessageModerationLevel: input.MessageModerationLevel,
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	g, err := client.Patch(context.Background(), "group@example.com", &group.Group{
		MessageModerationLevel: "MODERATE_ALL_MESSAGES",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.MessageModerationLevel != "MODERATE_ALL_MESSAGES" {
		t.Errorf("expected 'MODERATE_ALL_MESSAGES', got %q", g.MessageModerationLevel)
	}
}

func TestPatch_EmptyEmail(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.Patch(context.Background(), "", &group.Group{})
	if err == nil {
		t.Fatal("expected error for empty group email")
	}
}

func TestPatch_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Patch(ctx, "group@example.com", &group.Group{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGroupUrl(t *testing.T) {
	t.Parallel()

	u, _ := url.Parse("http://localhost:8080")
	client := NewClient(groups_settings_config.WithBaseUrl(u))
	got := client.groupUrl("group@example.com")
	expected := "http://localhost:8080/groups/v1/groups/group@example.com?alt=json"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}
