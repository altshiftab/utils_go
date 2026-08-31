package pubsub_test

import (
	"encoding/base64"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/pubsub_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/types/acknowledge_request"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/types/message"
)

// clientFor points a client at a test server standing in for the API.
func clientFor(t *testing.T, server *httptest.Server) *pubsub.Client {
	t.Helper()

	baseUrl, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}

	return pubsub.NewClient(pubsub_config.WithBaseUrl(baseUrl))
}

func TestPull(t *testing.T) {
	t.Parallel()

	const (
		project      = "test-proj"
		subscription = "test-sub"
	)
	payload := []byte(`{"logName":"projects/test-proj/logs/run.googleapis.com"}`)

	var gotMethod, gotPath string
	var gotMaxMessages int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path

		body, _ := io.ReadAll(r.Body)
		var request struct {
			MaxMessages int `json:"maxMessages"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("unmarshal request: %v", err)
		}
		gotMaxMessages = request.MaxMessages

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"receivedMessages":[{"ackId":"ack-1","message":{"messageId":"m-1","data":"` +
			base64.StdEncoding.EncodeToString(payload) + `"}}]}`))
	}))
	defer server.Close()

	response, err := clientFor(t, server).Pull(t.Context(), project, subscription, 16)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method: got %q, want POST", gotMethod)
	}
	if want := "/v1/projects/test-proj/subscriptions/test-sub:pull"; gotPath != want {
		t.Errorf("path: got %q, want %q", gotPath, want)
	}
	if gotMaxMessages != 16 {
		t.Errorf("max messages: got %d, want 16", gotMaxMessages)
	}
	if len(response.ReceivedMessages) != 1 {
		t.Fatalf("received messages: got %d, want 1", len(response.ReceivedMessages))
	}

	received := response.ReceivedMessages[0]
	if received.AckId != "ack-1" {
		t.Errorf("ack id: got %q, want %q", received.AckId, "ack-1")
	}

	// The payload is what a caller acts on, so it is read rather than compared
	// by identity: a message whose data does not decode is useless whatever the
	// struct looks like.
	decoded, err := received.Message.Payload()
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	if string(decoded) != string(payload) {
		t.Errorf("payload: got %q, want %q", decoded, payload)
	}
}

// TestPullAcknowledgesNothing is the property the log poller depends on: a pull
// leaves every message outstanding, so a consumer that dies before indexing sees
// them again rather than losing them.
func TestPullAcknowledgesNothing(t *testing.T) {
	t.Parallel()

	var acknowledged bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/projects/p/subscriptions/s:acknowledge" {
			acknowledged = true
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"receivedMessages":[{"ackId":"ack-1","message":{"messageId":"m-1","data":"aGk="}}]}`))
	}))
	defer server.Close()

	if _, err := clientFor(t, server).Pull(t.Context(), "p", "s", 1); err != nil {
		t.Fatalf("pull: %v", err)
	}

	if acknowledged {
		t.Error("a pull acknowledged the messages it returned")
	}
}

func TestPullEmpty(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	response, err := clientFor(t, server).Pull(t.Context(), "p", "s", 1)
	if err != nil {
		t.Fatalf("an idle subscription is not an error: %v", err)
	}
	if len(response.ReceivedMessages) != 0 {
		t.Errorf("received messages: got %d, want 0", len(response.ReceivedMessages))
	}
}

func TestPullArgumentErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	// The subtests run in parallel, so the parent returns before they do; a
	// deferred close would shut the server while they were still using it.
	t.Cleanup(server.Close)

	client := clientFor(t, server)

	testCases := []struct {
		name         string
		project      string
		subscription string
		maxMessages  int
	}{
		{name: "no project", project: "", subscription: "s", maxMessages: 1},
		{name: "no subscription", project: "p", subscription: "", maxMessages: 1},
		{name: "zero max messages", project: "p", subscription: "s", maxMessages: 0},
		{name: "negative max messages", project: "p", subscription: "s", maxMessages: -1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := client.Pull(
				t.Context(), testCase.project, testCase.subscription, testCase.maxMessages,
			); err == nil {
				t.Errorf("%s: expected an error, got none", testCase.name)
			}
		})
	}
}

func TestAcknowledge(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotAckIds []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		body, _ := io.ReadAll(r.Body)
		var request acknowledge_request.Request
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("unmarshal request: %v", err)
		}
		gotAckIds = request.AckIds

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	if err := clientFor(t, server).Acknowledge(t.Context(), "p", "s", []string{"a", "b"}); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}

	if want := "/v1/projects/p/subscriptions/s:acknowledge"; gotPath != want {
		t.Errorf("path: got %q, want %q", gotPath, want)
	}
	if len(gotAckIds) != 2 || gotAckIds[0] != "a" || gotAckIds[1] != "b" {
		t.Errorf("ack ids: got %v, want [a b]", gotAckIds)
	}
}

// TestAcknowledgeNothing checks that an empty batch is not a request. A consumer
// whose pull came back idle would otherwise call the API once per tick forever.
func TestAcknowledgeNothing(t *testing.T) {
	t.Parallel()

	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	if err := clientFor(t, server).Acknowledge(t.Context(), "p", "s", nil); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	if called {
		t.Error("acknowledging nothing made a request")
	}
}

func TestMessagePayload(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		message   *message.Message
		want      string
		wantError bool
	}{
		{name: "nil message", message: nil, want: ""},
		{name: "no data", message: &message.Message{MessageId: "m-1"}, want: ""},
		{name: "decodes", message: &message.Message{Data: "aGVsbG8="}, want: "hello"},
		{name: "not base64", message: &message.Message{Data: "!!!not base64!!!"}, wantError: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			payload, err := testCase.message.Payload()

			if testCase.wantError {
				if err == nil {
					t.Fatalf("%s: expected an error, got none", testCase.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", testCase.name, err)
			}
			if string(payload) != testCase.want {
				t.Errorf("%s: got %q, want %q", testCase.name, payload, testCase.want)
			}
		})
	}
}
