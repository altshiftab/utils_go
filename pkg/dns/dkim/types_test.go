package dkim

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/asn1"
	"encoding/base64"
	"errors"
	"testing"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

func TestRecord_GetVersion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		record   *Record
		expected int
	}{
		{name: "default", record: &Record{}, expected: 1},
		{name: "explicit", record: &Record{Version: 2}, expected: 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.record.GetVersion(); got != tc.expected {
				t.Errorf("GetVersion() = %d, want %d", got, tc.expected)
			}
		})
	}
}

func TestRecord_GetKeyType(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		record   *Record
		expected string
	}{
		{name: "default", record: &Record{}, expected: "rsa"},
		{name: "explicit", record: &Record{KeyType: "ed25519"}, expected: "ed25519"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.record.GetKeyType(); got != tc.expected {
				t.Errorf("GetKeyType() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestRecord_GetServiceType(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		record   *Record
		expected string
	}{
		{name: "default", record: &Record{}, expected: "*"},
		{name: "explicit", record: &Record{ServiceType: "email"}, expected: "email"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.record.GetServiceType(); got != tc.expected {
				t.Errorf("GetServiceType() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestRecord_GetPublicKey(t *testing.T) {
	t.Parallel()

	const rsaKey = "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDDmzRmJRQxLEuyYiyMg4suA2SyMwR5MGHpP9diNT1hRiwUd/mZp1ro7kIDTKS8ttkI6z6eTRW9e9dDOxzSxNuXmume60Cjbu08gOyhPG3GfWdg7QkdN6kR4V75MFlw624VY35DaXBvnlTJTgRg/EW72O1DiYVThkyCgpSYS8nmEQIDAQAB"
	const ed25519Key = "11qYAYKxCrfVS/7TyWQHOg7hcvPapiMlrwIaaPcHURo="

	t.Run("empty key", func(t *testing.T) {
		t.Parallel()
		r := &Record{}
		key, err := r.GetPublicKey()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != nil {
			t.Errorf("key = %v, want nil", key)
		}
	})

	t.Run("rsa default key type", func(t *testing.T) {
		t.Parallel()
		r := &Record{PublicKeyData: rsaKey}
		key, err := r.GetPublicKey()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := key.(*rsa.PublicKey); !ok {
			t.Errorf("key type = %T, want *rsa.PublicKey", key)
		}
	})

	t.Run("explicit ed25519", func(t *testing.T) {
		t.Parallel()
		r := &Record{KeyType: "ed25519", PublicKeyData: ed25519Key}
		key, err := r.GetPublicKey()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := key.(ed25519.PublicKey); !ok {
			t.Errorf("key type = %T, want ed25519.PublicKey", key)
		}
	})

	t.Run("malformed data", func(t *testing.T) {
		t.Parallel()
		r := &Record{PublicKeyData: "not-valid-base64!!"}
		_, err := r.GetPublicKey()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if _, ok := errors.AsType[*motmedelErrors.ExtendedError](err); !ok {
			t.Errorf("expected wrapped *motmedelErrors.ExtendedError, got %T", err)
		}
	})
}

func TestParseKey(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		inputData      string
		inputKeyType   string
		expectedErrors []error
		checkErr       func(*testing.T, error)
	}{
		{
			name: "empty data",
		},
		{
			name:      "empty key type",
			inputData: "garbage",
			checkErr: func(t *testing.T, err error) {
				emptyErr, ok := errors.AsType[*empty_error.Error](err)
				if !ok {
					t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
				}
				if emptyErr.Field != "key type" {
					t.Errorf("Field = %q, want %q", emptyErr.Field, "key type")
				}
			},
		},
		{
			name:         "rsa key",
			inputData:    "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDkHlOQoBTzWRiGs5V6NpP3idY6Wk08a5qhdR6wy5bdOKb2jLQiY/J16JYi0Qvx/byYzCNb3W91y3FutACDfzwQ/BC/e/8uBsCR+yz1Lxj+PL6lHvqMKrM3rG4hstT5QjvHO9PzoxZyVYLzBfO2EeC3Ip3G+2kryOTIKT+l/K4w3QIDAQAB",
			inputKeyType: "rsa",
		},
		{
			name:         "ed25519 key",
			inputData:    "11qYAYKxCrfVS/7TyWQHOg7hcvPapiMlrwIaaPcHURo=",
			inputKeyType: "ed25519",
		},
		{
			name:           "bad base64 data",
			inputData:      "garbage",
			inputKeyType:   "garbage",
			expectedErrors: []error{base64.CorruptInputError(4)},
		},
		{
			name:           "rsa mismatch",
			inputData:      "11qYAYKxCrfVS/7TyWQHOg7hcvPapiMlrwIaaPcHURo=",
			inputKeyType:   "rsa",
			expectedErrors: []error{asn1.StructuralError{Msg: "tags don't match (16 vs {class:3 tag:23 length:90 isCompound:false}) {optional:false explicit:false application:false private:false defaultValue:<nil> tag:<nil> stringType:0 timeType:0 set:false omitEmpty:false} publicKeyInfo @2"}},
		},
		{
			name:           "ed25519 mismatch",
			inputData:      "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDkHlOQoBTzWRiGs5V6NpP3idY6Wk08a5qhdR6wy5bdOKb2jLQiY/J16JYi0Qvx/byYzCNb3W91y3FutACDfzwQ/BC/e/8uBsCR+yz1Lxj+PL6lHvqMKrM3rG4hstT5QjvHO9PzoxZyVYLzBfO2EeC3Ip3G+2kryOTIKT+l/K4w3QIDAQAB",
			inputKeyType:   "ed25519",
			expectedErrors: []error{ErrBadEd25519Length},
		},
		{
			name:         "exotic key",
			inputData:    "11qYAYKxCrfVS/7TyWQHOg7hcvPapiMlrwIaaPcHURo=",
			inputKeyType: "exotic",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseKey(testCase.inputData, testCase.inputKeyType)
			expectedErrors := testCase.expectedErrors

			if len(expectedErrors) == 0 && testCase.checkErr == nil && err != nil {
				t.Fatalf("expected no errors, got: %v", err)
			}

			if !motmedelErrors.IsAll(err, expectedErrors...) {
				t.Fatalf("expected errors: %v, got: %v", expectedErrors, err)
			}

			if testCase.checkErr != nil {
				testCase.checkErr(t, err)
			}
		})
	}
}

func TestGetKeyData(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		input          string
		expectedErrors []error
		expected       []byte
	}{
		{
			name: "empty data",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			keyData, err := GetKeyData(testCase.input)
			expectedErrors := testCase.expectedErrors

			if len(expectedErrors) == 0 && err != nil {
				t.Fatalf("expected no errors, got: %v", err)
			}

			if !motmedelErrors.IsAll(err, expectedErrors...) {
				t.Fatalf("expected errors: %v, got: %v", expectedErrors, err)
			}

			expected := testCase.expected
			if !bytes.Equal(expected, keyData) {
				t.Fatalf("mismatch, expected: %v, got: %v", expected, keyData)
			}
		})
	}
}
