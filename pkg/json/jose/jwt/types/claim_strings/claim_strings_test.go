package claim_strings

import (
	"encoding/json/v2"
	"errors"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func TestClaimStrings_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       string
		expected    ClaimStrings
		expectError bool
	}{
		{
			name:     "single string",
			input:    `"audience1"`,
			expected: ClaimStrings{"audience1"},
		},
		{
			name:     "array of strings",
			input:    `["audience1", "audience2", "audience3"]`,
			expected: ClaimStrings{"audience1", "audience2", "audience3"},
		},
		{
			name:     "empty array",
			input:    `[]`,
			expected: ClaimStrings{},
		},
		{
			name:     "null value",
			input:    `null`,
			expected: nil,
		},
		{
			name:        "invalid json",
			input:       `{invalid`,
			expectError: true,
		},
		{
			name:        "array with non-string",
			input:       `["valid", 123]`,
			expectError: true,
		},
		{
			name:        "number instead of string",
			input:       `123`,
			expectError: true,
		},
		{
			name:        "object instead of string",
			input:       `{"key": "value"}`,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var cs ClaimStrings
			err := json.Unmarshal([]byte(tc.input), &cs)

			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if len(cs) != len(tc.expected) {
					t.Fatalf("expected length %d, got %d", len(tc.expected), len(cs))
				}

				for i, v := range tc.expected {
					if cs[i] != v {
						t.Fatalf("expected %s at index %d, got %s", v, i, cs[i])
					}
				}
			}
		})
	}
}

func TestClaimStrings_MarshalJSON(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    ClaimStrings
		expected string
	}{
		{
			name:     "single element as one-element array",
			input:    ClaimStrings{"audience1"},
			expected: `["audience1"]`,
		},
		{
			name:     "multiple elements as array",
			input:    ClaimStrings{"audience1", "audience2"},
			expected: `["audience1","audience2"]`,
		},
		{
			name:     "empty as array",
			input:    ClaimStrings{},
			expected: `[]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, err := json.Marshal(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if string(b) != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, string(b))
			}
		})
	}
}

func TestSingleAsString(t *testing.T) {
	t.Parallel()

	// Exercised on a nested "aud" field: the option reaches it because the
	// WithMarshalers dispatch runs at every node of the value tree.
	type claims struct {
		Audience ClaimStrings `json:"aud"`
		Issuer   string       `json:"iss"`
	}

	single := claims{Audience: ClaimStrings{"aud1"}, Issuer: "iss1"}
	multiple := claims{Audience: ClaimStrings{"aud1", "aud2"}, Issuer: "iss1"}

	testCases := []struct {
		name     string
		marshal  func(any) ([]byte, error)
		input    claims
		expected string
	}{
		{
			name:     "default marshals single audience as array",
			marshal:  func(v any) ([]byte, error) { return json.Marshal(v) },
			input:    single,
			expected: `{"aud":["aud1"],"iss":"iss1"}`,
		},
		{
			name:     "option marshals single audience as string",
			marshal:  func(v any) ([]byte, error) { return json.Marshal(v, SingleAsString()) },
			input:    single,
			expected: `{"aud":"aud1","iss":"iss1"}`,
		},
		{
			name:     "option keeps multiple audiences as array",
			marshal:  func(v any) ([]byte, error) { return json.Marshal(v, SingleAsString()) },
			input:    multiple,
			expected: `{"aud":["aud1","aud2"],"iss":"iss1"}`,
		},
		{
			name:     "option keeps empty audience as empty array",
			marshal:  func(v any) ([]byte, error) { return json.Marshal(v, SingleAsString()) },
			input:    claims{Audience: ClaimStrings{}, Issuer: "iss1"},
			expected: `{"aud":[],"iss":"iss1"}`,
		},
		{
			name:     "option keeps nil audience as empty array",
			marshal:  func(v any) ([]byte, error) { return json.Marshal(v, SingleAsString()) },
			input:    claims{Issuer: "iss1"},
			expected: `{"aud":[],"iss":"iss1"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, err := tc.marshal(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if string(b) != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, string(b))
			}
		})
	}
}

func TestConvert(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       any
		expected    ClaimStrings
		expectError bool
		errorCheck  func(error) bool
	}{
		{
			name:     "single string",
			input:    "audience1",
			expected: ClaimStrings{"audience1"},
		},
		{
			name:     "string slice",
			input:    []string{"audience1", "audience2"},
			expected: ClaimStrings{"audience1", "audience2"},
		},
		{
			name:     "any slice with strings",
			input:    []any{"audience1", "audience2"},
			expected: ClaimStrings{"audience1", "audience2"},
		},
		{
			name:        "any slice with non-string",
			input:       []any{"audience1", 123},
			expectError: true,
		},
		{
			name:        "integer (unsupported)",
			input:       123,
			expectError: true,
			errorCheck: func(err error) bool {
				return errors.Is(err, altshiftErrors.ErrUnexpectedType)
			},
		},
		{
			name:        "nil (unsupported)",
			input:       nil,
			expectError: true,
			errorCheck: func(err error) bool {
				return errors.Is(err, altshiftErrors.ErrUnexpectedType)
			},
		},
		{
			name:        "map (unsupported)",
			input:       map[string]string{"key": "value"},
			expectError: true,
			errorCheck: func(err error) bool {
				return errors.Is(err, altshiftErrors.ErrUnexpectedType)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := Convert(tc.input)

			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tc.errorCheck != nil && !tc.errorCheck(err) {
					t.Fatalf("error check failed for error: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if len(result) != len(tc.expected) {
					t.Fatalf("expected length %d, got %d", len(tc.expected), len(result))
				}

				for i, v := range tc.expected {
					if result[i] != v {
						t.Fatalf("expected %s at index %d, got %s", v, i, result[i])
					}
				}
			}
		})
	}
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input ClaimStrings
	}{
		{
			name:  "single string",
			input: ClaimStrings{"audience1"},
		},
		{
			name:  "multiple strings",
			input: ClaimStrings{"audience1", "audience2", "audience3"},
		},
		{
			name:  "empty",
			input: ClaimStrings{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, err := json.Marshal(tc.input)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			var decoded ClaimStrings
			if err := json.Unmarshal(b, &decoded); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}

			if len(decoded) != len(tc.input) {
				t.Fatalf("round trip failed: expected length %d, got %d", len(tc.input), len(decoded))
			}

			for i, v := range tc.input {
				if decoded[i] != v {
					t.Fatalf("round trip failed at index %d: expected %s, got %s", i, v, decoded[i])
				}
			}
		})
	}
}
