package key_handler

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	altshiftCryptoEcdsa "github.com/altshiftab/utils_go/pkg/crypto/ecdsa"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	ecKey "github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key/ec"
)

// ecJwk generates a P-256 key pair and returns its JWK map (with the given kid)
// alongside the private key so tests can produce signatures.
func ecJwk(t *testing.T, kid string) (map[string]any, *ecdsa.PrivateKey) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	material, err := ecKey.NewFromPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("new from public key: %v", err)
	}
	if material == nil {
		t.Fatal("expected non-nil material")
	}

	keyMap := map[string]any{
		"kty": "EC",
		"crv": material.Crv,
		"x":   material.X,
		"y":   material.Y,
		"kid": kid,
		"alg": "ES256",
		"use": "sig",
	}

	return keyMap, privateKey
}

// newTestServer serves the given JWKS payload with a Cache-Control max-age header,
// counting the number of requests received.
func newTestServer(t *testing.T, keys []map[string]any) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	body, err := json.Marshal(map[string]any{"keys": keys})
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}

	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	return server, &hits
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	return parsed
}

func TestNew_NilUrl(t *testing.T) {
	t.Parallel()

	handler, err := New(nil)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if handler != nil {
		t.Fatalf("expected nil handler, got %v", handler)
	}
	if _, ok := errors.AsType[*nil_error.Error](err); !ok {
		t.Fatalf("expected *nil_error.Error, got %v", err)
	}
}

func TestHandler_GetNamedVerifier_Success(t *testing.T) {
	t.Parallel()

	keyMap, privateKey := ecJwk(t, "key-1")
	server, hits := newTestServer(t, []map[string]any{keyMap})

	handler, err := New(mustParseURL(t, server.URL))
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	verifier, err := handler.GetNamedVerifier(context.Background(), "key-1")
	if err != nil {
		t.Fatalf("get named verifier: %v", err)
	}
	if verifier == nil {
		t.Fatal("expected non-nil verifier")
	}
	if name := verifier.GetName(); name != "ES256" {
		t.Fatalf("verifier name = %q, want ES256", name)
	}

	// The returned verifier must correspond to the served key: a signature made
	// with the matching private key verifies.
	signer, err := altshiftCryptoEcdsa.FromPrivateKey(privateKey)
	if err != nil {
		t.Fatalf("from private key: %v", err)
	}
	message := []byte("hello jose")
	signature, err := signer.Sign(message)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := verifier.Verify(message, signature); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// A second lookup is served from cache: same verifier instance, no extra fetch.
	verifier2, err := handler.GetNamedVerifier(context.Background(), "key-1")
	if err != nil {
		t.Fatalf("get named verifier (cached): %v", err)
	}
	if verifier2 != verifier {
		t.Fatal("expected cached verifier instance to be reused")
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("server hits = %d, want 1 (keys should be cached)", got)
	}
}

func TestHandler_GetNamedVerifier_UnknownKid(t *testing.T) {
	t.Parallel()

	keyMap, _ := ecJwk(t, "key-1")
	server, _ := newTestServer(t, []map[string]any{keyMap})

	handler, err := New(mustParseURL(t, server.URL))
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	verifier, err := handler.GetNamedVerifier(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verifier != nil {
		t.Fatalf("expected nil verifier for unknown kid, got %v", verifier)
	}
}

func TestHandler_GetNamedVerifier_MalformedKey(t *testing.T) {
	t.Parallel()

	// Matching kid but missing the crv/x/y material required to build the key.
	malformed := map[string]any{"kty": "EC", "kid": "key-1", "alg": "ES256"}
	server, _ := newTestServer(t, []map[string]any{malformed})

	handler, err := New(mustParseURL(t, server.URL))
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	if _, err := handler.GetNamedVerifier(context.Background(), "key-1"); err == nil {
		t.Fatal("expected error for malformed key but got nil")
	}
}

func TestHandler_GetNamedVerifier_InvalidJson(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write([]byte("not valid json"))
	}))
	t.Cleanup(server.Close)

	handler, err := New(mustParseURL(t, server.URL))
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	if _, err := handler.GetNamedVerifier(context.Background(), "key-1"); err == nil {
		t.Fatal("expected error for invalid JSON but got nil")
	}
}

func TestHandler_GetNamedVerifier_ExpiresHeaderFallback(t *testing.T) {
	t.Parallel()

	keyMap, _ := ecJwk(t, "key-1")
	body, err := json.Marshal(map[string]any{"keys": []map[string]any{keyMap}})
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}

	// No Cache-Control header, forcing the RFC1123 Expires fallback path.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		expires := time.Now().Add(time.Hour).UTC().Format(time.RFC1123)
		w.Header().Set("Expires", expires)
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	handler, err := New(mustParseURL(t, server.URL))
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	verifier, err := handler.GetNamedVerifier(context.Background(), "key-1")
	if err != nil {
		t.Fatalf("get named verifier: %v", err)
	}
	if verifier == nil || verifier.GetName() != "ES256" {
		t.Fatalf("unexpected verifier: %v", verifier)
	}
}

func TestHandler_GetNamedVerifier_FetchError(t *testing.T) {
	t.Parallel()

	// Point at a closed server address so the fetch fails outright.
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := server.URL
	server.Close()

	handler, err := New(mustParseURL(t, closedURL))
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	if _, err := handler.GetNamedVerifier(context.Background(), "key-1"); err == nil {
		t.Fatal("expected fetch error but got nil")
	}
}
