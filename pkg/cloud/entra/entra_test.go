package entra

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/entra/entra_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp"
)

func TestTokenUrl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		options  []entra_config.Option
		tenantId string
		want     string
	}{
		{
			name:     "default authority",
			tenantId: "72f988bf-86f1-41af-91ab-2d7cd011db47",
			want:     "https://login.microsoftonline.com/72f988bf-86f1-41af-91ab-2d7cd011db47/oauth2/v2.0/token",
		},
		{
			name:     "domain tenant",
			tenantId: "contoso.onmicrosoft.com",
			want:     "https://login.microsoftonline.com/contoso.onmicrosoft.com/oauth2/v2.0/token",
		},
		{
			name:     "national cloud",
			options:  []entra_config.Option{entra_config.WithAuthorityUrl(&url.URL{Scheme: "https", Host: "login.microsoftonline.us"})},
			tenantId: "t",
			want:     "https://login.microsoftonline.us/t/oauth2/v2.0/token",
		},
		{
			name:     "tenant cannot break out of its segment",
			tenantId: "a/../b",
			want:     "https://login.microsoftonline.com/a%2F..%2Fb/oauth2/v2.0/token",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := NewClient(testCase.options...).TokenUrl(testCase.tenantId); got != testCase.want {
				t.Errorf("TokenUrl = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestNewGoogleFederatedTokenSource(t *testing.T) {
	t.Parallel()

	const idToken = "google.id.token"

	metadataServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Metadata-Flavor") != "Google" {
			t.Errorf("missing Metadata-Flavor header")
		}
		if audience := r.URL.Query().Get("audience"); audience != FederatedAudience {
			t.Errorf("audience = %q, want %q", audience, FederatedAudience)
		}
		_, _ = w.Write([]byte(idToken))
	}))
	t.Cleanup(metadataServer.Close)

	tokenRequests := 0
	loginServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenRequests++
		if r.URL.Path != "/customer-tenant/oauth2/v2.0/token" {
			t.Errorf("token path = %q", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if assertion := r.PostForm.Get("client_assertion"); assertion != idToken {
			t.Errorf("client_assertion = %q, want the Google ID token", assertion)
		}
		if clientId := r.PostForm.Get("client_id"); clientId != "app-1" {
			t.Errorf("client_id = %q", clientId)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token_type":"Bearer","expires_in":3599,"access_token":"graph-token"}`))
	}))
	t.Cleanup(loginServer.Close)

	metadataUrl, err := url.Parse(metadataServer.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	loginUrl, err := url.Parse(loginServer.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	tokenSource, err := NewClient(entra_config.WithAuthorityUrl(loginUrl)).NewGoogleFederatedTokenSource(
		context.Background(),
		gcp.NewClientWithUrls(metadataUrl, gcp.DefaultTokenUrl),
		"customer-tenant",
		"app-1",
		[]string{ScopeMicrosoftGraph},
	)
	if err != nil {
		t.Fatalf("new token source: %v", err)
	}

	for range 2 {
		tok, err := tokenSource.Token()
		if err != nil {
			t.Fatalf("token: %v", err)
		}
		if tok.AccessToken != "graph-token" {
			t.Errorf("access token = %q", tok.AccessToken)
		}
	}
	if tokenRequests != 1 {
		t.Errorf("token requests = %d, want 1 (cached)", tokenRequests)
	}
}

func TestNewTokenSourceValidation(t *testing.T) {
	t.Parallel()

	client := NewClient()
	gcpClient := gcp.NewClient()

	testCases := []struct {
		name      string
		gcpClient *gcp.Client
		tenantId  string
		clientId  string
	}{
		{name: "no gcp client", tenantId: "t", clientId: "c"},
		{name: "no tenant", gcpClient: gcpClient, clientId: "c"},
		{name: "no client id", gcpClient: gcpClient, tenantId: "t"},
		{name: "common", gcpClient: gcpClient, tenantId: "common", clientId: "c"},
		{name: "organizations", gcpClient: gcpClient, tenantId: "Organizations", clientId: "c"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := client.NewGoogleFederatedTokenSource(
				context.Background(), testCase.gcpClient, testCase.tenantId, testCase.clientId, []string{ScopeMicrosoftGraph},
			)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}
