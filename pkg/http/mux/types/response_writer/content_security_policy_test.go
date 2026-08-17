package response_writer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
)

const testInlineScriptHash = "sha256-L2121qypPdYD4EOJ6AR1Amd2YKHYClryjHjORJFpR7U="

func TestApplyInlineScriptHashes(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		policy        string
		hashes        []string
		expected      string
		expectedError bool
	}{
		{
			name:     "existing script-src is extended",
			policy:   "default-src 'self'; script-src 'self'",
			hashes:   []string{testInlineScriptHash},
			expected: "default-src 'self'; script-src 'self' '" + testInlineScriptHash + "'",
		},
		{
			name:     "missing script-src is seeded from default-src",
			policy:   "default-src 'self'; object-src 'none'",
			hashes:   []string{testInlineScriptHash},
			expected: "default-src 'self'; object-src 'none'; script-src 'self' '" + testInlineScriptHash + "'",
		},
		{
			name:     "policy without script restrictions is unchanged",
			policy:   "frame-ancestors 'none'",
			hashes:   []string{testInlineScriptHash},
			expected: "frame-ancestors 'none'",
		},
		{
			name:     "present hash is not duplicated",
			policy:   "script-src 'self' '" + testInlineScriptHash + "'",
			hashes:   []string{testInlineScriptHash},
			expected: "script-src 'self' '" + testInlineScriptHash + "'",
		},
		{
			name:          "invalid hash",
			policy:        "default-src 'self'",
			hashes:        []string{"nonsense"},
			expectedError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			actual, err := applyInlineScriptHashes(testCase.policy, testCase.hashes)

			if testCase.expectedError {
				if err == nil {
					t.Error("expected an error")
				}
				return
			}

			if err != nil {
				t.Fatalf("apply inline script hashes: %v", err)
			}
			if actual != testCase.expected {
				t.Errorf("expected %q, got %q", testCase.expected, actual)
			}
		})
	}
}

func TestWriteResponseInlineScriptHashes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		headers          []*muxTypesResponse.HeaderEntry
		expectedContains []string
	}{
		{
			name: "default document policy gains the hash",
			headers: []*muxTypesResponse.HeaderEntry{
				{Name: "Content-Type", Value: "text/html"},
			},
			expectedContains: []string{
				"object-src 'none'",
				"script-src 'self' '" + testInlineScriptHash + "'",
			},
		},
		{
			// A service-patched per-endpoint policy, as in the signals services.
			name: "patched overwrite policy keeps its patches and gains the hash",
			headers: []*muxTypesResponse.HeaderEntry{
				{Name: "Content-Type", Value: "text/html"},
				{
					Name:      "Content-Security-Policy",
					Value:     "default-src 'self'; connect-src 'self' https://login.example.com; object-src 'none'",
					Overwrite: true,
				},
			},
			expectedContains: []string{
				"connect-src 'self' https://login.example.com",
				"script-src 'self' '" + testInlineScriptHash + "'",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			responseWriter := &ResponseWriter{ResponseWriter: recorder}

			err := responseWriter.WriteResponse(
				context.Background(),
				&muxTypesResponse.Response{
					StatusCode:         http.StatusOK,
					Headers:            testCase.headers,
					Body:               []byte("<html></html>"),
					InlineScriptHashes: []string{testInlineScriptHash},
				},
				nil,
			)
			if err != nil {
				t.Fatalf("write response: %v", err)
			}

			policy := recorder.Header().Get("Content-Security-Policy")
			for _, expected := range testCase.expectedContains {
				if !strings.Contains(policy, expected) {
					t.Errorf("expected policy to contain %q, got %q", expected, policy)
				}
			}
		})
	}
}
