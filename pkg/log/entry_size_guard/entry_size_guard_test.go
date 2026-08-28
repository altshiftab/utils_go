package entry_size_guard

import (
	"bytes"
	"encoding/json/v2"
	"strings"
	"testing"
	"unicode/utf8"
)

func marshalEntry(t *testing.T, entry map[string]any) []byte {
	t.Helper()

	entryBytes, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}

	return append(entryBytes, '\n')
}

func TestWrite(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		entryLimit  int
		stringLimit int
		entry       map[string]any
		check       func(t *testing.T, written map[string]any)
	}{
		{
			name:        "oversized string is truncated",
			entryLimit:  256,
			stringLimit: 16,
			entry: map[string]any{
				"severity": "ERROR",
				"message":  "boom",
				"http": map[string]any{
					"request": map[string]any{
						"body": map[string]any{"content": strings.Repeat("a", 1024)},
					},
				},
			},
			check: func(t *testing.T, written map[string]any) {
				t.Helper()

				if written[TruncatedKey] != true {
					t.Errorf("missing %q marker: %v", TruncatedKey, written)
				}
				if written["message"] != "boom" {
					t.Errorf("message: got %v", written["message"])
				}

				content, _ := written["http"].(map[string]any)["request"].(map[string]any)["body"].(map[string]any)["content"].(string)
				if !strings.Contains(content, "[truncated, 1024 bytes in full]") {
					t.Errorf("content not truncated: %q", content)
				}
			},
		},
		{
			name:        "oversized string in an array is truncated",
			entryLimit:  256,
			stringLimit: 16,
			entry: map[string]any{
				"severity": "ERROR",
				"values":   []any{strings.Repeat("b", 512)},
			},
			check: func(t *testing.T, written map[string]any) {
				t.Helper()

				element, _ := written["values"].([]any)[0].(string)
				if !strings.Contains(element, "[truncated, 512 bytes in full]") {
					t.Errorf("array element not truncated: %q", element)
				}
			},
		},
		{
			name:        "multi-byte strings are cut on rune boundaries",
			entryLimit:  256,
			stringLimit: 17,
			entry: map[string]any{
				"severity": "ERROR",
				"message":  strings.Repeat("ä", 512),
			},
			check: func(t *testing.T, written map[string]any) {
				t.Helper()

				message, _ := written["message"].(string)
				if !utf8.ValidString(message) {
					t.Errorf("message is not valid utf-8: %q", message)
				}
			},
		},
		{
			name:        "unshrinkable entry falls back to the essentials",
			entryLimit:  512,
			stringLimit: 64,
			entry: func() map[string]any {
				entry := map[string]any{
					"time":     "2026-08-13T12:00:00Z",
					"severity": "ERROR",
					"message":  "A build submission failed.",
					"error": map[string]any{
						"message":     "submitter submit: boom",
						"stack_trace": strings.Repeat("s", 128),
					},
				}
				for i := range 64 {
					entry[strings.Repeat("k", 8)+string(rune('a'+i%26))+string(rune('a'+i/26))] = strings.Repeat("v", 32)
				}
				return entry
			}(),
			check: func(t *testing.T, written map[string]any) {
				t.Helper()

				if written[TruncatedKey] != true {
					t.Errorf("missing %q marker: %v", TruncatedKey, written)
				}
				if written["severity"] != "ERROR" || written["message"] != "A build submission failed." {
					t.Errorf("essentials missing: %v", written)
				}
				if errorMessage, _ := written["error"].(map[string]any)["message"].(string); errorMessage != "submitter submit: boom" {
					t.Errorf("error message: got %q", errorMessage)
				}
				if len(written) > 5 {
					t.Errorf("fallback entry carries extra fields: %v", written)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buffer bytes.Buffer
			writer := &Writer{Writer: &buffer, EntryLimit: testCase.entryLimit, StringLimit: testCase.stringLimit}

			entryBytes := marshalEntry(t, testCase.entry)

			n, err := writer.Write(entryBytes)
			if err != nil {
				t.Fatalf("write: %v", err)
			}
			if n != len(entryBytes) {
				t.Errorf("write length: got %d, want %d", n, len(entryBytes))
			}

			writtenBytes := buffer.Bytes()
			if len(writtenBytes) > testCase.entryLimit+1 {
				t.Errorf("written entry exceeds the limit: %d > %d", len(writtenBytes), testCase.entryLimit+1)
			}

			var written map[string]any
			if err := json.Unmarshal(writtenBytes, &written); err != nil {
				t.Fatalf("json unmarshal (written entry): %v", err)
			}

			testCase.check(t, written)
		})
	}
}

func TestWritePassthrough(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	writer := New(&buffer)

	entryBytes := marshalEntry(t, map[string]any{"severity": "INFO", "message": "ok"})

	n, err := writer.Write(entryBytes)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != len(entryBytes) {
		t.Errorf("write length: got %d, want %d", n, len(entryBytes))
	}

	if !bytes.Equal(buffer.Bytes(), entryBytes) {
		t.Errorf("entry was modified: %q", buffer.Bytes())
	}
}

func TestWriteNonJson(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	writer := &Writer{Writer: &buffer, EntryLimit: 64, StringLimit: 16}

	entry := []byte(strings.Repeat("x", 256) + "\n")

	n, err := writer.Write(entry)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != len(entry) {
		t.Errorf("write length: got %d, want %d", n, len(entry))
	}

	if written := buffer.Bytes(); len(written) > 65 {
		t.Errorf("written entry exceeds the limit: %d", len(written))
	}
}

func TestWriteNilWriter(t *testing.T) {
	t.Parallel()

	writer := &Writer{}

	if _, err := writer.Write([]byte("{}\n")); err == nil {
		t.Error("expected an error for a nil underlying writer")
	}
}

// TestDefaultEntryLimitFitsAgentCut guards the default against the cut that
// actually destroys entries. Cloud Logging's own ceiling is 256 KiB, but Cloud
// Run truncates a stdout line around 100 KB, so a default above that leaves
// entries the guard passes through to be mangled anyway.
func TestDefaultEntryLimitFitsAgentCut(t *testing.T) {
	t.Parallel()

	const observedAgentCut = 99678

	if DefaultEntryLimit >= observedAgentCut {
		t.Errorf(
			"default entry limit does not fit the agent's cut: %d >= %d",
			DefaultEntryLimit,
			observedAgentCut,
		)
	}
}

// TestWriteOversizedByDefaultFits checks the whole path at the shipped
// defaults: an entry that would be cut by the agent is reduced to one that is
// not, and stays parseable JSON rather than a truncated fragment.
func TestWriteOversizedByDefaultFits(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	writer := New(&buffer)

	entryBytes := marshalEntry(t, map[string]any{
		"severity": "DEBUG",
		"message":  "a fetch was performed",
		"http": map[string]any{
			"request":  map[string]any{"body": map[string]any{"content": strings.Repeat("a", 91453)}},
			"response": map[string]any{"status_code": float64(503)},
		},
	})

	if _, err := writer.Write(entryBytes); err != nil {
		t.Fatalf("write: %v", err)
	}

	written := buffer.Bytes()
	if len(written) > DefaultEntryLimit {
		t.Errorf("written entry exceeds the default limit: %d > %d", len(written), DefaultEntryLimit)
	}

	var parsed map[string]any
	if err := json.Unmarshal(written, &parsed); err != nil {
		t.Fatalf("json unmarshal (written entry): %v", err)
	}
	if parsed[TruncatedKey] != true {
		t.Errorf("missing %q marker: %v", TruncatedKey, parsed)
	}

	// The fields the truncation exists to save: what the entry was about, and
	// what the response said.
	httpValue, ok := parsed["http"].(map[string]any)
	if !ok {
		t.Fatalf("http field lost: %v", parsed)
	}
	response, ok := httpValue["response"].(map[string]any)
	if !ok {
		t.Fatalf("http response lost: %v", httpValue)
	}
	if response["status_code"] != float64(503) {
		t.Errorf("status code: got %v, want 503", response["status_code"])
	}
}
