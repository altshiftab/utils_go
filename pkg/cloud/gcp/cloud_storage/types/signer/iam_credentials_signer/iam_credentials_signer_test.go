package iam_credentials_signer

import (
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/iam_credentials"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/iam_credentials/iam_credentials_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/iam_credentials/types/sign_blob_response"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

// testClient returns an iam_credentials.Client whose base URL points at a local
// httptest server, so no real GCP endpoint is contacted.
func testClient(t *testing.T, handler http.HandlerFunc) *iam_credentials.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return iam_credentials.NewClient(iam_credentials_config.WithBaseUrl(u))
}

func TestNew(t *testing.T) {
	t.Parallel()

	client := iam_credentials.NewClient()

	testCases := []struct {
		name      string
		client    *iam_credentials.Client
		email     string
		options   []fetch_config.Option
		wantErr   bool
		wantNil   bool
		wantEmpty bool
		errField  string
	}{
		{name: "nil client", client: nil, email: "robot@x.iam.gserviceaccount.com", wantErr: true, wantNil: true, errField: "iam credentials client"},
		{name: "empty email", client: client, email: "", wantErr: true, wantEmpty: true, errField: "email"},
		{name: "valid no options", client: client, email: "robot@x.iam.gserviceaccount.com"},
		{name: "valid with options", client: client, email: "robot@x.iam.gserviceaccount.com", options: []fetch_config.Option{fetch_config.WithMethod(http.MethodPost), fetch_config.WithSkipReadResponseBody(true)}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			signer, err := New(testCase.client, testCase.email, testCase.options...)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if signer != nil {
					t.Errorf("expected nil signer on error, got %v", signer)
				}
				if testCase.wantNil {
					ne, ok := errors.AsType[*nil_error.Error](err)
					if !ok {
						t.Fatalf("err type = %T (%v), want *nil_error.Error", err, err)
					}
					if ne.Field != testCase.errField {
						t.Errorf("Field = %q, want %q", ne.Field, testCase.errField)
					}
				}
				if testCase.wantEmpty {
					ee, ok := errors.AsType[*empty_error.Error](err)
					if !ok {
						t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
					}
					if ee.Field != testCase.errField {
						t.Errorf("Field = %q, want %q", ee.Field, testCase.errField)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if signer == nil {
				t.Fatal("expected non-nil signer")
			}
			if signer.client != testCase.client {
				t.Errorf("client field mismatch")
			}
			if signer.email != testCase.email {
				t.Errorf("email field = %q, want %q", signer.email, testCase.email)
			}
			if len(signer.fetchOptions) != len(testCase.options) {
				t.Errorf("fetchOptions len = %d, want %d", len(signer.fetchOptions), len(testCase.options))
			}
		})
	}
}

func TestEmail(t *testing.T) {
	t.Parallel()

	const email = "robot@x.iam.gserviceaccount.com"
	signer, err := New(iam_credentials.NewClient(), email)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := signer.Email(); got != email {
		t.Errorf("Email() = %q, want %q", got, email)
	}
}

func TestSign(t *testing.T) {
	t.Parallel()

	const email = "robot@x.iam.gserviceaccount.com"
	wantSignature := []byte{0xde, 0xad, 0xbe, 0xef}

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &sign_blob_response.Response{
			KeyId:      "key-1",
			SignedBlob: base64.StdEncoding.EncodeToString(wantSignature),
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	signer, err := New(client, email)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	signature, err := signer.Sign(context.Background(), []byte("payload"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(signature) != string(wantSignature) {
		t.Errorf("signature = %x, want %x", signature, wantSignature)
	}
}

func TestSign_ServerError(t *testing.T) {
	t.Parallel()

	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	signer, err := New(client, "robot@x.iam.gserviceaccount.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := signer.Sign(context.Background(), []byte("payload")); err == nil {
		t.Fatal("expected error for non-2xx status")
	}
}

func TestSign_InvalidBase64(t *testing.T) {
	t.Parallel()

	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &sign_blob_response.Response{
			KeyId:      "key-1",
			SignedBlob: "!!!not-valid-base64!!!",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	signer, err := New(client, "robot@x.iam.gserviceaccount.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := signer.Sign(context.Background(), []byte("payload")); err == nil {
		t.Fatal("expected error decoding invalid base64 signed blob")
	}
}
