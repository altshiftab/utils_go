package cloud_kms

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_kms/cloud_kms_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_kms/types/asymmetric_sign_response"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_kms/types/public_key"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftTestingCmp "github.com/altshiftab/utils_go/pkg/testing/cmp"
)

const testKeyVersion = "projects/p/locations/europe-north2/keyRings/cicd/cryptoKeys/image-signing/cryptoKeyVersions/1"

func testServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return NewClient(cloud_kms_config.WithBaseUrl(u), cloud_kms_config.WithFetchOptions(fetch_config.WithHttpClient(server.Client())))
}

func writeJson(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.MarshalWrite(w, value); err != nil {
		t.Errorf("encode: %v", err)
	}
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	client := NewClient()
	if client.baseUrl.Host != Domain {
		t.Errorf("host = %q, want %q", client.baseUrl.Host, Domain)
	}
	if client.baseUrl.Path != "/v1/" {
		t.Errorf("path = %q, want %q", client.baseUrl.Path, "/v1/")
	}
}

func TestAsymmetricSign(t *testing.T) {
	t.Parallel()

	digest := sha256.Sum256([]byte("what is signed"))
	signature := []byte{0x30, 0x44, 0x02, 0x20, 0x01, 0x02, 0x03}

	var requestMethod, requestUri string
	var requestBody []byte

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestMethod = r.Method
		requestUri = r.RequestURI

		var err error
		requestBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}

		writeJson(t, w, &asymmetric_sign_response.Response{
			Signature: base64.StdEncoding.EncodeToString(signature),
			Name:      testKeyVersion,
		})
	})

	got, err := client.AsymmetricSign(context.Background(), testKeyVersion, digest[:])
	if err != nil {
		t.Fatalf("asymmetric sign: %v", err)
	}

	if string(got) != string(signature) {
		t.Errorf("signature = %x, want %x", got, signature)
	}
	if requestMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", requestMethod)
	}
	// The resource name's slashes stay literal; transcoding routes on the raw path.
	if want := "/v1/" + testKeyVersion + ":asymmetricSign"; requestUri != want {
		t.Errorf("request uri = %q, want %q", requestUri, want)
	}

	// The digest is sent under the name of its algorithm, and nothing else is.
	var body map[string]map[string]string
	if err := json.Unmarshal(requestBody, &body); err != nil {
		t.Fatalf("decode body %s: %v", requestBody, err)
	}
	if len(body) != 1 || len(body["digest"]) != 1 {
		t.Fatalf("body = %s, want only digest.sha256", requestBody)
	}
	sentDigest, err := base64.StdEncoding.DecodeString(body["digest"]["sha256"])
	if err != nil {
		t.Fatalf("decode sent digest: %v", err)
	}
	if string(sentDigest) != string(digest[:]) {
		t.Errorf("sent digest = %x, want %x", sentDigest, digest)
	}
}

func TestAsymmetricSignArgumentErrors(t *testing.T) {
	t.Parallel()

	digest := sha256.Sum256([]byte("x"))

	testCases := []struct {
		name       string
		keyVersion string
		digest     []byte
		wantErr    error
	}{
		{name: "empty key version", keyVersion: "", digest: digest[:], wantErr: empty_error.New("key version")},
		{name: "empty digest", keyVersion: testKeyVersion, digest: nil, wantErr: empty_error.New("digest")},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewClient().AsymmetricSign(context.Background(), testCase.keyVersion, testCase.digest)
			altshiftTestingCmp.CompareErr(t, err, testCase.wantErr)
		})
	}
}

// A digest of another algorithm's length is refused before anything is sent.
func TestAsymmetricSignRefusesAnotherDigestLength(t *testing.T) {
	t.Parallel()

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL)
	})

	_, err := client.AsymmetricSign(context.Background(), testKeyVersion, make([]byte, 48))
	if !errors.Is(err, altshiftErrors.ErrValidationError) {
		t.Errorf("error = %v, want a validation error", err)
	}
}

func TestAsymmetricSignResponses(t *testing.T) {
	t.Parallel()

	digest := sha256.Sum256([]byte("x"))

	testCases := []struct {
		name           string
		statusCode     int
		body           string
		wantStatusCode int
	}{
		{name: "permission denied", statusCode: http.StatusForbidden, body: `{}`, wantStatusCode: http.StatusForbidden},
		{name: "no signature", statusCode: http.StatusOK, body: `{"name":"k"}`},
		{name: "a signature that is not base64", statusCode: http.StatusOK, body: `{"signature":"not base64!"}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(testCase.statusCode)
				if _, err := io.WriteString(w, testCase.body); err != nil {
					t.Errorf("write: %v", err)
				}
			})

			signature, err := client.AsymmetricSign(context.Background(), testKeyVersion, digest[:])
			if err == nil {
				t.Fatalf("expected an error, got signature %x", signature)
			}

			if testCase.wantStatusCode != 0 {
				non2xxError, ok := errors.AsType[*altshiftHttpErrors.Non2xxStatusCodeError](err)
				if !ok {
					t.Fatalf("error = %v, want a non-2xx status code error", err)
				}
				if non2xxError.StatusCode != testCase.wantStatusCode {
					t.Errorf("status code = %d, want %d", non2xxError.StatusCode, testCase.wantStatusCode)
				}
			}
		})
	}
}

func TestAsymmetricSignCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	digest := sha256.Sum256([]byte("x"))
	if _, err := NewClient().AsymmetricSign(ctx, testKeyVersion, digest[:]); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
}

func TestGetPublicKey(t *testing.T) {
	t.Parallel()

	const pem = "-----BEGIN PUBLIC KEY-----\nMFkw\n-----END PUBLIC KEY-----\n"

	var requestMethod, requestUri string

	client := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestMethod = r.Method
		requestUri = r.RequestURI

		writeJson(t, w, &public_key.PublicKey{
			Pem:       pem,
			Algorithm: "EC_SIGN_P256_SHA256",
			Name:      testKeyVersion,
		})
	})

	publicKey, err := client.GetPublicKey(context.Background(), testKeyVersion)
	if err != nil {
		t.Fatalf("get public key: %v", err)
	}

	if requestMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", requestMethod)
	}
	if want := "/v1/" + testKeyVersion + "/publicKey"; requestUri != want {
		t.Errorf("request uri = %q, want %q", requestUri, want)
	}
	if publicKey.Pem != pem {
		t.Errorf("pem = %q, want %q", publicKey.Pem, pem)
	}
	if publicKey.Algorithm != "EC_SIGN_P256_SHA256" {
		t.Errorf("algorithm = %q", publicKey.Algorithm)
	}
}

func TestGetPublicKeyErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		keyVersion string
		statusCode int
		body       string
	}{
		{name: "empty key version", keyVersion: ""},
		{name: "not found", keyVersion: testKeyVersion, statusCode: http.StatusNotFound, body: `{}`},
		{name: "no pem", keyVersion: testKeyVersion, statusCode: http.StatusOK, body: `{"algorithm":"EC_SIGN_P256_SHA256"}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(testCase.statusCode)
				if _, err := io.WriteString(w, testCase.body); err != nil {
					t.Errorf("write: %v", err)
				}
			})

			publicKey, err := client.GetPublicKey(context.Background(), testCase.keyVersion)
			if err == nil {
				t.Fatalf("expected an error, got %+v", publicKey)
			}
			if testCase.keyVersion == "" && !strings.Contains(err.Error(), "key version") {
				t.Errorf("error = %v, want one naming the key version", err)
			}
		})
	}
}
