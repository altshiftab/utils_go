package client_assertion_token_source

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var errMetadataDown = errors.New("metadata down")

func staticAssertion(assertion string) AssertionFunc {
	return func(context.Context) (string, error) { return assertion, nil }
}

func TestToken(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if contentType := r.Header.Get("Content-Type"); contentType != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", contentType)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		want := map[string]string{
			"grant_type":            "client_credentials",
			"client_id":             "app-1",
			"scope":                 "https://graph.microsoft.com/.default other",
			"client_assertion_type": "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
			"client_assertion":      "header.claims.signature",
		}
		for key, value := range want {
			if got := r.PostForm.Get(key); got != value {
				t.Errorf("form %s = %q, want %q", key, got, value)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token_type":"Bearer","expires_in":3599,"ext_expires_in":3599,"access_token":"graph-token"}`))
	}))
	t.Cleanup(server.Close)

	tokenSource, err := New(
		context.Background(),
		server.URL,
		"app-1",
		[]string{"https://graph.microsoft.com/.default", "other"},
		staticAssertion("header.claims.signature"),
	)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	tok, err := tokenSource.Token()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if tok.AccessToken != "graph-token" || tok.Type() != "Bearer" {
		t.Errorf("token = %q %q", tok.Type(), tok.AccessToken)
	}
	if remaining := time.Until(tok.Expiry); remaining < 59*time.Minute || remaining > time.Hour {
		t.Errorf("expiry in %v, want about an hour", remaining)
	}
}

func TestTokenErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		assertion AssertionFunc
		status    int
		body      string
	}{
		{
			name:      "assertion error",
			assertion: func(context.Context) (string, error) { return "", errMetadataDown },
			status:    http.StatusOK,
			body:      `{"access_token":"x"}`,
		},
		{name: "empty assertion", assertion: staticAssertion(""), status: http.StatusOK, body: `{"access_token":"x"}`},
		{
			name:      "refused",
			assertion: staticAssertion("jwt"),
			status:    http.StatusBadRequest,
			body:      `{"error":"invalid_client","error_description":"AADSTS700213: No matching federated identity record found"}`,
		},
		{name: "no access token", assertion: staticAssertion("jwt"), status: http.StatusOK, body: `{"token_type":"Bearer"}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(testCase.status)
				_, _ = w.Write([]byte(testCase.body))
			}))
			t.Cleanup(server.Close)

			tokenSource, err := New(context.Background(), server.URL, "app-1", []string{"s"}, testCase.assertion)
			if err != nil {
				t.Fatalf("new: %v", err)
			}
			if _, err := tokenSource.Token(); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestTokenCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	tokenSource, err := New(ctx, "https://login.example/t/oauth2/v2.0/token", "app-1", []string{"s"}, staticAssertion("jwt"))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	cancel()

	if _, err := tokenSource.Token(); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestNewValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		tokenUrl  string
		clientId  string
		scopes    []string
		assertion AssertionFunc
	}{
		{name: "no token url", clientId: "c", scopes: []string{"s"}, assertion: staticAssertion("a")},
		{name: "no client id", tokenUrl: "u", scopes: []string{"s"}, assertion: staticAssertion("a")},
		{name: "no scopes", tokenUrl: "u", clientId: "c", assertion: staticAssertion("a")},
		{name: "no assertion", tokenUrl: "u", clientId: "c", scopes: []string{"s"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(context.Background(), testCase.tokenUrl, testCase.clientId, testCase.scopes, testCase.assertion); err == nil {
				t.Error("expected error")
			}
		})
	}
}
