package gmail

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/get_message_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/gmail_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/list_history_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/list_messages_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/filter"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/send_as"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/watch_request"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/watch_response"
)

func testServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return NewClient(gmail_config.WithBaseUrl(u))
}

func TestSend(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/me/messages/send") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var input message.Message
		if err := json.UnmarshalRead(r.Body, &input); err != nil {
			t.Errorf("unmarshal: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &message.Message{
			Id:       "msg-123",
			ThreadId: "thread-456",
			LabelIds: []string{"SENT"},
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	msg, err := client.Send(context.Background(), "me", &message.Message{
		Raw: "dGVzdA==",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Id != "msg-123" {
		t.Errorf("expected id 'msg-123', got %q", msg.Id)
	}
	if msg.ThreadId != "thread-456" {
		t.Errorf("expected thread id 'thread-456', got %q", msg.ThreadId)
	}
}

func TestSend_EmptyUserId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.Send(context.Background(), "", &message.Message{Raw: "dGVzdA=="})
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestSend_NilMessage(t *testing.T) {
	t.Parallel()

	client := NewClient()
	msg, err := client.Send(context.Background(), "me", nil)
	if err == nil {
		t.Fatal("expected an error for nil input")
	}
	if msg != nil {
		t.Error("expected nil for nil message")
	}
}

func TestSend_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Send(ctx, "me", &message.Message{Raw: "dGVzdA=="})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestWatch(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/me/watch") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var input watch_request.WatchRequest
		if err := json.UnmarshalRead(r.Body, &input); err != nil {
			t.Errorf("unmarshal: %v", err)
		}

		if input.TopicName != "projects/my-project/topics/my-topic" {
			t.Errorf("expected topic 'projects/my-project/topics/my-topic', got %q", input.TopicName)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &watch_response.WatchResponse{
			HistoryId:  "12345",
			Expiration: "1431990098200",
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	resp, err := client.Watch(context.Background(), "me", &watch_request.WatchRequest{
		TopicName: "projects/my-project/topics/my-topic",
		LabelIds:  []string{"INBOX"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.HistoryId != "12345" {
		t.Errorf("expected history id '12345', got %q", resp.HistoryId)
	}
	if resp.Expiration != "1431990098200" {
		t.Errorf("expected expiration '1431990098200', got %q", resp.Expiration)
	}
}

func TestWatch_EmptyUserId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.Watch(context.Background(), "", &watch_request.WatchRequest{
		TopicName: "projects/my-project/topics/my-topic",
	})
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestWatch_NilRequest(t *testing.T) {
	t.Parallel()

	client := NewClient()
	resp, err := client.Watch(context.Background(), "me", nil)
	if err == nil {
		t.Fatal("expected an error for nil input")
	}
	if resp != nil {
		t.Error("expected nil for nil request")
	}
}

func TestWatch_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Watch(ctx, "me", &watch_request.WatchRequest{
		TopicName: "projects/my-project/topics/my-topic",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestWatchUrl(t *testing.T) {
	t.Parallel()

	u, _ := url.Parse("http://localhost:8080")
	client := NewClient(gmail_config.WithBaseUrl(u))
	got := client.watchUrl("user@example.com")
	expected := "http://localhost:8080/gmail/v1/users/user@example.com/watch"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestListHistory(t *testing.T) {
	t.Parallel()

	callCount := 0
	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/me/history") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("startHistoryId") != "12345" {
			t.Errorf("expected startHistoryId=12345, got %q", r.URL.Query().Get("startHistoryId"))
		}

		w.Header().Set("Content-Type", "application/json")
		callCount++
		if callCount == 1 {
			if err := json.MarshalWrite(w, map[string]any{
				"history": []map[string]any{
					{
						"id": "12346",
						"messagesAdded": []map[string]any{
							{"message": map[string]string{"id": "msg-1", "threadId": "thread-1"}},
						},
					},
				},
				"nextPageToken": "token-abc",
				"historyId":     "12350",
			}); err != nil {
				t.Errorf("marshal: %v", err)
			}
		} else {
			if err := json.MarshalWrite(w, map[string]any{
				"history": []map[string]any{
					{
						"id": "12347",
						"messagesAdded": []map[string]any{
							{"message": map[string]string{"id": "msg-2", "threadId": "thread-2"}},
						},
					},
				},
				"historyId": "12350",
			}); err != nil {
				t.Errorf("marshal: %v", err)
			}
		}
	})

	records, err := client.ListHistory(context.Background(), "me", "12345",
		list_history_config.WithHistoryTypes(list_history_config.HistoryTypeMessageAdded),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].Id != "12346" {
		t.Errorf("expected first record id '12346', got %q", records[0].Id)
	}
	if len(records[0].MessagesAdded) != 1 || records[0].MessagesAdded[0].Message.Id != "msg-1" {
		t.Errorf("unexpected first record messagesAdded")
	}
	if records[1].MessagesAdded[0].Message.Id != "msg-2" {
		t.Errorf("expected second message id 'msg-2', got %q", records[1].MessagesAdded[0].Message.Id)
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls for pagination, got %d", callCount)
	}
}

func TestListHistory_WithHistoryTypesAndLabelId(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		historyTypes := r.URL.Query()["historyTypes"]
		if len(historyTypes) != 2 || historyTypes[0] != "messageAdded" || historyTypes[1] != "labelAdded" {
			t.Errorf("expected historyTypes=[messageAdded, labelAdded], got %v", historyTypes)
		}
		if r.URL.Query().Get("labelId") != "INBOX" {
			t.Errorf("expected labelId=INBOX, got %q", r.URL.Query().Get("labelId"))
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{
			"history":   []map[string]any{},
			"historyId": "12345",
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	_, err := client.ListHistory(context.Background(), "me", "12345",
		list_history_config.WithHistoryTypes(list_history_config.HistoryTypeMessageAdded, list_history_config.HistoryTypeLabelAdded),
		list_history_config.WithLabelId("INBOX"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListHistory_EmptyUserId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.ListHistory(context.Background(), "", "12345")
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestListHistory_EmptyStartHistoryId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.ListHistory(context.Background(), "me", "")
	if err == nil {
		t.Fatal("expected error for empty start history id")
	}
}

func TestListHistory_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.ListHistory(ctx, "me", "12345")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestHistoryUrl(t *testing.T) {
	t.Parallel()

	u, _ := url.Parse("http://localhost:8080")
	client := NewClient(gmail_config.WithBaseUrl(u))
	got := client.historyUrl("user@example.com")
	expected := "http://localhost:8080/gmail/v1/users/user@example.com/history"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestSendUrl(t *testing.T) {
	t.Parallel()

	u, _ := url.Parse("http://localhost:8080")
	client := NewClient(gmail_config.WithBaseUrl(u))
	got := client.sendUrl("user@example.com")
	expected := "http://localhost:8080/gmail/v1/users/user@example.com/messages/send"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestCreateSendAs(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/me/settings/sendAs") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var input send_as.SendAs
		if err := json.UnmarshalRead(r.Body, &input); err != nil {
			t.Errorf("unmarshal: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &send_as.SendAs{
			SendAsEmail:        input.SendAsEmail,
			DisplayName:        input.DisplayName,
			TreatAsAlias:       true,
			VerificationStatus: "accepted",
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	created, err := client.CreateSendAs(context.Background(), "me", &send_as.SendAs{
		SendAsEmail: "support@example.com",
		DisplayName: "Support",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created.SendAsEmail != "support@example.com" {
		t.Errorf("expected email 'support@example.com', got %q", created.SendAsEmail)
	}
	if created.VerificationStatus != "accepted" {
		t.Errorf("expected status 'accepted', got %q", created.VerificationStatus)
	}
}

func TestCreateSendAs_EmptyUserId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.CreateSendAs(context.Background(), "", &send_as.SendAs{SendAsEmail: "a@b.com"})
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestCreateSendAs_NilSendAs(t *testing.T) {
	t.Parallel()

	client := NewClient()
	result, err := client.CreateSendAs(context.Background(), "me", nil)
	if err == nil {
		t.Fatal("expected an error for nil input")
	}
	if result != nil {
		t.Error("expected nil for nil send-as")
	}
}

func TestCreateSendAs_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.CreateSendAs(ctx, "me", &send_as.SendAs{SendAsEmail: "a@b.com"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGetSendAs(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/me/settings/sendAs/support@example.com") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.MarshalWrite(
			w,
			&send_as.SendAs{
				SendAsEmail:        "support@example.com",
				DisplayName:        "Support",
				VerificationStatus: "accepted",
			},
		)
		if err != nil {
			t.Fatalf("json marshal write: %v", err)
		}
	})

	s, err := client.GetSendAs(context.Background(), "me", "support@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.SendAsEmail != "support@example.com" {
		t.Errorf("expected email 'support@example.com', got %q", s.SendAsEmail)
	}
}

func TestGetSendAs_EmptyUserId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.GetSendAs(context.Background(), "", "a@b.com")
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestGetSendAs_EmptySendAsEmail(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.GetSendAs(context.Background(), "me", "")
	if err == nil {
		t.Fatal("expected error for empty send-as email")
	}
}

func TestUpdateSendAs(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/me/settings/sendAs/support@example.com") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var input send_as.SendAs
		if err := json.UnmarshalRead(r.Body, &input); err != nil {
			t.Errorf("unmarshal: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &send_as.SendAs{
			SendAsEmail: "support@example.com",
			DisplayName: input.DisplayName,
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	updated, err := client.UpdateSendAs(context.Background(), "me", "support@example.com", &send_as.SendAs{
		DisplayName: "New Support Name",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.DisplayName != "New Support Name" {
		t.Errorf("expected display name 'New Support Name', got %q", updated.DisplayName)
	}
}

func TestUpdateSendAs_EmptySendAsEmail(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.UpdateSendAs(context.Background(), "me", "", &send_as.SendAs{})
	if err == nil {
		t.Fatal("expected error for empty send-as email")
	}
}

func TestUpdateSendAs_NilSendAs(t *testing.T) {
	t.Parallel()

	client := NewClient()
	result, err := client.UpdateSendAs(context.Background(), "me", "a@b.com", nil)
	if err == nil {
		t.Fatal("expected an error for nil input")
	}
	if result != nil {
		t.Error("expected nil for nil send-as")
	}
}

func TestDeleteSendAs(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/me/settings/sendAs/support@example.com") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	err := client.DeleteSendAs(context.Background(), "me", "support@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteSendAs_EmptyUserId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	err := client.DeleteSendAs(context.Background(), "", "a@b.com")
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestDeleteSendAs_EmptySendAsEmail(t *testing.T) {
	t.Parallel()

	client := NewClient()
	err := client.DeleteSendAs(context.Background(), "me", "")
	if err == nil {
		t.Fatal("expected error for empty send-as email")
	}
}

func TestListMessages(t *testing.T) {
	t.Parallel()

	callCount := 0
	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/me/messages") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("q") != "in:inbox" {
			t.Errorf("expected q=in:inbox, got %q", r.URL.Query().Get("q"))
		}

		w.Header().Set("Content-Type", "application/json")
		callCount++
		if callCount == 1 {
			if err := json.MarshalWrite(w, map[string]any{
				"messages": []map[string]string{
					{"id": "msg-1", "threadId": "thread-1"},
					{"id": "msg-2", "threadId": "thread-2"},
				},
				"nextPageToken":      "token-abc",
				"resultSizeEstimate": 3,
			}); err != nil {
				t.Errorf("marshal: %v", err)
			}
		} else {
			if err := json.MarshalWrite(w, map[string]any{
				"messages": []map[string]string{
					{"id": "msg-3", "threadId": "thread-3"},
				},
				"resultSizeEstimate": 3,
			}); err != nil {
				t.Errorf("marshal: %v", err)
			}
		}
	})

	messages, err := client.ListMessages(context.Background(), "me", list_messages_config.WithQuery("in:inbox"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	if messages[0].Id != "msg-1" {
		t.Errorf("expected first message id 'msg-1', got %q", messages[0].Id)
	}
	if messages[2].Id != "msg-3" {
		t.Errorf("expected third message id 'msg-3', got %q", messages[2].Id)
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls for pagination, got %d", callCount)
	}
}

func TestListMessages_EmptyUserId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.ListMessages(context.Background(), "", list_messages_config.WithQuery("in:inbox"))
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestListMessages_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.ListMessages(ctx, "me", list_messages_config.WithQuery("in:inbox"))
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestListMessages_EmptyQuery(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "" {
			t.Errorf("expected no q parameter, got %q", r.URL.Query().Get("q"))
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{
			"messages": []map[string]string{
				{"id": "msg-1", "threadId": "thread-1"},
			},
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	messages, err := client.ListMessages(context.Background(), "me")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
}

// The options the config exists for: what a caller taking stock of a whole
// mailbox needs, and what the bare query string could not express.
func TestListMessages_Options(t *testing.T) {
	t.Parallel()

	var got url.Values

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{
			"messages": []map[string]string{{"id": "msg-1"}},
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	if _, err := client.ListMessages(
		context.Background(),
		"me",
		list_messages_config.WithQuery("after:1756382400"),
		list_messages_config.WithIncludeSpamTrash(true),
		list_messages_config.WithMaxResults(500),
		list_messages_config.WithLabelIds("INBOX", "UNREAD"),
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Get("q") != "after:1756382400" {
		t.Errorf("q = %q", got.Get("q"))
	}
	if got.Get("includeSpamTrash") != "true" {
		t.Errorf("includeSpamTrash = %q, want true", got.Get("includeSpamTrash"))
	}
	if got.Get("maxResults") != "500" {
		t.Errorf("maxResults = %q, want 500", got.Get("maxResults"))
	}
	if labelIds := got["labelIds"]; len(labelIds) != 2 || labelIds[0] != "INBOX" || labelIds[1] != "UNREAD" {
		t.Errorf("labelIds = %v, want [INBOX UNREAD]", labelIds)
	}
}

// What is not asked for is not sent, so a caller wanting Gmail's own defaults
// gets them.
func TestListMessages_UnsetOptionsAreAbsent(t *testing.T) {
	t.Parallel()

	var got url.Values

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{
			"messages": []map[string]string{{"id": "msg-1"}},
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	if _, err := client.ListMessages(context.Background(), "me"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, name := range []string{"q", "includeSpamTrash", "maxResults", "labelIds"} {
		if got.Has(name) {
			t.Errorf("%s = %q, want it absent", name, got.Get(name))
		}
	}
}

func TestGetMessage(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/me/messages/msg-123") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &message.Message{
			Id:           "msg-123",
			ThreadId:     "thread-456",
			LabelIds:     []string{"INBOX"},
			Snippet:      "Hello world",
			InternalDate: "1234567890000",
			SizeEstimate: 1024,
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	msg, err := client.GetMessage(context.Background(), "me", "msg-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Id != "msg-123" {
		t.Errorf("expected id 'msg-123', got %q", msg.Id)
	}
	if msg.ThreadId != "thread-456" {
		t.Errorf("expected thread id 'thread-456', got %q", msg.ThreadId)
	}
	if msg.Snippet != "Hello world" {
		t.Errorf("expected snippet 'Hello world', got %q", msg.Snippet)
	}
}

func TestGetMessage_WithFormatAndMetadataHeaders(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") != "metadata" {
			t.Errorf("expected format=metadata, got %q", r.URL.Query().Get("format"))
		}
		headers := r.URL.Query()["metadataHeaders"]
		if len(headers) != 2 || headers[0] != "Subject" || headers[1] != "From" {
			t.Errorf("expected metadataHeaders=[Subject, From], got %v", headers)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &message.Message{
			Id: "msg-123",
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	msg, err := client.GetMessage(
		context.Background(), "me", "msg-123",
		get_message_config.WithFormat(get_message_config.FormatMetadata),
		get_message_config.WithMetadataHeaders("Subject", "From"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Id != "msg-123" {
		t.Errorf("expected id 'msg-123', got %q", msg.Id)
	}
}

func TestGetMessage_EmptyUserId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.GetMessage(context.Background(), "", "msg-123")
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestGetMessage_EmptyMessageId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.GetMessage(context.Background(), "me", "")
	if err == nil {
		t.Fatal("expected error for empty message id")
	}
}

func TestGetMessage_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.GetMessage(ctx, "me", "msg-123")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestTrash(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/me/messages/msg-123/trash") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &message.Message{
			Id:       "msg-123",
			ThreadId: "thread-456",
			LabelIds: []string{"TRASH"},
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	msg, err := client.Trash(context.Background(), "me", "msg-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Id != "msg-123" {
		t.Errorf("expected id 'msg-123', got %q", msg.Id)
	}
	if msg.ThreadId != "thread-456" {
		t.Errorf("expected thread id 'thread-456', got %q", msg.ThreadId)
	}
	if len(msg.LabelIds) != 1 || msg.LabelIds[0] != "TRASH" {
		t.Errorf("expected label ids ['TRASH'], got %v", msg.LabelIds)
	}
}

func TestTrash_EmptyUserId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.Trash(context.Background(), "", "msg-123")
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestTrash_EmptyMessageId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.Trash(context.Background(), "me", "")
	if err == nil {
		t.Fatal("expected error for empty message id")
	}
}

func TestTrash_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Trash(ctx, "me", "msg-123")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestMessagesUrl(t *testing.T) {
	t.Parallel()

	u, _ := url.Parse("http://localhost:8080")
	client := NewClient(gmail_config.WithBaseUrl(u))

	got := client.messagesUrl("user@example.com", "")
	expected := "http://localhost:8080/gmail/v1/users/user@example.com/messages"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}

	got = client.messagesUrl("user@example.com", "msg-123")
	expected = "http://localhost:8080/gmail/v1/users/user@example.com/messages/msg-123"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestSendAsUrl(t *testing.T) {
	t.Parallel()

	u, _ := url.Parse("http://localhost:8080")
	client := NewClient(gmail_config.WithBaseUrl(u))

	got := client.sendAsUrl("user@example.com", "")
	expected := "http://localhost:8080/gmail/v1/users/user@example.com/settings/sendAs"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}

	got = client.sendAsUrl("user@example.com", "alias@example.com")
	expected = "http://localhost:8080/gmail/v1/users/user@example.com/settings/sendAs/alias@example.com"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestCreateFilter(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/me/settings/filters") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var input filter.Filter
		if err := json.UnmarshalRead(r.Body, &input); err != nil {
			t.Errorf("unmarshal: %v", err)
		}

		if input.Criteria == nil || input.Criteria.From != "boss@example.com" {
			t.Errorf("unexpected criteria: %+v", input.Criteria)
		}
		if input.Action == nil || len(input.Action.AddLabelIds) != 1 || input.Action.AddLabelIds[0] != "Label_1" {
			t.Errorf("unexpected action: %+v", input.Action)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &filter.Filter{
			Id:       "filter-1",
			Criteria: input.Criteria,
			Action:   input.Action,
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	created, err := client.CreateFilter(context.Background(), "me", &filter.Filter{
		Criteria: &filter.Criteria{From: "boss@example.com"},
		Action:   &filter.Action{AddLabelIds: []string{"Label_1"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created.Id != "filter-1" {
		t.Errorf("expected id 'filter-1', got %q", created.Id)
	}
	if created.Criteria.From != "boss@example.com" {
		t.Errorf("expected criteria from 'boss@example.com', got %q", created.Criteria.From)
	}
}

func TestCreateFilter_EmptyUserId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.CreateFilter(context.Background(), "", &filter.Filter{})
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestCreateFilter_NilFilter(t *testing.T) {
	t.Parallel()

	client := NewClient()
	result, err := client.CreateFilter(context.Background(), "me", nil)
	if err == nil {
		t.Fatal("expected an error for nil input")
	}
	if result != nil {
		t.Error("expected nil for nil filter")
	}
}

func TestCreateFilter_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.CreateFilter(ctx, "me", &filter.Filter{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGetFilter(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/me/settings/filters/filter-1") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		err := json.MarshalWrite(w, &filter.Filter{
			Id:       "filter-1",
			Criteria: &filter.Criteria{From: "boss@example.com"},
			Action:   &filter.Action{AddLabelIds: []string{"Label_1"}},
		})
		if err != nil {
			t.Fatalf("json marshal write: %v", err)
		}
	})

	f, err := client.GetFilter(context.Background(), "me", "filter-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Id != "filter-1" {
		t.Errorf("expected id 'filter-1', got %q", f.Id)
	}
	if f.Criteria == nil || f.Criteria.From != "boss@example.com" {
		t.Errorf("unexpected criteria: %+v", f.Criteria)
	}
}

func TestGetFilter_EmptyUserId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.GetFilter(context.Background(), "", "filter-1")
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestGetFilter_EmptyFilterId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.GetFilter(context.Background(), "me", "")
	if err == nil {
		t.Fatal("expected error for empty filter id")
	}
}

func TestGetFilter_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.GetFilter(ctx, "me", "filter-1")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestListFilters(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/me/settings/filters") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{
			"filter": []map[string]any{
				{
					"id":       "filter-1",
					"criteria": map[string]string{"from": "a@example.com"},
					"action":   map[string]any{"addLabelIds": []string{"Label_1"}},
				},
				{
					"id":       "filter-2",
					"criteria": map[string]string{"subject": "Important"},
					"action":   map[string]any{"removeLabelIds": []string{"INBOX"}},
				},
			},
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	filters, err := client.ListFilters(context.Background(), "me")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(filters))
	}
	if filters[0].Id != "filter-1" {
		t.Errorf("expected first id 'filter-1', got %q", filters[0].Id)
	}
	if filters[1].Criteria == nil || filters[1].Criteria.Subject != "Important" {
		t.Errorf("unexpected second filter criteria: %+v", filters[1].Criteria)
	}
}

func TestListFilters_EmptyUserId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.ListFilters(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestListFilters_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.ListFilters(ctx, "me")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDeleteFilter(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/me/settings/filters/filter-1") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	err := client.DeleteFilter(context.Background(), "me", "filter-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteFilter_EmptyUserId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	err := client.DeleteFilter(context.Background(), "", "filter-1")
	if err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestDeleteFilter_EmptyFilterId(t *testing.T) {
	t.Parallel()

	client := NewClient()
	err := client.DeleteFilter(context.Background(), "me", "")
	if err == nil {
		t.Fatal("expected error for empty filter id")
	}
}

func TestFiltersUrl(t *testing.T) {
	t.Parallel()

	u, _ := url.Parse("http://localhost:8080")
	client := NewClient(gmail_config.WithBaseUrl(u))

	got := client.filtersUrl("user@example.com", "")
	expected := "http://localhost:8080/gmail/v1/users/user@example.com/settings/filters"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}

	got = client.filtersUrl("user@example.com", "filter-1")
	expected = "http://localhost:8080/gmail/v1/users/user@example.com/settings/filters/filter-1"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}
