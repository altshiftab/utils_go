package response

import (
	"bytes"
	"errors"
	"net/http"
	"testing"
)

var errStreamFailure = errors.New("stream failure")

func TestHeaderEntry_Fields(t *testing.T) {
	t.Parallel()

	entry := &HeaderEntry{Name: "Content-Type", Value: "application/json", Overwrite: true}

	if entry.Name != "Content-Type" {
		t.Errorf("Name = %q, want %q", entry.Name, "Content-Type")
	}
	if entry.Value != "application/json" {
		t.Errorf("Value = %q, want %q", entry.Value, "application/json")
	}
	if !entry.Overwrite {
		t.Error("Overwrite = false, want true")
	}
}

func TestHeaderEntry_ZeroValue(t *testing.T) {
	t.Parallel()

	var entry HeaderEntry
	if entry.Name != "" || entry.Value != "" || entry.Overwrite {
		t.Errorf("zero value HeaderEntry = %+v, want empty", entry)
	}
}

func TestResponse_Fields(t *testing.T) {
	t.Parallel()

	body := []byte("hello")
	response := &Response{
		StatusCode: http.StatusOK,
		Headers: []*HeaderEntry{
			{Name: "X-Test", Value: "1"},
		},
		Body:          body,
		SensitiveBody: true,
	}

	if response.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if len(response.Headers) != 1 || response.Headers[0].Name != "X-Test" {
		t.Errorf("Headers = %+v, unexpected", response.Headers)
	}
	if !bytes.Equal(response.Body, body) {
		t.Errorf("Body = %q, want %q", response.Body, body)
	}
	if !response.SensitiveBody {
		t.Error("SensitiveBody = false, want true")
	}
	if response.BodyStreamer != nil {
		t.Error("BodyStreamer = non-nil, want nil")
	}
}

func TestResponse_ZeroValue(t *testing.T) {
	t.Parallel()

	var response Response
	if response.StatusCode != 0 {
		t.Errorf("StatusCode = %d, want 0", response.StatusCode)
	}
	if response.Headers != nil {
		t.Error("Headers = non-nil, want nil")
	}
	if response.Body != nil {
		t.Error("Body = non-nil, want nil")
	}
	if response.BodyStreamer != nil {
		t.Error("BodyStreamer = non-nil, want nil")
	}
	if response.SensitiveBody {
		t.Error("SensitiveBody = true, want false")
	}
}

func TestResponse_BodyStreamerIteration(t *testing.T) {
	t.Parallel()

	chunks := [][]byte{[]byte("a"), []byte("bc"), []byte("def")}

	response := &Response{
		BodyStreamer: func(yield func([]byte, error) bool) {
			for _, chunk := range chunks {
				if !yield(chunk, nil) {
					return
				}
			}
		},
	}

	var collected [][]byte
	for chunk, err := range response.BodyStreamer {
		if err != nil {
			t.Fatalf("unexpected streamer error: %v", err)
		}
		collected = append(collected, chunk)
	}

	if len(collected) != len(chunks) {
		t.Fatalf("collected %d chunks, want %d", len(collected), len(chunks))
	}
	for i := range chunks {
		if !bytes.Equal(collected[i], chunks[i]) {
			t.Errorf("chunk %d = %q, want %q", i, collected[i], chunks[i])
		}
	}
}

func TestResponse_BodyStreamerPropagatesError(t *testing.T) {
	t.Parallel()

	response := &Response{
		BodyStreamer: func(yield func([]byte, error) bool) {
			yield(nil, errStreamFailure)
		},
	}

	var gotErr error
	for _, err := range response.BodyStreamer {
		if err != nil {
			gotErr = err
			break
		}
	}

	if !errors.Is(gotErr, errStreamFailure) {
		t.Errorf("streamer error = %v, want %v", gotErr, errStreamFailure)
	}
}

func TestResponse_BodyStreamerEarlyStop(t *testing.T) {
	t.Parallel()

	yielded := 0
	response := &Response{
		BodyStreamer: func(yield func([]byte, error) bool) {
			for i := range 5 {
				yielded++
				if !yield([]byte{byte(i)}, nil) {
					return
				}
			}
		},
	}

	count := 0
	for _, err := range response.BodyStreamer {
		if err != nil {
			t.Fatalf("chunk %d: %v", count, err)
		}
		count++
		if count == 2 {
			break
		}
	}

	if count != 2 {
		t.Errorf("consumed %d chunks, want 2", count)
	}
	if yielded != 2 {
		t.Errorf("streamer yielded %d times, want 2 (early stop should halt production)", yielded)
	}
}
