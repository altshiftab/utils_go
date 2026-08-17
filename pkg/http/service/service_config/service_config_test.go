package service_config

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

func TestNew(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		options []Option
		check   func(*testing.T, *Config)
	}{
		{
			name: "defaults",
			check: func(t *testing.T, config *Config) {
				if config.ShutdownTimeout != DefaultShutdownTimeout {
					t.Errorf(
						"shutdown timeout: got %v, want %v",
						config.ShutdownTimeout,
						DefaultShutdownTimeout,
					)
				}
				if config.ReadHeaderTimeout != DefaultReadHeaderTimeout {
					t.Errorf(
						"read header timeout: got %v, want %v",
						config.ReadHeaderTimeout,
						DefaultReadHeaderTimeout,
					)
				}
				if len(config.Signals) != len(DefaultSignals) {
					t.Errorf("signals: got %v, want %v", config.Signals, DefaultSignals)
				}
				if config.Address != "" {
					t.Errorf("address: got %q, want empty", config.Address)
				}
				if config.Protocols != nil {
					t.Errorf("protocols: got %v, want none", config.Protocols)
				}
				if config.UnencryptedHttp2 {
					t.Error("unencrypted http/2")
				}
				if config.GeneralOptionsHandler {
					t.Error("the general options handler is enabled")
				}
				if config.ErrorLog != nil {
					t.Error("an error log")
				}
				if config.Endpoints != nil {
					t.Errorf("endpoints: got %+v, want none", config.Endpoints)
				}
				if config.Host != "" {
					t.Errorf("host: got %q, want empty", config.Host)
				}
				if config.Redirects != nil {
					t.Errorf("redirects: got %+v, want none", config.Redirects)
				}
				if !config.SecurityTxt {
					t.Error("no security.txt")
				}
			},
		},
		{
			name: "with endpoints",
			options: []Option{
				WithEndpoints(&endpoint.Endpoint{Path: "/first"}),
				WithEndpoints(&endpoint.Endpoint{Path: "/second"}),
			},
			check: func(t *testing.T, config *Config) {
				// Endpoints accumulate, so that an option adding some does not discard what an
				// earlier one added.
				if len(config.Endpoints) != 2 {
					t.Fatalf("endpoints: got %+v, want two", config.Endpoints)
				}
				if config.Endpoints[0].Path != "/first" || config.Endpoints[1].Path != "/second" {
					t.Errorf("endpoints: got %+v", config.Endpoints)
				}
			},
		},
		{
			name: "with the patching options",
			options: []Option{
				WithStrictTransportSecurity(true),
				WithRobotsTxt(true),
				WithSitemap(true),
				WithReporting(true),
			},
			check: func(t *testing.T, config *Config) {
				if !config.StrictTransportSecurity || !config.RobotsTxt || !config.Sitemap || !config.Reporting {
					t.Errorf("config: got %+v", config)
				}
			},
		},
		{
			name: "with security txt content",
			options: []Option{
				WithSecurityTxtContent(
					&altshiftHttpTypes.SecurityTxt{Contacts: []string{"mailto:security@example.com"}},
				),
			},
			check: func(t *testing.T, config *Config) {
				securityTxtContent := config.SecurityTxtContent
				if securityTxtContent == nil {
					t.Fatal("nil security txt content")
				}
				if len(securityTxtContent.Contacts) != 1 {
					t.Errorf("contacts: got %+v, want one", securityTxtContent.Contacts)
				}
			},
		},
		{
			// A service says how a vulnerability in it is reported unless it is told not to.
			name:    "without security txt",
			options: []Option{WithSecurityTxt(false)},
			check: func(t *testing.T, config *Config) {
				if config.SecurityTxt {
					t.Error("the security.txt was not turned off")
				}
			},
		},
		{
			name:    "with security txt url",
			options: []Option{WithSecurityTxtUrl(&url.URL{Scheme: "https", Host: "example.com"})},
			check: func(t *testing.T, config *Config) {
				securityTxtUrl := config.SecurityTxtUrl
				if securityTxtUrl == nil {
					t.Fatal("nil security txt url")
				}
				if got := securityTxtUrl.String(); got != "https://example.com" {
					t.Errorf("security txt url: got %q", got)
				}
			},
		},
		{
			name:    "with host",
			options: []Option{WithHost("example.com")},
			check: func(t *testing.T, config *Config) {
				if config.Host != "example.com" {
					t.Errorf("host: got %q, want %q", config.Host, "example.com")
				}
			},
		},
		{
			name: "with redirects",
			options: []Option{
				WithRedirects(&Redirect{Host: "www.example.com", To: "https://example.com"}),
				WithRedirects(&Redirect{Host: "old.example.com", To: "https://example.com"}),
			},
			check: func(t *testing.T, config *Config) {
				if len(config.Redirects) != 2 {
					t.Fatalf("redirects: got %+v, want two", config.Redirects)
				}
				if config.Redirects[0].Host != "www.example.com" || config.Redirects[1].Host != "old.example.com" {
					t.Errorf("redirects: got %+v", config.Redirects)
				}
			},
		},
		{
			name:    "nil options are skipped",
			options: []Option{nil, WithAddress(":8080"), nil},
			check: func(t *testing.T, config *Config) {
				if config.Address != ":8080" {
					t.Errorf("address: got %q, want %q", config.Address, ":8080")
				}
			},
		},
		{
			name:    "with address",
			options: []Option{WithAddress("127.0.0.1:8080")},
			check: func(t *testing.T, config *Config) {
				if config.Address != "127.0.0.1:8080" {
					t.Errorf("address: got %q, want %q", config.Address, "127.0.0.1:8080")
				}
			},
		},
		{
			name:    "with shutdown timeout",
			options: []Option{WithShutdownTimeout(time.Minute)},
			check: func(t *testing.T, config *Config) {
				if config.ShutdownTimeout != time.Minute {
					t.Errorf("shutdown timeout: got %v, want %v", config.ShutdownTimeout, time.Minute)
				}
			},
		},
		{
			name:    "with signals",
			options: []Option{WithSignals(syscall.SIGHUP, syscall.SIGUSR1)},
			check: func(t *testing.T, config *Config) {
				expected := []os.Signal{syscall.SIGHUP, syscall.SIGUSR1}
				if len(config.Signals) != len(expected) {
					t.Fatalf("signals: got %v, want %v", config.Signals, expected)
				}
				for i, signal := range expected {
					if config.Signals[i] != signal {
						t.Errorf("signal %d: got %v, want %v", i, config.Signals[i], signal)
					}
				}
			},
		},
		{
			name:    "with read header timeout",
			options: []Option{WithReadHeaderTimeout(time.Second)},
			check: func(t *testing.T, config *Config) {
				if config.ReadHeaderTimeout != time.Second {
					t.Errorf(
						"read header timeout: got %v, want %v",
						config.ReadHeaderTimeout,
						time.Second,
					)
				}
			},
		},
		{
			name: "with protocols",
			options: []Option{
				WithProtocols(func() *http.Protocols {
					protocols := new(http.Protocols)
					protocols.SetHTTP1(true)
					return protocols
				}()),
			},
			check: func(t *testing.T, config *Config) {
				protocols := config.Protocols
				if protocols == nil {
					t.Fatal("nil protocols")
				}
				if !protocols.HTTP1() {
					t.Error("http/1 is not spoken")
				}
			},
		},
		{
			name:    "with unencrypted http/2",
			options: []Option{WithUnencryptedHttp2(true)},
			check: func(t *testing.T, config *Config) {
				if !config.UnencryptedHttp2 {
					t.Error("no unencrypted http/2")
				}
			},
		},
		{
			name:    "with general options handler",
			options: []Option{WithGeneralOptionsHandler(true)},
			check: func(t *testing.T, config *Config) {
				if !config.GeneralOptionsHandler {
					t.Error("the general options handler is disabled")
				}
			},
		},
		{
			name:    "with certificate files",
			options: []Option{WithCertificateFiles("certificate.pem", "key.pem")},
			check: func(t *testing.T, config *Config) {
				if config.CertificateFile != "certificate.pem" {
					t.Errorf("certificate file: got %q", config.CertificateFile)
				}
				if config.KeyFile != "key.pem" {
					t.Errorf("key file: got %q", config.KeyFile)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			config := New(testCase.options...)
			if config == nil {
				t.Fatal("nil config")
			}

			testCase.check(t, config)
		})
	}
}

func TestWithErrorLog(t *testing.T) {
	t.Parallel()

	errorLog := log.New(io.Discard, "", 0)

	config := New(WithErrorLog(errorLog))
	if config == nil {
		t.Fatal("nil config")
	}
	if config.ErrorLog != errorLog {
		t.Error("the error log is not the configured one")
	}
}
