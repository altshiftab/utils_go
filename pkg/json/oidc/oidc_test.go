package oidc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestFetchProviderMetadata_NilProviderUrl(t *testing.T) {
	t.Parallel()

	metadata, err := FetchProviderMetadata(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metadata != nil {
		t.Fatalf("expected nil metadata, got %+v", metadata)
	}
}

func TestFetchProviderMetadata_Success(t *testing.T) {
	t.Parallel()

	const body = `{
		"issuer": "https://issuer.example.com",
		"authorization_endpoint": "https://issuer.example.com/authorize",
		"token_endpoint": "https://issuer.example.com/token",
		"jwks_uri": "https://issuer.example.com/jwks",
		"response_types_supported": ["code"],
		"subject_types_supported": ["public"],
		"id_token_signing_alg_values_supported": ["RS256"],
		"scopes_supported": ["openid", "profile"]
	}`

	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	providerUrl, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server url: %v", err)
	}

	metadata, err := FetchProviderMetadata(
		context.Background(),
		providerUrl,
		fetch_config.WithHttpClient(server.Client()),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metadata == nil {
		t.Fatal("expected metadata, got nil")
	}

	if requestedPath != "/.well-known/openid-configuration" {
		t.Fatalf("expected well-known path to be requested, got %q", requestedPath)
	}
	if metadata.Issuer != "https://issuer.example.com" {
		t.Fatalf("unexpected issuer: %q", metadata.Issuer)
	}
	if metadata.TokenEndpoint != "https://issuer.example.com/token" {
		t.Fatalf("unexpected token endpoint: %q", metadata.TokenEndpoint)
	}
	if len(metadata.ScopesSupported) != 2 {
		t.Fatalf("expected 2 scopes, got %v", metadata.ScopesSupported)
	}
}

func TestFetchProviderMetadata_ServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	providerUrl, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server url: %v", err)
	}

	_, err = FetchProviderMetadata(
		context.Background(),
		providerUrl,
		fetch_config.WithHttpClient(server.Client()),
	)
	if err == nil {
		t.Fatal("expected error for non-2xx status, got nil")
	}
}

func TestFetchProviderMetadata_InvalidJson(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issuer":`))
	}))
	defer server.Close()

	providerUrl, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server url: %v", err)
	}

	_, err = FetchProviderMetadata(
		context.Background(),
		providerUrl,
		fetch_config.WithHttpClient(server.Client()),
	)
	if err == nil {
		t.Fatal("expected error for invalid json, got nil")
	}
}
