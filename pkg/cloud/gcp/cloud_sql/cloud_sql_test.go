package cloud_sql

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/cloud_sql_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/types/connect_settings"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/types/generate_ephemeral_cert_request"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/types/generate_ephemeral_cert_response"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return NewClient(cloud_sql_config.WithBaseUrl(u))
}

func TestGetConnectSettings(t *testing.T) {
	t.Parallel()

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if want := "/sql/v1beta4/projects/proj/instances/db/connectSettings"; r.URL.Path != want {
			t.Errorf("path = %q, want %q", r.URL.Path, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.MarshalWrite(w, &connect_settings.ConnectSettings{
			Region:       "europe-north1",
			IpAddresses:  []*connect_settings.IpMapping{{Type: "PRIMARY", IpAddress: "192.0.2.1"}},
			ServerCaCert: &connect_settings.SslCert{Cert: "pem"},
		})
	})

	settings, err := client.GetConnectSettings(context.Background(), "proj", "db")
	if err != nil {
		t.Fatalf("GetConnectSettings: %v", err)
	}
	if settings.Region != "europe-north1" || len(settings.IpAddresses) != 1 || settings.IpAddresses[0].IpAddress != "192.0.2.1" {
		t.Errorf("settings = %+v", settings)
	}
}

func TestGenerateEphemeralCert(t *testing.T) {
	t.Parallel()

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if want := "/sql/v1beta4/projects/proj/instances/db:generateEphemeralCert"; r.URL.Path != want {
			t.Errorf("path = %q, want %q", r.URL.Path, want)
		}
		var request generate_ephemeral_cert_request.Request
		if err := json.UnmarshalRead(r.Body, &request); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if request.PublicKey != "public" || request.AccessToken != "token" {
			t.Errorf("request = %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.MarshalWrite(w, &generate_ephemeral_cert_response.Response{
			EphemeralCert: &generate_ephemeral_cert_response.SslCert{Cert: "cert"},
		})
	})

	response, err := client.GenerateEphemeralCert(context.Background(), "proj", "db", "public", "token")
	if err != nil {
		t.Fatalf("GenerateEphemeralCert: %v", err)
	}
	if response.EphemeralCert.Cert != "cert" {
		t.Errorf("cert = %q", response.EphemeralCert.Cert)
	}
}

func TestGenerateEphemeralCert_MissingCert(t *testing.T) {
	t.Parallel()

	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})

	if _, err := client.GenerateEphemeralCert(context.Background(), "proj", "db", "public", ""); err == nil {
		t.Fatal("want error for a response without a certificate")
	}
}

func TestClient_EmptyArguments(t *testing.T) {
	t.Parallel()

	client := NewClient()
	testCases := []struct {
		name string
		call func() error
	}{
		{name: "settings project", call: func() error { _, err := client.GetConnectSettings(context.Background(), "", "db"); return err }},
		{name: "settings instance", call: func() error { _, err := client.GetConnectSettings(context.Background(), "p", ""); return err }},
		{name: "cert project", call: func() error {
			_, err := client.GenerateEphemeralCert(context.Background(), "", "db", "k", "")
			return err
		}},
		{name: "cert instance", call: func() error {
			_, err := client.GenerateEphemeralCert(context.Background(), "p", "", "k", "")
			return err
		}},
		{name: "cert key", call: func() error {
			_, err := client.GenerateEphemeralCert(context.Background(), "p", "db", "", "")
			return err
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if testCase.call() == nil {
				t.Error("want error")
			}
		})
	}
}

func TestDatabaseUsername(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		principal string
		want      string
		wantErr   bool
	}{
		{name: "service account member", principal: "serviceAccount:app@proj.iam.gserviceaccount.com", want: "app@proj.iam"},
		{name: "bare service account", principal: "app@proj.iam.gserviceaccount.com", want: "app@proj.iam"},
		{name: "user member", principal: "user:V@Example.com", want: "v@example.com"},
		{name: "bare user", principal: "v@example.com", want: "v@example.com"},
		{name: "group member", principal: "group:admins@example.com", want: "admins@example.com"},
		{name: "unsupported type", principal: "domain:example.com", wantErr: true},
		{name: "not an email", principal: "user:someone", wantErr: true},
		{name: "empty", principal: "", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := DatabaseUsername(testCase.principal)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("DatabaseUsername(%q) = %q, want error", testCase.principal, got)
				}
				if !errors.Is(err, altshiftErrors.ErrValidationError) {
					t.Errorf("error %v is not a validation error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("DatabaseUsername(%q): %v", testCase.principal, err)
			}
			if got != testCase.want {
				t.Errorf("DatabaseUsername(%q) = %q, want %q", testCase.principal, got, testCase.want)
			}
		})
	}
}
