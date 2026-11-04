package gmail

import (
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/get_message_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/list_drafts_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/list_messages_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/draft"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/label"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message"
)

// TestRequests checks that each call asks Gmail what it should -- method, path, query and body -- and reads back
// what Gmail answers.
func TestRequests(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		call     func(client *Client) (any, error)
		method   string
		path     string
		query    url.Values
		body     string
		response string
		// want is the result, as JSON; empty for a call that returns none.
		want string
	}{
		{
			name: "list messages page",
			call: func(client *Client) (any, error) {
				return client.ListMessagesPage(
					context.Background(),
					"me",
					list_messages_config.WithQuery("has:attachment"),
					list_messages_config.WithMaxResults(10),
					list_messages_config.WithPageToken("token-1"),
				)
			},
			method:   http.MethodGet,
			path:     "/gmail/v1/users/me/messages",
			query:    url.Values{"q": {"has:attachment"}, "maxResults": {"10"}, "pageToken": {"token-1"}},
			response: `{"messages":[{"id":"m1","threadId":"t1"}],"nextPageToken":"token-2","resultSizeEstimate":31}`,
			want:     `{"Items":[{"id":"m1","threadId":"t1"}],"NextPageToken":"token-2","ResultSizeEstimate":31}`,
		},
		{
			name: "list threads page",
			call: func(client *Client) (any, error) {
				return client.ListThreadsPage(
					context.Background(),
					"me",
					list_messages_config.WithQuery("from:a@example.com"),
					list_messages_config.WithLabelIds("INBOX"),
					list_messages_config.WithIncludeSpamTrash(true),
				)
			},
			method:   http.MethodGet,
			path:     "/gmail/v1/users/me/threads",
			query:    url.Values{"q": {"from:a@example.com"}, "labelIds": {"INBOX"}, "includeSpamTrash": {"true"}},
			response: `{"threads":[{"id":"t1","snippet":"hello"}],"resultSizeEstimate":1}`,
			want:     `{"Items":[{"id":"t1","snippet":"hello"}],"NextPageToken":"","ResultSizeEstimate":1}`,
		},
		{
			name: "get thread",
			call: func(client *Client) (any, error) {
				return client.GetThread(
					context.Background(),
					"me",
					"t1",
					get_message_config.WithFormat(get_message_config.FormatMetadata),
					get_message_config.WithMetadataHeaders("Subject", "From"),
				)
			},
			method:   http.MethodGet,
			path:     "/gmail/v1/users/me/threads/t1",
			query:    url.Values{"format": {"metadata"}, "metadataHeaders": {"Subject", "From"}},
			response: `{"id":"t1","messages":[{"id":"m1"},{"id":"m2"}]}`,
			want:     `{"id":"t1","messages":[{"id":"m1"},{"id":"m2"}]}`,
		},
		{
			name: "trash thread",
			call: func(client *Client) (any, error) {
				return client.TrashThread(context.Background(), "me", "t1")
			},
			method:   http.MethodPost,
			path:     "/gmail/v1/users/me/threads/t1/trash",
			response: `{"id":"t1"}`,
			want:     `{"id":"t1"}`,
		},
		{
			name: "untrash thread",
			call: func(client *Client) (any, error) {
				return client.UntrashThread(context.Background(), "me", "t1")
			},
			method:   http.MethodPost,
			path:     "/gmail/v1/users/me/threads/t1/untrash",
			response: `{"id":"t1"}`,
			want:     `{"id":"t1"}`,
		},
		{
			name: "untrash message",
			call: func(client *Client) (any, error) {
				return client.Untrash(context.Background(), "me", "m1")
			},
			method:   http.MethodPost,
			path:     "/gmail/v1/users/me/messages/m1/untrash",
			response: `{"id":"m1"}`,
			want:     `{"id":"m1"}`,
		},
		{
			name: "get attachment",
			call: func(client *Client) (any, error) {
				return client.GetAttachment(context.Background(), "me", "m1", "ANGjdJ-x_y")
			},
			method:   http.MethodGet,
			path:     "/gmail/v1/users/me/messages/m1/attachments/ANGjdJ-x_y",
			response: `{"size":5,"data":"aGVsbG8"}`,
			want:     `{"size":5,"data":"aGVsbG8"}`,
		},
		{
			name: "create draft",
			call: func(client *Client) (any, error) {
				return client.CreateDraft(
					context.Background(),
					"me",
					&draft.Draft{Message: &message.Message{Raw: "cmF3", ThreadId: "t1"}},
				)
			},
			method:   http.MethodPost,
			path:     "/gmail/v1/users/me/drafts",
			body:     `{"message":{"threadId":"t1","raw":"cmF3"}}`,
			response: `{"id":"r1","message":{"id":"m1","threadId":"t1"}}`,
			want:     `{"id":"r1","message":{"id":"m1","threadId":"t1"}}`,
		},
		{
			name: "get draft",
			call: func(client *Client) (any, error) {
				return client.GetDraft(
					context.Background(),
					"me",
					"r1",
					get_message_config.WithFormat(get_message_config.FormatFull),
				)
			},
			method:   http.MethodGet,
			path:     "/gmail/v1/users/me/drafts/r1",
			query:    url.Values{"format": {"full"}},
			response: `{"id":"r1","message":{"id":"m1"}}`,
			want:     `{"id":"r1","message":{"id":"m1"}}`,
		},
		{
			name: "list drafts page",
			call: func(client *Client) (any, error) {
				return client.ListDraftsPage(
					context.Background(),
					"me",
					list_drafts_config.WithQuery("subject:x"),
					list_drafts_config.WithMaxResults(5),
					list_drafts_config.WithPageToken("p"),
				)
			},
			method:   http.MethodGet,
			path:     "/gmail/v1/users/me/drafts",
			query:    url.Values{"q": {"subject:x"}, "maxResults": {"5"}, "pageToken": {"p"}},
			response: `{"drafts":[{"id":"r1","message":{"id":"m1"}}],"nextPageToken":"q"}`,
			want:     `{"Items":[{"id":"r1","message":{"id":"m1"}}],"NextPageToken":"q","ResultSizeEstimate":0}`,
		},
		{
			name: "update draft",
			call: func(client *Client) (any, error) {
				return client.UpdateDraft(context.Background(), "me", "r1", &draft.Draft{Message: &message.Message{Raw: "bmV3"}})
			},
			method:   http.MethodPut,
			path:     "/gmail/v1/users/me/drafts/r1",
			body:     `{"message":{"raw":"bmV3"}}`,
			response: `{"id":"r1","message":{"id":"m2"}}`,
			want:     `{"id":"r1","message":{"id":"m2"}}`,
		},
		{
			name: "delete draft",
			call: func(client *Client) (any, error) {
				return nil, client.DeleteDraft(context.Background(), "me", "r1")
			},
			method: http.MethodDelete,
			path:   "/gmail/v1/users/me/drafts/r1",
		},
		{
			name: "send draft",
			call: func(client *Client) (any, error) {
				return client.SendDraft(context.Background(), "me", "r1")
			},
			method:   http.MethodPost,
			path:     "/gmail/v1/users/me/drafts/send",
			body:     `{"id":"r1"}`,
			response: `{"id":"m3","labelIds":["SENT"]}`,
			want:     `{"id":"m3","labelIds":["SENT"]}`,
		},
		{
			name: "get label",
			call: func(client *Client) (any, error) {
				return client.GetLabel(context.Background(), "me", "Label_1")
			},
			method:   http.MethodGet,
			path:     "/gmail/v1/users/me/labels/Label_1",
			response: `{"id":"Label_1","name":"Kvitton","messagesTotal":12,"threadsUnread":2}`,
			want:     `{"id":"Label_1","name":"Kvitton","messagesTotal":12,"threadsUnread":2}`,
		},
		{
			name: "update label",
			call: func(client *Client) (any, error) {
				return client.UpdateLabel(context.Background(), "me", "Label_1", &label.Label{Name: "Receipts"})
			},
			method:   http.MethodPatch,
			path:     "/gmail/v1/users/me/labels/Label_1",
			body:     `{"name":"Receipts"}`,
			response: `{"id":"Label_1","name":"Receipts"}`,
			want:     `{"id":"Label_1","name":"Receipts"}`,
		},
		{
			name: "delete label",
			call: func(client *Client) (any, error) {
				return nil, client.DeleteLabel(context.Background(), "me", "Label_1")
			},
			method: http.MethodDelete,
			path:   "/gmail/v1/users/me/labels/Label_1",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != testCase.method {
					t.Errorf("method = %s, want %s", r.Method, testCase.method)
				}
				if r.URL.EscapedPath() != testCase.path {
					t.Errorf("path = %s, want %s", r.URL.EscapedPath(), testCase.path)
				}
				query := r.URL.Query()
				if len(query) != len(testCase.query) {
					t.Errorf("query = %v, want %v", query, testCase.query)
				}
				for name, want := range testCase.query {
					if got := query[name]; strings.Join(got, ",") != strings.Join(want, ",") {
						t.Errorf("query %s = %v, want %v", name, got, want)
					}
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
				}
				if testCase.body != "" && strings.TrimSpace(string(body)) != testCase.body {
					t.Errorf("body = %s, want %s", body, testCase.body)
				}
				if testCase.response == "" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, testCase.response); err != nil {
					t.Errorf("write: %v", err)
				}
			})

			result, err := testCase.call(client)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if testCase.want == "" {
				return
			}
			got, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("marshal result: %v", err)
			}
			if string(got) != testCase.want {
				t.Errorf("result = %s, want %s", got, testCase.want)
			}
		})
	}
}

// TestArgumentErrors checks that a call missing what names its target is refused before anything is asked of Gmail.
func TestArgumentErrors(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	ctx := context.Background()
	saved := &draft.Draft{Message: &message.Message{Raw: "cmF3"}}

	testCases := []struct {
		name string
		call func() error
	}{
		{name: "list messages page, empty user", call: func() error { _, err := client.ListMessagesPage(ctx, ""); return err }},
		{name: "list threads page, empty user", call: func() error { _, err := client.ListThreadsPage(ctx, ""); return err }},
		{name: "get thread, empty user", call: func() error { _, err := client.GetThread(ctx, "", "t1"); return err }},
		{name: "get thread, empty thread", call: func() error { _, err := client.GetThread(ctx, "me", ""); return err }},
		{name: "trash thread, empty thread", call: func() error { _, err := client.TrashThread(ctx, "me", ""); return err }},
		{name: "untrash thread, empty user", call: func() error { _, err := client.UntrashThread(ctx, "", "t1"); return err }},
		{name: "untrash, empty message", call: func() error { _, err := client.Untrash(ctx, "me", ""); return err }},
		{name: "get attachment, empty message", call: func() error { _, err := client.GetAttachment(ctx, "me", "", "a"); return err }},
		{name: "get attachment, empty attachment", call: func() error { _, err := client.GetAttachment(ctx, "me", "m", ""); return err }},
		{name: "create draft, nil draft", call: func() error { _, err := client.CreateDraft(ctx, "me", nil); return err }},
		{name: "create draft, nil message", call: func() error { _, err := client.CreateDraft(ctx, "me", &draft.Draft{}); return err }},
		{
			name: "create draft, empty raw",
			call: func() error {
				_, err := client.CreateDraft(ctx, "me", &draft.Draft{Message: &message.Message{ThreadId: "t1"}})
				return err
			},
		},
		{name: "create draft, empty user", call: func() error { _, err := client.CreateDraft(ctx, "", saved); return err }},
		{name: "get draft, empty draft", call: func() error { _, err := client.GetDraft(ctx, "me", ""); return err }},
		{name: "list drafts page, empty user", call: func() error { _, err := client.ListDraftsPage(ctx, ""); return err }},
		{name: "update draft, empty draft", call: func() error { _, err := client.UpdateDraft(ctx, "me", "", saved); return err }},
		{name: "update draft, nil draft", call: func() error { _, err := client.UpdateDraft(ctx, "me", "r1", nil); return err }},
		{name: "delete draft, empty draft", call: func() error { return client.DeleteDraft(ctx, "me", "") }},
		{name: "send draft, empty draft", call: func() error { _, err := client.SendDraft(ctx, "me", ""); return err }},
		{name: "get label, empty label", call: func() error { _, err := client.GetLabel(ctx, "me", ""); return err }},
		{name: "update label, nil label", call: func() error { _, err := client.UpdateLabel(ctx, "me", "Label_1", nil); return err }},
		{name: "delete label, empty user", call: func() error { return client.DeleteLabel(ctx, "", "Label_1") }},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if err := testCase.call(); err == nil {
				t.Errorf("%s: expected an error", testCase.name)
			}
		})
	}
}
