package authorized_user_token_source

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/credentials_file"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/token_response"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func testCredentialsFile() *credentials_file.File {
	return &credentials_file.File{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RefreshToken: "refresh-token",
	}
}

func TestNewFromCredentialsFile(t *testing.T) {
	t.Parallel()

	t.Run("empty token url", func(t *testing.T) {
		t.Parallel()

		ts, err := NewFromCredentialsFile(context.Background(), "", testCredentialsFile())
		if err == nil {
			t.Fatal("expected error for empty token url")
		}
		if ts != nil {
			t.Errorf("expected nil token source, got %v", ts)
		}
		ee, ok := errors.AsType[*empty_error.Error](err)
		if !ok {
			t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
		}
		if ee.Field != "token url" {
			t.Errorf("Field = %q, want %q", ee.Field, "token url")
		}
	})

	t.Run("nil credentials file", func(t *testing.T) {
		t.Parallel()

		ts, err := NewFromCredentialsFile(context.Background(), "https://oauth2.example/token", nil)
		if err == nil {
			t.Fatal("expected error for nil credentials file")
		}
		if ts != nil {
			t.Errorf("expected nil token source, got %v", ts)
		}
		ne, ok := errors.AsType[*nil_error.Error](err)
		if !ok {
			t.Fatalf("err type = %T (%v), want *nil_error.Error", err, err)
		}
		if ne.Field != "credentials file" {
			t.Errorf("Field = %q, want %q", ne.Field, "credentials file")
		}
	})

	t.Run("fields set", func(t *testing.T) {
		t.Parallel()

		credFile := testCredentialsFile()
		options := []fetch_config.Option{fetch_config.WithMethod(http.MethodPost)}

		ts, err := NewFromCredentialsFile(context.Background(), "https://oauth2.example/token", credFile, options...)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ts.clientID != credFile.ClientID {
			t.Errorf("clientID = %q, want %q", ts.clientID, credFile.ClientID)
		}
		if ts.clientSecret != credFile.ClientSecret {
			t.Errorf("clientSecret = %q, want %q", ts.clientSecret, credFile.ClientSecret)
		}
		if ts.refreshToken != credFile.RefreshToken {
			t.Errorf("refreshToken = %q, want %q", ts.refreshToken, credFile.RefreshToken)
		}
		if ts.tokenUrl != "https://oauth2.example/token" {
			t.Errorf("tokenUrl = %q, want %q", ts.tokenUrl, "https://oauth2.example/token")
		}
		if len(ts.options) != len(options) {
			t.Errorf("options len = %d, want %d", len(ts.options), len(options))
		}
		if ts.CredentialsFile() != credFile {
			t.Error("CredentialsFile() did not return the provided file")
		}
	})
}

func TestToken(t *testing.T) {
	t.Parallel()

	credFile := testCredentialsFile()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, want %q", got, "application/x-www-form-urlencoded")
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
			return
		}
		if got := r.PostForm.Get("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q, want %q", got, "refresh_token")
		}
		if got := r.PostForm.Get("client_id"); got != credFile.ClientID {
			t.Errorf("client_id = %q, want %q", got, credFile.ClientID)
		}
		if got := r.PostForm.Get("client_secret"); got != credFile.ClientSecret {
			t.Errorf("client_secret = %q, want %q", got, credFile.ClientSecret)
		}
		if got := r.PostForm.Get("refresh_token"); got != credFile.RefreshToken {
			t.Errorf("refresh_token = %q, want %q", got, credFile.RefreshToken)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, &token_response.Response{ //nolint:gosec // G117: fake OAuth token in test fixture
			AccessToken: "access-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}); err != nil {
			t.Errorf("encode: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	ts, err := NewFromCredentialsFile(context.Background(), server.URL, credFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.AccessToken != "access-token" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "access-token")
	}
	if tok.Type() != "Bearer" {
		t.Errorf("Type() = %q, want %q", tok.Type(), "Bearer")
	}
	if tok.Expiry.IsZero() {
		t.Error("expected non-zero Expiry")
	}
}

func TestToken_CancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ts, err := NewFromCredentialsFile(ctx, "https://oauth2.example/token", testCredentialsFile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := ts.Token(); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestToken_ServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	ts, err := NewFromCredentialsFile(context.Background(), server.URL, testCredentialsFile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := ts.Token(); err == nil {
		t.Fatal("expected error for non-2xx status")
	}
}

func TestToken_EmptyBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	ts, err := NewFromCredentialsFile(context.Background(), server.URL, testCredentialsFile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = ts.Token()
	if err == nil {
		t.Fatal("expected error for empty token response")
	}
	ne, ok := errors.AsType[*nil_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *nil_error.Error", err, err)
	}
	if ne.Field != "token response" {
		t.Errorf("Field = %q, want %q", ne.Field, "token response")
	}
}
