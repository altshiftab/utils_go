package config

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	motmedelHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
	oauth2Errors "github.com/altshiftab/utils_go/pkg/oauth2/errors"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/auth_code_option"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/endpoint"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token"
)

// capturedRequest records the parts of a token-endpoint request the tests assert on.
type capturedRequest struct {
	PostForm url.Values
	Header   http.Header
}

func TestConfigAuthCodeURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		authURL    string
		clientID   string
		redirect   string
		scopes     []string
		state      string
		opts       []auth_code_option.AuthCodeOption
		wantValues map[string]string
		wantAbsent []string
	}{
		{
			name:     "full parameters",
			authURL:  "https://auth.test/authorize",
			clientID: "client-id",
			redirect: "https://app.test/callback",
			scopes:   []string{"read", "write"},
			state:    "xyz",
			opts:     []auth_code_option.AuthCodeOption{auth_code_option.New("access_type", "offline")},
			wantValues: map[string]string{
				"response_type": "code",
				"client_id":     "client-id",
				"redirect_uri":  "https://app.test/callback",
				"scope":         "read write",
				"state":         "xyz",
				"access_type":   "offline",
			},
		},
		{
			name:     "minimal parameters omit empty",
			authURL:  "https://auth.test/authorize",
			clientID: "client-id",
			wantValues: map[string]string{
				"response_type": "code",
				"client_id":     "client-id",
			},
			wantAbsent: []string{"redirect_uri", "scope", "state"},
		},
		{
			name:     "existing query uses ampersand",
			authURL:  "https://auth.test/authorize?foo=bar",
			clientID: "client-id",
			state:    "st",
			wantValues: map[string]string{
				"foo":           "bar",
				"response_type": "code",
				"client_id":     "client-id",
				"state":         "st",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				ClientID:    testCase.clientID,
				RedirectURL: testCase.redirect,
				Scopes:      testCase.scopes,
				Endpoint:    endpoint.Endpoint{AuthURL: testCase.authURL},
			}

			raw := cfg.AuthCodeURL(testCase.state, testCase.opts...)

			parsed, err := url.Parse(raw)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", raw, err)
			}
			query := parsed.Query()

			for key, want := range testCase.wantValues {
				if got := query.Get(key); got != want {
					t.Errorf("query %q = %q, want %q", key, got, want)
				}
			}
			for _, key := range testCase.wantAbsent {
				if _, ok := query[key]; ok {
					t.Errorf("query %q present, want absent", key)
				}
			}
		})
	}
}

// jsonTokenServer returns a server responding with a JSON token, capturing the request.
func jsonTokenServer(t *testing.T, body string, capture *capturedRequest) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if capture != nil {
			capture.PostForm = r.PostForm
			capture.Header = r.Header.Clone()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestConfigExchange(t *testing.T) {
	t.Parallel()

	var captured capturedRequest
	server := jsonTokenServer(
		t,
		`{"access_token":"at","token_type":"Bearer","refresh_token":"rt","expires_in":3600}`,
		&captured,
	)

	cfg := &Config{
		ClientID:     "cid",
		ClientSecret: "csecret",
		RedirectURL:  "https://app.test/cb",
		Endpoint:     endpoint.Endpoint{TokenURL: server.URL, AuthStyle: endpoint.AuthStyleInHeader},
	}

	before := time.Now()
	tok, err := cfg.Exchange(context.Background(), "the-code", auth_code_option.New("code_verifier", "pkce"))
	if err != nil {
		t.Fatalf("Exchange error = %v", err)
	}

	if tok.AccessToken != "at" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "at")
	}
	if tok.RefreshToken != "rt" {
		t.Errorf("RefreshToken = %q, want %q", tok.RefreshToken, "rt")
	}
	if tok.Type() != "Bearer" {
		t.Errorf("Type() = %q, want %q", tok.Type(), "Bearer")
	}
	if tok.Expiry.Before(before.Add(time.Hour-time.Minute)) || tok.Expiry.After(time.Now().Add(time.Hour+time.Minute)) {
		t.Errorf("Expiry = %v, want roughly now+1h", tok.Expiry)
	}

	if got := captured.PostForm.Get("grant_type"); got != "authorization_code" {
		t.Errorf("grant_type = %q, want %q", got, "authorization_code")
	}
	if got := captured.PostForm.Get("code"); got != "the-code" {
		t.Errorf("code = %q, want %q", got, "the-code")
	}
	if got := captured.PostForm.Get("redirect_uri"); got != "https://app.test/cb" {
		t.Errorf("redirect_uri = %q, want %q", got, "https://app.test/cb")
	}
	if got := captured.PostForm.Get("code_verifier"); got != "pkce" {
		t.Errorf("code_verifier = %q, want %q", got, "pkce")
	}
	// AuthStyleInHeader attaches basic auth, not client params.
	wantAuth := "Basic " + motmedelHttpUtils.BasicAuth("cid", "csecret")
	if got := captured.Header.Get("Authorization"); got != wantAuth {
		t.Errorf("Authorization = %q, want %q", got, wantAuth)
	}
	if got := captured.PostForm.Get("client_id"); got != "" {
		t.Errorf("client_id in body = %q, want empty for header style", got)
	}
}

func TestConfigClientCredentialsToken(t *testing.T) {
	t.Parallel()

	var captured capturedRequest
	server := jsonTokenServer(t, `{"access_token":"at","token_type":"Bearer"}`, &captured)

	cfg := &Config{
		ClientID:     "cid",
		ClientSecret: "csecret",
		Scopes:       []string{"a", "b"},
		Endpoint:     endpoint.Endpoint{TokenURL: server.URL, AuthStyle: endpoint.AuthStyleInParams},
	}

	tok, err := cfg.ClientCredentialsToken(context.Background())
	if err != nil {
		t.Fatalf("ClientCredentialsToken error = %v", err)
	}
	if tok.AccessToken != "at" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "at")
	}

	if got := captured.PostForm.Get("grant_type"); got != "client_credentials" {
		t.Errorf("grant_type = %q, want %q", got, "client_credentials")
	}
	if got := captured.PostForm.Get("scope"); got != "a b" {
		t.Errorf("scope = %q, want %q", got, "a b")
	}
	// AuthStyleInParams places credentials in the body.
	if got := captured.PostForm.Get("client_id"); got != "cid" {
		t.Errorf("client_id = %q, want %q", got, "cid")
	}
	if got := captured.PostForm.Get("client_secret"); got != "csecret" {
		t.Errorf("client_secret = %q, want %q", got, "csecret")
	}
	if got := captured.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want empty for params style", got)
	}
}

func TestConfigPasswordCredentialsToken(t *testing.T) {
	t.Parallel()

	var captured capturedRequest
	server := jsonTokenServer(t, `{"access_token":"at"}`, &captured)

	cfg := &Config{
		ClientID: "cid",
		Scopes:   []string{"scope1"},
		Endpoint: endpoint.Endpoint{TokenURL: server.URL, AuthStyle: endpoint.AuthStyleInParams},
	}

	tok, err := cfg.PasswordCredentialsToken(context.Background(), "user", "pass")
	if err != nil {
		t.Fatalf("PasswordCredentialsToken error = %v", err)
	}
	if tok.AccessToken != "at" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "at")
	}

	if got := captured.PostForm.Get("grant_type"); got != "password" {
		t.Errorf("grant_type = %q, want %q", got, "password")
	}
	if got := captured.PostForm.Get("username"); got != "user" {
		t.Errorf("username = %q, want %q", got, "user")
	}
	if got := captured.PostForm.Get("password"); got != "pass" {
		t.Errorf("password = %q, want %q", got, "pass")
	}
	if got := captured.PostForm.Get("scope"); got != "scope1" {
		t.Errorf("scope = %q, want %q", got, "scope1")
	}
}

func TestConfigRetrieveTokenFormEncodedResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-www-form-urlencoded")
		_, _ = io.WriteString(w, "access_token=at&token_type=bearer&refresh_token=rt")
	}))
	t.Cleanup(server.Close)

	cfg := &Config{
		Endpoint: endpoint.Endpoint{TokenURL: server.URL, AuthStyle: endpoint.AuthStyleInHeader},
	}

	tok, err := cfg.ClientCredentialsToken(context.Background())
	if err != nil {
		t.Fatalf("ClientCredentialsToken error = %v", err)
	}
	if tok.AccessToken != "at" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "at")
	}
	if tok.TokenType != "bearer" {
		t.Errorf("TokenType = %q, want %q", tok.TokenType, "bearer")
	}
	if tok.RefreshToken != "rt" {
		t.Errorf("RefreshToken = %q, want %q", tok.RefreshToken, "rt")
	}
}

func TestConfigRetrieveTokenErrorResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"bad","error_uri":"https://e.test"}`)
	}))
	t.Cleanup(server.Close)

	cfg := &Config{
		Endpoint: endpoint.Endpoint{TokenURL: server.URL, AuthStyle: endpoint.AuthStyleInHeader},
	}

	_, err := cfg.ClientCredentialsToken(context.Background())
	if err == nil {
		t.Fatal("error = nil, want error")
	}
	if !errors.Is(err, oauth2Errors.ErrRetrieveToken) {
		t.Errorf("errors.Is(err, ErrRetrieveToken) = false, want true (err=%v)", err)
	}

	retrieveErr, ok := errors.AsType[*oauth2Errors.RetrieveError](err)
	if !ok {
		t.Fatalf("errors.AsType[*RetrieveError] = false, want true (err=%v)", err)
	}
	if retrieveErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want %d", retrieveErr.StatusCode, http.StatusBadRequest)
	}
	if retrieveErr.ErrorCode != "invalid_grant" {
		t.Errorf("ErrorCode = %q, want %q", retrieveErr.ErrorCode, "invalid_grant")
	}
	if retrieveErr.ErrorDescription != "bad" {
		t.Errorf("ErrorDescription = %q, want %q", retrieveErr.ErrorDescription, "bad")
	}
	if retrieveErr.ErrorURI != "https://e.test" {
		t.Errorf("ErrorURI = %q, want %q", retrieveErr.ErrorURI, "https://e.test")
	}
}

func TestConfigRetrieveTokenEmptyTokenURL(t *testing.T) {
	t.Parallel()

	cfg := &Config{Endpoint: endpoint.Endpoint{TokenURL: ""}}

	_, err := cfg.ClientCredentialsToken(context.Background())
	if err == nil {
		t.Fatal("error = nil, want error")
	}
	emptyErr, ok := errors.AsType[*empty_error.Error](err)
	if !ok {
		t.Fatalf("errors.AsType[*empty_error.Error] = false, want true (err=%v)", err)
	}
	if emptyErr.Field != "token url" {
		t.Errorf("empty field = %q, want %q", emptyErr.Field, "token url")
	}
}

func TestConfigRetrieveTokenMissingAccessToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"token_type":"Bearer"}`)
	}))
	t.Cleanup(server.Close)

	cfg := &Config{
		Endpoint: endpoint.Endpoint{TokenURL: server.URL, AuthStyle: endpoint.AuthStyleInHeader},
	}

	_, err := cfg.ClientCredentialsToken(context.Background())
	if err == nil {
		t.Fatal("error = nil, want error")
	}
	emptyErr, ok := errors.AsType[*empty_error.Error](err)
	if !ok {
		t.Fatalf("errors.AsType[*empty_error.Error] = false, want true (err=%v)", err)
	}
	if emptyErr.Field != "access token" {
		t.Errorf("empty field = %q, want %q", emptyErr.Field, "access token")
	}
}

func TestConfigRetrieveTokenAutoDetectFallsBackToParams(t *testing.T) {
	t.Parallel()

	var sawHeaderAttempt, sawParamsAttempt bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("client_id") != "" {
			// Params-style attempt: succeed.
			sawParamsAttempt = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"at"}`)
			return
		}
		// Header-style attempt: reject so auto-detect falls back.
		sawHeaderAttempt = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"invalid_client"}`)
	}))
	t.Cleanup(server.Close)

	cfg := &Config{
		ClientID: "cid",
		Endpoint: endpoint.Endpoint{TokenURL: server.URL}, // AuthStyleAutoDetect (zero value)
	}

	tok, err := cfg.ClientCredentialsToken(context.Background())
	if err != nil {
		t.Fatalf("ClientCredentialsToken error = %v", err)
	}
	if tok.AccessToken != "at" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "at")
	}
	if !sawHeaderAttempt {
		t.Error("expected a header-style attempt first")
	}
	if !sawParamsAttempt {
		t.Error("expected a params-style fallback attempt")
	}
}

func TestConfigTokenSourceReusesValidToken(t *testing.T) {
	t.Parallel()

	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"refreshed"}`)
	}))
	t.Cleanup(server.Close)

	cfg := &Config{Endpoint: endpoint.Endpoint{TokenURL: server.URL, AuthStyle: endpoint.AuthStyleInHeader}}

	valid := &token.Token{AccessToken: "cached", Expiry: time.Now().Add(time.Hour), RefreshToken: "rt"}
	src := cfg.TokenSource(context.Background(), valid)

	got, err := src.Token()
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if got.AccessToken != "cached" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "cached")
	}
	if hits != 0 {
		t.Errorf("token endpoint hits = %d, want 0", hits)
	}
}

func TestConfigTokenSourceRefreshesExpiredToken(t *testing.T) {
	t.Parallel()

	var captured capturedRequest
	server := jsonTokenServer(t, `{"access_token":"refreshed","refresh_token":"rt2"}`, &captured)

	cfg := &Config{Endpoint: endpoint.Endpoint{TokenURL: server.URL, AuthStyle: endpoint.AuthStyleInHeader}}

	expired := &token.Token{AccessToken: "old", Expiry: time.Now().Add(-time.Hour), RefreshToken: "rt1"}
	src := cfg.TokenSource(context.Background(), expired)

	got, err := src.Token()
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if got.AccessToken != "refreshed" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "refreshed")
	}
	if got.RefreshToken != "rt2" {
		t.Errorf("RefreshToken = %q, want %q", got.RefreshToken, "rt2")
	}
	if g := captured.PostForm.Get("grant_type"); g != "refresh_token" {
		t.Errorf("grant_type = %q, want %q", g, "refresh_token")
	}
	if g := captured.PostForm.Get("refresh_token"); g != "rt1" {
		t.Errorf("refresh_token = %q, want %q", g, "rt1")
	}
}

func TestConfigTokenSourceRetainsRefreshTokenWhenAbsent(t *testing.T) {
	t.Parallel()

	// Response omits refresh_token; the refresher should keep the prior one.
	server := jsonTokenServer(t, `{"access_token":"refreshed"}`, nil)

	cfg := &Config{Endpoint: endpoint.Endpoint{TokenURL: server.URL, AuthStyle: endpoint.AuthStyleInHeader}}

	expired := &token.Token{AccessToken: "old", Expiry: time.Now().Add(-time.Hour), RefreshToken: "keep-me"}
	src := cfg.TokenSource(context.Background(), expired)

	got, err := src.Token()
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if got.RefreshToken != "keep-me" {
		t.Errorf("RefreshToken = %q, want %q", got.RefreshToken, "keep-me")
	}
}

func TestConfigTokenSourceEmptyRefreshToken(t *testing.T) {
	t.Parallel()

	// TokenURL is intentionally unset: the empty refresh token must fail before any request.
	cfg := &Config{}

	// A nil token yields an empty refresh token, so refreshing must fail before any request.
	src := cfg.TokenSource(context.Background(), nil)

	_, err := src.Token()
	if err == nil {
		t.Fatal("error = nil, want error")
	}
	emptyErr, ok := errors.AsType[*empty_error.Error](err)
	if !ok {
		t.Fatalf("errors.AsType[*empty_error.Error] = false, want true (err=%v)", err)
	}
	if emptyErr.Field != "refresh token" {
		t.Errorf("empty field = %q, want %q", emptyErr.Field, "refresh token")
	}
}

func TestConfigClientAttachesAuthorization(t *testing.T) {
	t.Parallel()

	var gotAuth string
	resource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(resource.Close)

	// TokenURL is intentionally unset: the cached valid token means no refresh occurs.
	cfg := &Config{}

	valid := &token.Token{AccessToken: "abc", Expiry: time.Now().Add(time.Hour)}
	client := cfg.Client(context.Background(), valid)
	if client == nil {
		t.Fatal("Client() = nil, want non-nil")
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, resource.URL, nil)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do error = %v", err)
	}
	defer resp.Body.Close()

	if gotAuth != "Bearer abc" {
		t.Errorf("resource saw Authorization = %q, want %q", gotAuth, "Bearer abc")
	}
}
