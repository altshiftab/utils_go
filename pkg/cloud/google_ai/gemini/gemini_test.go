package gemini

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/gemini_config"
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/candidate"
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/content"
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/generate_content_request"
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/generate_content_response"
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/generation_config"
	"github.com/altshiftab/utils_go/pkg/cloud/google_ai/gemini/types/part"
)

func testServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return NewClient(gemini_config.WithBaseUrl(u), gemini_config.WithApiKey("test-key"))
}

func TestGenerateContent(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1beta/models/gemini-2.5-flash:generateContent" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if apiKey := r.Header.Get("x-goog-api-key"); apiKey != "test-key" {
			t.Errorf("expected api key 'test-key', got %q", apiKey)
		}

		var input generate_content_request.GenerateContentRequest
		if err := json.UnmarshalRead(r.Body, &input); err != nil {
			t.Errorf("unmarshal: %v", err)
		}

		if len(input.Contents) != 1 || len(input.Contents[0].Parts) != 1 {
			t.Fatalf("unexpected contents shape: %+v", input.Contents)
		}
		if text := input.Contents[0].Parts[0].Text; text != "Hello" {
			t.Errorf("expected text 'Hello', got %q", text)
		}
		if input.GenerationConfig == nil || input.GenerationConfig.ResponseMimeType != "application/json" {
			t.Errorf("expected response mime type 'application/json', got %+v", input.GenerationConfig)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &generate_content_response.GenerateContentResponse{
			Candidates: []*candidate.Candidate{
				{
					Content: &content.Content{
						Role:  "model",
						Parts: []*part.Part{{Text: `{"ads":[]}`}},
					},
					FinishReason: "STOP",
				},
			},
		}); err != nil {
			t.Errorf("marshal: %v", err)
		}
	})

	response, err := client.GenerateContent(
		context.Background(),
		"gemini-2.5-flash",
		&generate_content_request.GenerateContentRequest{
			Contents: []*content.Content{content.NewText("user", "Hello")},
			GenerationConfig: &generation_config.GenerationConfig{
				ResponseMimeType: "application/json",
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text := response.Text(); text != `{"ads":[]}` {
		t.Errorf("expected text '{\"ads\":[]}', got %q", text)
	}
}

func TestGenerateContent_EmptyModel(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.GenerateContent(
		context.Background(),
		"",
		&generate_content_request.GenerateContentRequest{},
	)
	if err == nil {
		t.Fatal("expected error for empty model")
	}
}

func TestGenerateContent_NilRequest(t *testing.T) {
	t.Parallel()

	client := NewClient()
	response, err := client.GenerateContent(context.Background(), "gemini-2.5-flash", nil)
	if err == nil {
		t.Fatal("expected an error for nil input")
	}
	if response != nil {
		t.Error("expected nil for nil request")
	}
}

func TestGenerateContent_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.GenerateContent(ctx, "gemini-2.5-flash", &generate_content_request.GenerateContentRequest{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// The fetch helpers marshal request bodies with encoding/json/v2, where omitempty no
// longer omits zero numbers; unset numeric fields must not reach the API, which
// rejects zero values (e.g. candidateCount must be in [1, 8]).
func TestGenerationConfig_MarshalOmitsZeroFields(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(&generation_config.GenerationConfig{ResponseMimeType: "application/json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expected := `{"responseMimeType":"application/json"}`; string(data) != expected {
		t.Errorf("expected %s, got %s", expected, data)
	}
}

func TestText_SkipsThoughtParts(t *testing.T) {
	t.Parallel()

	response := &generate_content_response.GenerateContentResponse{
		Candidates: []*candidate.Candidate{
			{
				Content: &content.Content{
					Parts: []*part.Part{
						{Text: "reasoning...", Thought: true},
						{Text: "answer"},
					},
				},
			},
		},
	}
	if text := response.Text(); text != "answer" {
		t.Errorf("expected text 'answer', got %q", text)
	}
}
