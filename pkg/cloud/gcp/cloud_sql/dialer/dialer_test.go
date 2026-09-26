package dialer

import (
	"cmp"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json/v2"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/cloud_sql_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/dialer/dialer_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/types/connect_settings"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/types/generate_ephemeral_cert_request"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/types/generate_ephemeral_cert_response"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token_source"
)

const (
	testConnectionName = "proj:europe-north1:db"
	testDnsName        = "abc123.europe-north1.sql.goog."
)

type authority struct {
	certificate *x509.Certificate
	key         *ecdsa.PrivateKey
	pem         string
}

func newAuthority(t *testing.T, name string) *authority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create ca: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse ca: %v", err)
	}
	return &authority{
		certificate: certificate,
		key:         key,
		pem:         string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
	}
}

func (a *authority) sign(t *testing.T, template *x509.Certificate, publicKey any) []byte {
	t.Helper()
	template.SerialNumber = big.NewInt(time.Now().UnixNano())
	template.NotBefore = time.Now().Add(-time.Hour)
	if template.NotAfter.IsZero() {
		template.NotAfter = time.Now().Add(time.Hour)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, a.certificate, publicKey, a.key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return der
}

// fakeInstance is the Admin API and the server-side proxy of one instance.
type fakeInstance struct {
	adminUrl *url.URL
	port     string

	mu            sync.Mutex
	settingsCalls int
	certCalls     int
	accessTokens  []string
}

type fakeOptions struct {
	serverCommonName string
	serverDnsNames   []string
	settingsDnsName  string
	backendType      string
	// advertisedCa replaces the CA the Admin API reports, to model a server whose certificate it does not sign.
	advertisedCa *authority
}

func newFakeInstance(t *testing.T, options fakeOptions) *fakeInstance {
	t.Helper()

	serverCa := newAuthority(t, "server ca")
	clientCa := newAuthority(t, "client ca")

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	serverDer := serverCa.sign(t, &x509.Certificate{
		Subject:     pkix.Name{CommonName: options.serverCommonName},
		DNSNames:    options.serverDnsNames,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}, &serverKey.PublicKey)

	clientRoots := x509.NewCertPool()
	clientRoots.AddCert(clientCa.certificate)
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{serverDer}, PrivateKey: serverKey}},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientRoots,
		MinVersion:   tls.VersionTLS13,
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				if err := conn.(*tls.Conn).HandshakeContext(context.Background()); err != nil {
					return
				}
				_, _ = conn.Write([]byte("ok"))
			}()
		}
	}()
	_, port, _ := net.SplitHostPort(listener.Addr().String())

	advertised := serverCa
	if options.advertisedCa != nil {
		advertised = options.advertisedCa
	}

	instance := &fakeInstance{port: port}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/connectSettings"):
			instance.mu.Lock()
			instance.settingsCalls++
			instance.mu.Unlock()
			_ = json.MarshalWrite(w, &connect_settings.ConnectSettings{
				Region:       "europe-north1",
				BackendType:  cmp.Or(options.backendType, "SECOND_GEN"),
				DnsName:      options.settingsDnsName,
				IpAddresses:  []*connect_settings.IpMapping{{Type: "PRIMARY", IpAddress: "127.0.0.1"}},
				ServerCaCert: &connect_settings.SslCert{Cert: advertised.pem},
			})
		case strings.HasSuffix(r.URL.Path, ":generateEphemeralCert"):
			var request generate_ephemeral_cert_request.Request
			if err := json.UnmarshalRead(r.Body, &request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			block, _ := pem.Decode([]byte(request.PublicKey))
			if block == nil || block.Type != "RSA PUBLIC KEY" {
				http.Error(w, "bad public key", http.StatusBadRequest)
				return
			}
			publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			instance.mu.Lock()
			instance.certCalls++
			instance.accessTokens = append(instance.accessTokens, request.AccessToken)
			instance.mu.Unlock()
			der := clientCa.sign(t, &x509.Certificate{
				Subject:     pkix.Name{CommonName: "client"},
				ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
				KeyUsage:    x509.KeyUsageDigitalSignature,
			}, publicKey)
			_ = json.MarshalWrite(w, &generate_ephemeral_cert_response.Response{
				EphemeralCert: &generate_ephemeral_cert_response.SslCert{
					Cert: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	adminUrl, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	instance.adminUrl = adminUrl

	return instance
}

func (f *fakeInstance) dialer(t *testing.T, loginTokenSource token_source.TokenSource, options ...dialer_config.Option) *Dialer {
	t.Helper()
	d, err := New(
		cloud_sql.NewClient(cloud_sql_config.WithBaseUrl(f.adminUrl)),
		loginTokenSource,
		append([]dialer_config.Option{dialer_config.WithPort(f.port)}, options...)...,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return d
}

func readAll(t *testing.T, conn net.Conn) string {
	t.Helper()
	defer func() { _ = conn.Close() }()
	data, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(data)
}

func TestDial_Verification(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		options fakeOptions
		wantErr string
	}{
		{
			name:    "dns name in san",
			options: fakeOptions{serverDnsNames: []string{strings.TrimSuffix(testDnsName, ".")}, settingsDnsName: testDnsName},
		},
		{
			name:    "legacy common name",
			options: fakeOptions{serverCommonName: "proj:db"},
		},
		{
			name:    "dns name advertised, certificate has only the legacy common name",
			options: fakeOptions{serverCommonName: "proj:db", settingsDnsName: testDnsName},
		},
		{
			name:    "common name of another instance",
			options: fakeOptions{serverCommonName: "proj:other"},
			wantErr: "neither",
		},
		{
			name:    "san of another instance",
			options: fakeOptions{serverDnsNames: []string{"other.europe-north1.sql.goog"}, settingsDnsName: testDnsName},
			wantErr: "neither",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			instance := newFakeInstance(t, testCase.options)
			conn, err := instance.dialer(t, nil).Dial(context.Background(), testConnectionName)
			if testCase.wantErr != "" {
				if err == nil {
					_ = conn.Close()
					t.Fatalf("Dial succeeded, want error containing %q", testCase.wantErr)
				}
				if !strings.Contains(err.Error(), testCase.wantErr) {
					t.Fatalf("Dial error = %v, want it to contain %q", err, testCase.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			if got := readAll(t, conn); got != "ok" {
				t.Errorf("read %q, want ok", got)
			}
		})
	}
}

func TestDial_UntrustedServerCaRefetches(t *testing.T) {
	t.Parallel()

	instance := newFakeInstance(t, fakeOptions{serverCommonName: "proj:db", advertisedCa: newAuthority(t, "other ca")})
	d := instance.dialer(t, nil)

	for range 2 {
		if conn, err := d.Dial(context.Background(), testConnectionName); err == nil {
			_ = conn.Close()
			t.Fatal("Dial succeeded against a certificate the advertised CA did not sign")
		}
	}

	instance.mu.Lock()
	defer instance.mu.Unlock()
	if instance.settingsCalls != 2 {
		t.Errorf("connect settings fetched %d times, want 2 (a failed handshake drops the cache)", instance.settingsCalls)
	}
}

func TestDial_CertificateCaching(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		tokenLifetime time.Duration
		wantCertCalls int
	}{
		{name: "token outlives the refresh buffer", tokenLifetime: time.Hour, wantCertCalls: 1},
		{name: "token inside the refresh buffer", tokenLifetime: time.Minute, wantCertCalls: 2},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			instance := newFakeInstance(t, fakeOptions{serverCommonName: "proj:db"})
			loginTokenSource := token_source.NewStatic(&token.Token{
				AccessToken: "login-token",
				Expiry:      time.Now().Add(testCase.tokenLifetime),
			})
			d := instance.dialer(t, loginTokenSource)

			for range 2 {
				conn, err := d.Dial(context.Background(), testConnectionName)
				if err != nil {
					t.Fatalf("Dial: %v", err)
				}
				if got := readAll(t, conn); got != "ok" {
					t.Errorf("read %q, want ok", got)
				}
			}

			instance.mu.Lock()
			defer instance.mu.Unlock()
			if instance.certCalls != testCase.wantCertCalls {
				t.Errorf("ephemeral certificates generated %d times, want %d", instance.certCalls, testCase.wantCertCalls)
			}
			for _, accessToken := range instance.accessTokens {
				if accessToken != "login-token" {
					t.Errorf("certificate requested with access token %q, want login-token", accessToken)
				}
			}
		})
	}
}

func TestDial_FirstGenerationInstance(t *testing.T) {
	t.Parallel()

	instance := newFakeInstance(t, fakeOptions{serverCommonName: "proj:db", backendType: "FIRST_GEN"})
	if conn, err := instance.dialer(t, nil).Dial(context.Background(), testConnectionName); err == nil {
		_ = conn.Close()
		t.Fatal("Dial succeeded against a first-generation instance")
	}
}

func TestDial_Errors(t *testing.T) {
	t.Parallel()

	instance := newFakeInstance(t, fakeOptions{serverCommonName: "proj:db"})

	testCases := []struct {
		name           string
		connectionName string
		options        []dialer_config.Option
	}{
		{name: "malformed connection name", connectionName: "proj:db"},
		{name: "region mismatch", connectionName: "proj:us-central1:db"},
		{name: "no private address", connectionName: testConnectionName, options: []dialer_config.Option{dialer_config.WithIpType(dialer_config.IpTypePrivate)}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			conn, err := instance.dialer(t, nil, testCase.options...).Dial(context.Background(), testCase.connectionName)
			if err == nil {
				_ = conn.Close()
				t.Fatal("Dial succeeded, want error")
			}
		})
	}
}

func TestNew_NilClient(t *testing.T) {
	t.Parallel()

	if _, err := New(nil, nil); err == nil {
		t.Fatal("New(nil) succeeded, want error")
	}
}
