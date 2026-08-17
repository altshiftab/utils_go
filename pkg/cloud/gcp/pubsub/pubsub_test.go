package pubsub_test

import (
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/pubsub_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/types/message"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/pubsub/types/publish_request"
)

func TestPublish(t *testing.T) {
	t.Parallel()

	const (
		project = "test-proj"
		topic   = "test-topic"
	)
	payload := []byte(`{"order_id":"abc"}`)

	var gotMethod, gotPath, gotData string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path

		body, _ := io.ReadAll(r.Body)
		var req publish_request.Request
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("unmarshal request: %v", err)
		}
		if len(req.Messages) == 1 {
			gotData = req.Messages[0].Data
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messageIds":["msg-1"]}`))
	}))
	defer server.Close()

	baseUrl, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}

	client := pubsub.NewClient(pubsub_config.WithBaseUrl(baseUrl))
	resp, err := client.Publish(
		context.Background(),
		project,
		topic,
		&publish_request.Request{Messages: []*message.Message{message.New(payload, map[string]string{"k": "v"})}},
	)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if want := "/v1/projects/test-proj/topics/test-topic:publish"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if want := base64.StdEncoding.EncodeToString(payload); gotData != want {
		t.Errorf("message data = %q, want base64 %q", gotData, want)
	}
	if len(resp.MessageIds) != 1 || resp.MessageIds[0] != "msg-1" {
		t.Errorf("messageIds = %v, want [msg-1]", resp.MessageIds)
	}
}
