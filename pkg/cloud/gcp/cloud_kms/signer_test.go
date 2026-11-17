package cloud_kms

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json/v2"
	"encoding/pem"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_kms/types/asymmetric_sign_request"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_kms/types/asymmetric_sign_response"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_kms/types/public_key"
)

// fakeKms serves a key version's public half under the given algorithm and signs with signingKey, which a test may make another key than the one it publishes.
func fakeKms(t *testing.T, algorithm string, publishedKey crypto.Signer, signingKey crypto.Signer) http.HandlerFunc {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(publishedKey.Public())
	if err != nil {
		t.Fatalf("x509 marshal pkix public key: %v", err)
	}
	publicPem := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))

	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/publicKey"):
			writeJson(t, w, &public_key.PublicKey{Pem: publicPem, Algorithm: algorithm})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":asymmetricSign"):
			var request asymmetric_sign_request.Request
			if err := json.UnmarshalRead(r.Body, &request); err != nil || request.Digest == nil {
				t.Errorf("decode request: %v", err)
				return
			}
			digest, err := base64.StdEncoding.DecodeString(request.Digest.Sha256)
			if err != nil {
				t.Errorf("decode digest: %v", err)
				return
			}
			signature, err := signingKey.Sign(rand.Reader, digest, crypto.SHA256)
			if err != nil {
				t.Errorf("sign: %v", err)
				return
			}
			writeJson(t, w, &asymmetric_sign_response.Response{Signature: base64.StdEncoding.EncodeToString(signature)})
		default:
			http.NotFound(w, r)
		}
	}
}

func TestSigner(t *testing.T) {
	t.Parallel()

	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa generate key: %v", err)
	}
	digest := sha256.Sum256([]byte("the signed attributes"))

	testCases := []struct {
		name        string
		algorithm   string
		published   crypto.Signer
		signing     crypto.Signer
		opts        crypto.SignerOpts
		wantNewErr  error
		wantSignErr error
	}{
		{name: "ecdsa p-256", algorithm: "EC_SIGN_P256_SHA256", published: ecdsaKey, signing: ecdsaKey, opts: crypto.SHA256},
		{name: "rsa pkcs #1 v1.5", algorithm: "RSA_SIGN_PKCS1_2048_SHA256", published: rsaKey, signing: rsaKey, opts: crypto.SHA256},
		{name: "a key algorithm it cannot sign with", algorithm: "RSA_SIGN_PSS_2048_SHA256", published: rsaKey, signing: rsaKey, opts: crypto.SHA256, wantNewErr: ErrUnsupportedAlgorithm},
		{name: "a signature from another key", algorithm: "EC_SIGN_P256_SHA256", published: ecdsaKey, signing: otherKey, opts: crypto.SHA256, wantSignErr: ErrSignatureInvalid},
		{name: "another digest", algorithm: "EC_SIGN_P256_SHA256", published: ecdsaKey, signing: ecdsaKey, opts: crypto.SHA384, wantSignErr: ErrUnsupportedAlgorithm},
		{name: "pss options", algorithm: "RSA_SIGN_PKCS1_2048_SHA256", published: rsaKey, signing: rsaKey, opts: &rsa.PSSOptions{Hash: crypto.SHA256}, wantSignErr: ErrUnsupportedAlgorithm},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := testServer(t, fakeKms(t, testCase.algorithm, testCase.published, testCase.signing))
			signer, err := NewSigner(context.Background(), client, testKeyVersion)
			if testCase.wantNewErr != nil {
				if !errors.Is(err, testCase.wantNewErr) {
					t.Errorf("%s: new signer = %v, want %v", testCase.name, err, testCase.wantNewErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: new signer: %v", testCase.name, err)
			}
			publicKey, ok := signer.Public().(interface{ Equal(crypto.PublicKey) bool })
			if !ok || !publicKey.Equal(testCase.published.Public()) {
				t.Errorf("%s: public is not the published key", testCase.name)
			}

			signature, err := signer.Sign(rand.Reader, digest[:], testCase.opts)
			if testCase.wantSignErr != nil {
				if !errors.Is(err, testCase.wantSignErr) {
					t.Errorf("%s: sign = %v, want %v", testCase.name, err, testCase.wantSignErr)
				}
				return
			}
			if err != nil || len(signature) == 0 {
				t.Errorf("%s: sign = %x, %v", testCase.name, signature, err)
			}
		})
	}
}

// contextKey carries a value through a construction context, to see it reach the signatures.
type contextKey struct{}

// TestSignerOutlivesItsConstructionContext signs after the context the Signer was made with is cancelled, as a Signer kept for
// later requests is: each signature still reaches KMS, under the construction context's values.
func TestSignerOutlivesItsConstructionContext(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}
	client := testServer(t, fakeKms(t, "EC_SIGN_P256_SHA256", key, key))

	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "kept"))
	signer, err := NewSigner(ctx, client, testKeyVersion)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	cancel()

	for i := range 2 {
		digest := sha256.Sum256([]byte{byte(i)})
		signature, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
		if err != nil || !ecdsa.VerifyASN1(&key.PublicKey, digest[:], signature) {
			t.Fatalf("sign %d after the construction context was cancelled: %x, %v", i, signature, err)
		}
	}
	if value, _ := signer.ctx.Value(contextKey{}).(string); value != "kept" {
		t.Errorf("the construction context's values were not kept: %q", value)
	}
}

func TestNewSignerArguments(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		client     *Client
		keyVersion string
	}{
		{name: "no client", keyVersion: testKeyVersion},
		{name: "no key version", client: NewClient()},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewSigner(context.Background(), testCase.client, testCase.keyVersion); err == nil {
				t.Errorf("%s: new signer succeeded", testCase.name)
			}
		})
	}
}
