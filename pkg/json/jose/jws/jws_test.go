package jws

import (
	"encoding/base64"
	"errors"
	"testing"

	errors2 "github.com/altshiftab/utils_go/pkg/errors"
)

func TestSplit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		token       string
		expected    [3]string
		expectError bool
	}{
		{
			name:        "empty token",
			token:       "",
			expected:    [3]string{},
			expectError: true,
		},
		{
			name:        "single part",
			token:       "header",
			expected:    [3]string{},
			expectError: true,
		},
		{
			name:        "two parts",
			token:       "header.payload",
			expected:    [3]string{},
			expectError: true,
		},
		{
			name:        "three parts",
			token:       "header.payload.signature",
			expected:    [3]string{"header", "payload", "signature"},
			expectError: false,
		},
		{
			name:        "four parts (extra dots in signature)",
			token:       "header.payload.sig.nature",
			expected:    [3]string{"header", "payload", "sig.nature"},
			expectError: false,
		},
		{
			name:        "empty parts",
			token:       "..",
			expected:    [3]string{"", "", ""},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parts, err := Split(tc.token)

			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if !errors.Is(err, errors2.ErrBadSplit) {
					t.Fatalf("expected ErrBadSplit, got: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error but got: %v", err)
				}
				if parts != tc.expected {
					t.Fatalf("expected %v, got %v", tc.expected, parts)
				}
			}
		})
	}
}

func TestParse(t *testing.T) {
	t.Parallel()

	// Create valid base64-encoded parts
	validHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))
	validPayload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"1234567890"}`))
	validSignature := base64.RawURLEncoding.EncodeToString([]byte("signature"))

	testCases := []struct {
		name           string
		token          string
		expectedHeader []byte
		expectError    bool
	}{
		{
			name:        "empty token",
			token:       "",
			expectError: true,
		},
		{
			name:        "invalid split",
			token:       "only.two",
			expectError: true,
		},
		{
			name:        "invalid base64 header",
			token:       "!!!invalid!!!" + "." + validPayload + "." + validSignature,
			expectError: true,
		},
		{
			name:        "invalid base64 payload",
			token:       validHeader + ".!!!invalid!!!." + validSignature,
			expectError: true,
		},
		{
			name:        "invalid base64 signature",
			token:       validHeader + "." + validPayload + ".!!!invalid!!!",
			expectError: true,
		},
		{
			name:           "valid token",
			token:          validHeader + "." + validPayload + "." + validSignature,
			expectedHeader: []byte(`{"alg":"HS256"}`),
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			header, _, _, err := Parse(tc.token)

			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error but got: %v", err)
				}
				if string(header) != string(tc.expectedHeader) {
					t.Fatalf("expected header %s, got %s", tc.expectedHeader, header)
				}
			}
		})
	}
}
