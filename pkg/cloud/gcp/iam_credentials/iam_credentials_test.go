package iam_credentials

import (
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/iam_credentials/iam_credentials_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/iam_credentials/types/sign_blob_request"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/iam_credentials/types/sign_blob_response"
)

func testServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return NewClient(iam_credentials_config.WithBaseUrl(u))
}

func TestSignBlob(t *testing.T) {
	t.Parallel()

	const email = "robot@example.iam.gserviceaccount.com"
	payload := []byte("string-to-sign")
	signature := []byte{0x01, 0x02, 0x03, 0x04}

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		// Assert the on-the-wire URL keeps the resource-name slashes literal — gRPC
		// transcoding routes against the raw path and rejects %2F-encoded separators.
		wantSuffix := "/projects/-/serviceAccounts/" + email + ":signBlob"
		if !strings.HasSuffix(r.RequestURI, wantSuffix) {
			t.Errorf("unexpected RequestURI: want suffix %q, got %q", wantSuffix, r.RequestURI)
		}
		if strings.Contains(r.RequestURI, "%2F") {
			t.Errorf("RequestURI must not percent-encode resource-name slashes: %s", r.RequestURI)
		}

		var body sign_blob_request.Request
		if err := json.UnmarshalRead(r.Body, &body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		decoded, err := base64.StdEncoding.DecodeString(body.Payload)
		if err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if string(decoded) != string(payload) {
			t.Errorf("payload mismatch: want %q, got %q", payload, decoded)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &sign_blob_response.Response{
			KeyId:      "key-1",
			SignedBlob: base64.StdEncoding.EncodeToString(signature),
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	resp, err := client.SignBlob(context.Background(), email, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.KeyId != "key-1" {
		t.Errorf("KeyId: want key-1, got %s", resp.KeyId)
	}
	gotSig, err := base64.StdEncoding.DecodeString(resp.SignedBlob)
	if err != nil {
		t.Fatalf("decode signed blob: %v", err)
	}
	if string(gotSig) != string(signature) {
		t.Errorf("signature mismatch: want %x, got %x", signature, gotSig)
	}
}

func TestSignBlob_EmptyEmail(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.SignBlob(context.Background(), "", []byte("x"))
	if err == nil {
		t.Fatal("expected error for empty service account email")
	}
}

func TestSignBlob_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.SignBlob(ctx, "x@y.iam.gserviceaccount.com", []byte("x"))
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
