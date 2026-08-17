package gcp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json/v2"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/credentials_file"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/token_response"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/token_source/authorized_user_token_source"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/token_source/metadata_token_source"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/token_source/service_account_token_source"
)

func testMetadataServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *url.URL) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL + "/computeMetadata/v1")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return server, u
}

func testTokenServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func generateTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return key
}

func encodePKCS1PEM(key *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
}

func TestGetIdToken(t *testing.T) {
	t.Parallel()

	_, metadataUrl := testMetadataServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/instance/service-accounts/default/identity") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Metadata-Flavor") != "Google" {
			t.Errorf("missing Metadata-Flavor header")
		}
		audience := r.URL.Query().Get("audience")
		if audience != "https://example.com" {
			t.Errorf("unexpected audience: %s", audience)
		}
		fmt.Fprint(w, "test-id-token")
	})

	client := NewClientWithUrls(metadataUrl, DefaultTokenUrl)
	token, err := client.GetIdToken(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "test-id-token" {
		t.Fatalf("expected 'test-id-token', got %q", token)
	}
}

func TestGetIdToken_EmptyAudience(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.GetIdToken(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty audience")
	}
}

func TestGetIdToken_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.GetIdToken(ctx, "https://example.com")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGetProjectId(t *testing.T) {
	t.Parallel()

	_, metadataUrl := testMetadataServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/project/project-id") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Metadata-Flavor") != "Google" {
			t.Errorf("missing Metadata-Flavor header")
		}
		fmt.Fprint(w, "my-project-123")
	})

	client := NewClientWithUrls(metadataUrl, DefaultTokenUrl)
	projectId, err := client.GetProjectId(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if projectId != "my-project-123" {
		t.Fatalf("expected 'my-project-123', got %q", projectId)
	}
}

func TestGetProjectId_CancelledContext(t *testing.T) {
	t.Parallel()

	client := NewClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.GetProjectId(ctx)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestParseTokenResponse(t *testing.T) {
	t.Parallel()

	data := []byte(`{"access_token":"ya29.abc","expires_in":3600,"token_type":"Bearer"}`)
	resp, err := token_response.Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AccessToken != "ya29.abc" {
		t.Errorf("expected access token 'ya29.abc', got %q", resp.AccessToken)
	}
	if resp.TokenType != "Bearer" {
		t.Errorf("expected token type 'Bearer', got %q", resp.TokenType)
	}
	if resp.ExpiresIn <= 0 {
		t.Error("expected positive expires_in")
	}
}

func TestParseTokenResponse_NoExpiry(t *testing.T) {
	t.Parallel()

	data := []byte(`{"access_token":"ya29.abc","token_type":"Bearer"}`)
	resp, err := token_response.Parse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ExpiresIn != 0 {
		t.Error("expected zero expires_in")
	}
}

func TestParseTokenResponse_EmptyAccessToken(t *testing.T) {
	t.Parallel()

	data := []byte(`{"access_token":"","expires_in":3600}`)
	_, err := token_response.Parse(data)
	if err == nil {
		t.Fatal("expected error for empty access token")
	}
}

func TestParseTokenResponse_InvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := token_response.Parse([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestAuthorizedUserTokenSource(t *testing.T) {
	t.Parallel()

	server := testTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.PostForm.Get("grant_type") != "refresh_token" {
			t.Errorf("unexpected grant_type: %s", r.PostForm.Get("grant_type"))
		}
		if r.PostForm.Get("client_id") != "test-client-id" {
			t.Errorf("unexpected client_id: %s", r.PostForm.Get("client_id"))
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{
			"access_token": "ya29.refreshed",
			"expires_in":   3600,
			"token_type":   "Bearer",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	creds := &credentials_file.File{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RefreshToken: "test-refresh-token",
	}
	ts, err := authorized_user_token_source.NewFromCredentialsFile(
		context.Background(),
		server.URL,
		creds,
	)
	if err != nil {
		t.Fatalf("unexpected error creating token source: %v", err)
	}

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.AccessToken != "ya29.refreshed" {
		t.Errorf("expected 'ya29.refreshed', got %q", tok.AccessToken)
	}
}

func TestAuthorizedUserTokenSource_CancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	creds := &credentials_file.File{
		ClientID:     "id",
		ClientSecret: "secret",
		RefreshToken: "token",
	}
	ts, err := authorized_user_token_source.NewFromCredentialsFile(
		ctx,
		"http://localhost",
		creds,
	)
	if err != nil {
		t.Fatalf("unexpected error creating token source: %v", err)
	}
	_, err = ts.Token()
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestServiceAccountTokenSource(t *testing.T) {
	t.Parallel()

	key := generateTestRSAKey(t)

	server := testTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.PostForm.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Errorf("unexpected grant_type: %s", r.PostForm.Get("grant_type"))
		}
		assertion := r.PostForm.Get("assertion")
		if assertion == "" {
			t.Error("missing assertion")
		}
		parts := strings.Split(assertion, ".")
		if len(parts) != 3 {
			t.Errorf("expected 3-part JWT, got %d parts", len(parts))
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{ //nolint:gosec // G101: fake OAuth token in test fixture
			"access_token": "ya29.sa-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	creds := &credentials_file.File{
		ClientEmail:  "test@test.iam.gserviceaccount.com",
		PrivateKeyID: "key-id-123",
		PrivateKey:   encodePKCS1PEM(key),
	}
	ts, err := service_account_token_source.NewFromCredentialsFile(
		context.Background(),
		server.URL,
		creds,
		[]string{"https://www.googleapis.com/auth/cloud-platform"},
	)
	if err != nil {
		t.Fatalf("unexpected error creating token source: %v", err)
	}

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.AccessToken != "ya29.sa-token" { //nolint:gosec // G101: fake OAuth token in test
		t.Errorf("expected 'ya29.sa-token', got %q", tok.AccessToken)
	}
}

func TestServiceAccountTokenSource_CancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	key := generateTestRSAKey(t)
	creds := &credentials_file.File{
		ClientEmail: "test@test.iam.gserviceaccount.com",
		PrivateKey:  encodePKCS1PEM(key),
	}
	ts, err := service_account_token_source.NewFromCredentialsFile(
		ctx,
		"http://localhost",
		creds,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error creating token source: %v", err)
	}
	_, err = ts.Token()
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestMetadataTokenSource(t *testing.T) {
	t.Parallel()

	_, metadataUrl := testMetadataServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/instance/service-accounts/default/token") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Metadata-Flavor") != "Google" {
			t.Errorf("missing Metadata-Flavor header")
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{ //nolint:gosec // G101: fake OAuth token in test fixture
			"access_token": "ya29.metadata",
			"expires_in":   3600,
			"token_type":   "Bearer",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	ts, err := metadata_token_source.New(
		context.Background(),
		metadataUrl,
		[]string{"https://www.googleapis.com/auth/cloud-platform"},
	)
	if err != nil {
		t.Fatalf("unexpected error creating token source: %v", err)
	}

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.AccessToken != "ya29.metadata" { //nolint:gosec // G101: fake OAuth token in test
		t.Errorf("expected 'ya29.metadata', got %q", tok.AccessToken)
	}
}

func TestMetadataTokenSource_CancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ts, err := metadata_token_source.New(
		ctx,
		defaultMetadataBaseUrl,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error creating token source: %v", err)
	}
	_, err = ts.Token()
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestMetadataTokenSource_WithScopes(t *testing.T) {
	t.Parallel()

	_, metadataUrl := testMetadataServer(t, func(w http.ResponseWriter, r *http.Request) {
		scopes := r.URL.Query().Get("scopes")
		if scopes != "scope1,scope2" {
			t.Errorf("unexpected scopes: %s", scopes)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{ //nolint:gosec // G101: fake OAuth token in test fixture
			"access_token": "ya29.scoped",
			"expires_in":   3600,
			"token_type":   "Bearer",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	ts, err := metadata_token_source.New(
		context.Background(),
		metadataUrl,
		[]string{"scope1", "scope2"},
	)
	if err != nil {
		t.Fatalf("unexpected error creating token source: %v", err)
	}

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.AccessToken != "ya29.scoped" { //nolint:gosec // G101: fake OAuth token in test
		t.Errorf("expected 'ya29.scoped', got %q", tok.AccessToken)
	}
}

func TestCredentialsFileTokenSource_AuthorizedUser(t *testing.T) {
	t.Parallel()

	server := testTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{ //nolint:gosec // G101: fake OAuth token in test fixture
			"access_token": "ya29.creds",
			"expires_in":   3600,
			"token_type":   "Bearer",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	creds := credentials_file.File{
		Type:         credentialTypeAuthorizedUser,
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RefreshToken: "refresh-token",
	}
	data, _ := json.Marshal(creds) //nolint:gosec // test fixture

	client := NewClientWithUrls(defaultMetadataBaseUrl, server.URL)
	ts, err := client.credentialsFileTokenSource(context.Background(), data, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("token error: %v", err)
	}
	if tok.AccessToken != "ya29.creds" { //nolint:gosec // G101: fake OAuth token in test
		t.Errorf("expected 'ya29.creds', got %q", tok.AccessToken)
	}
}

func TestCredentialsFileTokenSource_ServiceAccount(t *testing.T) {
	t.Parallel()

	key := generateTestRSAKey(t)

	server := testTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{ //nolint:gosec // G101: fake OAuth token in test fixture
			"access_token": "ya29.sa-creds",
			"expires_in":   3600,
			"token_type":   "Bearer",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	creds := credentials_file.File{
		Type:         credentialTypeServiceAccount,
		ClientEmail:  "test@test.iam.gserviceaccount.com",
		PrivateKeyID: "key-id",
		PrivateKey:   encodePKCS1PEM(key),
		TokenURL:     server.URL,
	}
	data, _ := json.Marshal(creds) //nolint:gosec // test fixture

	client := NewClientWithUrls(defaultMetadataBaseUrl, server.URL)
	ts, err := client.credentialsFileTokenSource(context.Background(), data, []string{"scope1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("token error: %v", err)
	}
	if tok.AccessToken != "ya29.sa-creds" { //nolint:gosec // G101: fake OAuth token in test
		t.Errorf("expected 'ya29.sa-creds', got %q", tok.AccessToken)
	}
}

func TestCredentialsFileTokenSource_ServiceAccount_FallbackTokenUrl(t *testing.T) {
	t.Parallel()

	key := generateTestRSAKey(t)

	server := testTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{ //nolint:gosec // G101: fake OAuth token in test fixture
			"access_token": "ya29.fallback",
			"expires_in":   3600,
			"token_type":   "Bearer",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	creds := credentials_file.File{
		Type:         credentialTypeServiceAccount,
		ClientEmail:  "test@test.iam.gserviceaccount.com",
		PrivateKeyID: "key-id",
		PrivateKey:   encodePKCS1PEM(key),
		// TokenURI intentionally empty — should fall back to client's tokenUrl
	}
	data, _ := json.Marshal(creds) //nolint:gosec // test fixture

	client := NewClientWithUrls(defaultMetadataBaseUrl, server.URL)
	ts, err := client.credentialsFileTokenSource(context.Background(), data, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("token error: %v", err)
	}
	if tok.AccessToken != "ya29.fallback" { //nolint:gosec // G101: fake OAuth token in test
		t.Errorf("expected 'ya29.fallback', got %q", tok.AccessToken)
	}
}

func TestCredentialsFileTokenSource_UnsupportedType(t *testing.T) {
	t.Parallel()

	data := []byte(`{"type":"external_account"}`)
	client := NewClient()
	_, err := client.credentialsFileTokenSource(context.Background(), data, nil)
	if err == nil {
		t.Fatal("expected error for unsupported credential type")
	}
}

func TestCredentialsFileTokenSource_InvalidJSON(t *testing.T) {
	t.Parallel()

	client := NewClient()
	_, err := client.credentialsFileTokenSource(context.Background(), []byte(`not json`), nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFindDefaultCredentials_EnvVar(t *testing.T) {
	key := generateTestRSAKey(t)

	server := testTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{ //nolint:gosec // G101: fake OAuth token in test fixture
			"access_token": "ya29.env",
			"expires_in":   3600,
			"token_type":   "Bearer",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	creds := credentials_file.File{
		Type:         credentialTypeServiceAccount,
		ClientEmail:  "test@test.iam.gserviceaccount.com",
		PrivateKeyID: "key-id",
		PrivateKey:   encodePKCS1PEM(key),
		TokenURL:     server.URL,
	}
	data, err := json.Marshal(creds) //nolint:gosec // test fixture
	if err != nil {
		t.Fatalf("marshal creds: %v", err)
	}

	tmpFile := filepath.Join(t.TempDir(), "creds.json")
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", tmpFile)

	client := NewClientWithUrls(defaultMetadataBaseUrl, server.URL)
	ts, err := client.FindDefaultCredentials(context.Background(), []string{"scope1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("token error: %v", err)
	}
	if tok.AccessToken != "ya29.env" { //nolint:gosec // G101: fake OAuth token in test
		t.Errorf("expected 'ya29.env', got %q", tok.AccessToken)
	}
}

func TestFindDefaultCredentials_EnvVar_FileNotFound(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nonexistent/path/creds.json")

	client := NewClient()
	_, err := client.FindDefaultCredentials(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nonexistent credentials file")
	}
}

func TestFindDefaultCredentials_MetadataFallback(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())

	_, metadataUrl := testMetadataServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{ //nolint:gosec // G101: fake OAuth token in test fixture
			"access_token": "ya29.metadata-fallback",
			"expires_in":   3600,
			"token_type":   "Bearer",
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	})

	client := NewClientWithUrls(metadataUrl, DefaultTokenUrl)
	ts, err := client.FindDefaultCredentials(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("token error: %v", err)
	}
	if tok.AccessToken != "ya29.metadata-fallback" { //nolint:gosec // G101: fake OAuth token in test
		t.Errorf("expected 'ya29.metadata-fallback', got %q", tok.AccessToken)
	}
}

func TestFindDefaultCredentials_CancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewClient()
	_, err := client.FindDefaultCredentials(ctx, nil)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestFindDefaultCredentials_NoCredentials(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())

	// No metadata URL — should return nil token source.
	client := NewClientWithUrls(nil, DefaultTokenUrl)
	ts, err := client.FindDefaultCredentials(context.Background(), nil)
	if err == nil {
		t.Fatal("expected an error when no credentials are available")
	}
	if ts != nil {
		t.Fatal("expected nil token source when no credentials are available")
	}
}
