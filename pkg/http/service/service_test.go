package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	motmedelMux "github.com/altshiftab/utils_go/pkg/http/mux"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	muxResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	muxResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/service/service_config"
)

func noContentEndpoint() *endpoint.Endpoint {
	return &endpoint.Endpoint{
		Path:   "/",
		Method: http.MethodGet,
		Handler: func(_ *http.Request, _ []byte) (*muxResponse.Response, *muxResponseError.ResponseError) {
			return &muxResponse.Response{StatusCode: http.StatusNoContent}, nil
		},
	}
}

// serveListener serves the service on a port of its own, and returns the address it can be reached
// at. Serving is stopped, and checked to have stopped without error, when the test ends.
func serveListener(t *testing.T, service *Service) string {
	t.Helper()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen config listen: %v", err)
	}
	address := listener.Addr().String()

	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))

	served := make(chan error, 1)
	go func() { served <- service.ServeListener(ctx, listener) }()

	t.Cleanup(func() {
		cancel()

		// Stopping takes as long as the requests still being handled are given, so the wait is
		// longer than that -- shorter, and a connection that has not gone idle yet fails the test
		// for a shutdown that was going to finish.
		select {
		case err := <-served:
			if err != nil {
				t.Errorf("serve listener: %v", err)
			}
		case <-time.After(service_config.DefaultShutdownTimeout + 5*time.Second):
			t.Error("serving never stopped")
		}
	})

	return address
}

// responseSummary is what the tests observe of a response, the response itself being read and
// closed where it was made.
type responseSummary struct {
	proto      string
	statusCode int
	headers    http.Header
	overTls    bool
}

func doRequest(t *testing.T, client *http.Client, url string, host string) *responseSummary {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("http new request with context: %v", err)
	}
	if host != "" {
		request.Host = host
	}

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("client do: %v", err)
	}
	if response == nil {
		t.Fatal("nil response")
	}
	defer func() {
		if _, err := io.Copy(io.Discard, response.Body); err != nil {
			t.Errorf("io copy: %v", err)
		}
		if err := response.Body.Close(); err != nil {
			t.Errorf("response body close: %v", err)
		}
	}()

	return &responseSummary{
		proto:      response.Proto,
		statusCode: response.StatusCode,
		headers:    response.Header,
		overTls:    response.TLS != nil,
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	errorLog := log.New(io.Discard, "", 0)

	testCases := []struct {
		name    string
		options []service_config.Option
		check   func(*testing.T, *Service)
	}{
		{
			name: "defaults",
			check: func(t *testing.T, service *Service) {
				server := service.Server
				if server.Addr != "" {
					t.Errorf("address: got %q, want empty", server.Addr)
				}
				if server.ReadHeaderTimeout != service_config.DefaultReadHeaderTimeout {
					t.Errorf(
						"read header timeout: got %v, want %v",
						server.ReadHeaderTimeout,
						service_config.DefaultReadHeaderTimeout,
					)
				}
				if !server.DisableGeneralOptionsHandler {
					t.Error("the general options handler is enabled")
				}
				if server.ErrorLog == nil {
					t.Error("nil error log")
				}
				// Unset protocols leave the standard library's default set in effect.
				if server.Protocols != nil {
					t.Errorf("protocols: got %v, want none", server.Protocols)
				}
				if service.shutdownTimeout != service_config.DefaultShutdownTimeout {
					t.Errorf(
						"shutdown timeout: got %v, want %v",
						service.shutdownTimeout,
						service_config.DefaultShutdownTimeout,
					)
				}
				if len(service.signals) != len(service_config.DefaultSignals) {
					t.Errorf("signals: got %v, want %v", service.signals, service_config.DefaultSignals)
				}
				// Without a host to tell apart, the mux is served as it is.
				if server.Handler != http.Handler(service.Mux) {
					t.Errorf("handler: got %T, want the mux", server.Handler)
				}
			},
		},
		{
			name:    "address",
			options: []service_config.Option{service_config.WithAddress(":8080")},
			check: func(t *testing.T, service *Service) {
				if got := service.Server.Addr; got != ":8080" {
					t.Errorf("address: got %q, want %q", got, ":8080")
				}
			},
		},
		{
			name:    "endpoints",
			options: []service_config.Option{service_config.WithEndpoints(noContentEndpoint())},
			check: func(t *testing.T, service *Service) {
				if service.Mux.Get("/", http.MethodGet) == nil {
					t.Error("the endpoint did not reach the mux")
				}
			},
		},
		{
			name:    "host",
			options: []service_config.Option{service_config.WithHost("example.com")},
			check: func(t *testing.T, service *Service) {
				vhostMux, ok := service.Server.Handler.(*motmedelMux.VhostMux)
				if !ok {
					t.Fatalf("handler: got %T, want a vhost mux", service.Server.Handler)
				}
				specification := vhostMux.HostToSpecification["example.com"]
				if specification == nil {
					t.Fatal("the host is not among the vhost mux's")
				}
				if specification.Mux != http.Handler(service.Mux) {
					t.Error("the host is not served by the service's mux")
				}
			},
		},
		{
			name:    "unencrypted http/2",
			options: []service_config.Option{service_config.WithUnencryptedHttp2(true)},
			check: func(t *testing.T, service *Service) {
				protocols := service.Server.Protocols
				if protocols == nil {
					t.Fatal("nil protocols")
				}
				if !protocols.UnencryptedHTTP2() {
					t.Error("unencrypted http/2 is not spoken")
				}
				if !protocols.HTTP1() {
					t.Error("http/1 is not spoken")
				}
				// HTTP/2 over TLS must not be lost by the protocols being named at all.
				if !protocols.HTTP2() {
					t.Error("http/2 over tls is not spoken")
				}
			},
		},
		{
			name: "unencrypted http/2 added to configured protocols",
			options: []service_config.Option{
				service_config.WithProtocols(func() *http.Protocols {
					protocols := new(http.Protocols)
					protocols.SetHTTP1(true)
					return protocols
				}()),
				service_config.WithUnencryptedHttp2(true),
			},
			check: func(t *testing.T, service *Service) {
				protocols := service.Server.Protocols
				if protocols == nil {
					t.Fatal("nil protocols")
				}
				if !protocols.UnencryptedHTTP2() {
					t.Error("unencrypted http/2 is not spoken")
				}
				if !protocols.HTTP1() {
					t.Error("http/1 is not spoken")
				}
				if protocols.HTTP2() {
					t.Error("http/2 over tls is spoken, though it was not configured")
				}
			},
		},
		{
			name: "general options handler",
			options: []service_config.Option{
				service_config.WithGeneralOptionsHandler(true),
			},
			check: func(t *testing.T, service *Service) {
				if service.Server.DisableGeneralOptionsHandler {
					t.Error("the general options handler is disabled")
				}
			},
		},
		{
			name:    "error log",
			options: []service_config.Option{service_config.WithErrorLog(errorLog)},
			check: func(t *testing.T, service *Service) {
				if service.Server.ErrorLog != errorLog {
					t.Error("the error log is not the configured one")
				}
			},
		},
		{
			name: "shutdown timeout and signals",
			options: []service_config.Option{
				service_config.WithShutdownTimeout(time.Minute),
				service_config.WithSignals(syscall.SIGHUP),
			},
			check: func(t *testing.T, service *Service) {
				if service.shutdownTimeout != time.Minute {
					t.Errorf("shutdown timeout: got %v, want %v", service.shutdownTimeout, time.Minute)
				}
				if len(service.signals) != 1 || service.signals[0] != syscall.SIGHUP {
					t.Errorf("signals: got %v, want %v", service.signals, []os.Signal{syscall.SIGHUP})
				}
			},
		},
		{
			name: "certificate files",
			options: []service_config.Option{
				service_config.WithCertificateFiles("certificate.pem", "key.pem"),
			},
			check: func(t *testing.T, service *Service) {
				if service.certificateFile != "certificate.pem" {
					t.Errorf("certificate file: got %q", service.certificateFile)
				}
				if service.keyFile != "key.pem" {
					t.Errorf("key file: got %q", service.keyFile)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			service, err := New(testCase.options...)
			if err != nil {
				t.Fatalf("new: %v", err)
			}
			if service == nil {
				t.Fatal("nil service")
			}
			if service.Server == nil {
				t.Fatal("nil http server")
			}
			if service.Mux == nil {
				t.Fatal("nil mux")
			}

			testCase.check(t, service)
		})
	}
}

func TestNewWithUnusableRedirects(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		options []service_config.Option
	}{
		{
			name: "without a host of its own",
			options: []service_config.Option{
				service_config.WithRedirects(&service_config.Redirect{Host: "www.example.com", To: "https://example.com"}),
			},
		},
		{
			name: "without a host to redirect",
			options: []service_config.Option{
				service_config.WithHost("example.com"),
				service_config.WithRedirects(&service_config.Redirect{To: "https://example.com"}),
			},
		},
		{
			name: "without somewhere to redirect to",
			options: []service_config.Option{
				service_config.WithHost("example.com"),
				service_config.WithRedirects(&service_config.Redirect{Host: "www.example.com"}),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := New(testCase.options...); err == nil {
				t.Error("new: got no error, want one")
			}
		})
	}
}

// TestServesTheHostItAnswersFor verifies that a service configured with a host serves that host,
// redirects the hosts configured as redirects, and answers for no others.
func TestServesTheHostItAnswersFor(t *testing.T) {
	t.Parallel()

	service, err := New(
		service_config.WithEndpoints(noContentEndpoint()),
		service_config.WithHost("example.com"),
		service_config.WithRedirects(
			&service_config.Redirect{Host: "www.example.com", To: "https://example.com"},
		),
	)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	address := serveListener(t, service)

	// The redirect must be observed rather than followed to somewhere this test does not serve.
	client := &http.Client{
		Transport: &http.Transport{},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	testCases := []struct {
		name               string
		host               string
		expectedStatusCode int
		expectedLocation   string
	}{
		{
			name:               "the host it answers for",
			host:               "example.com",
			expectedStatusCode: http.StatusNoContent,
		},
		{
			name:               "a host it redirects",
			host:               "www.example.com",
			expectedStatusCode: http.StatusMovedPermanently,
			expectedLocation:   "https://example.com/",
		},
		{
			name:               "a host it does not answer for",
			host:               "elsewhere.example.com",
			expectedStatusCode: http.StatusMisdirectedRequest,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			response := doRequest(t, client, "http://"+address+"/", testCase.host)

			if response.statusCode != testCase.expectedStatusCode {
				t.Errorf("status code: got %d, want %d", response.statusCode, testCase.expectedStatusCode)
			}

			if location := response.headers.Get("Location"); location != testCase.expectedLocation {
				t.Errorf("location: got %q, want %q", location, testCase.expectedLocation)
			}
		})
	}
}

// TestServesUnencryptedHttp2 verifies that a service that speaks unencrypted HTTP/2 serves both it,
// to a client that begins with prior knowledge, and HTTP/1.1, on the same port.
func TestServesUnencryptedHttp2(t *testing.T) {
	t.Parallel()

	service, err := New(
		service_config.WithEndpoints(noContentEndpoint()),
		service_config.WithUnencryptedHttp2(true),
	)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	address := serveListener(t, service)

	unencryptedHttp2Protocols := new(http.Protocols)
	unencryptedHttp2Protocols.SetUnencryptedHTTP2(true)
	unencryptedHttp2Client := &http.Client{
		Transport: &http.Transport{Protocols: unencryptedHttp2Protocols},
	}

	response := doRequest(t, unencryptedHttp2Client, "http://"+address+"/", "")
	if response.proto != "HTTP/2.0" {
		t.Errorf("proto: got %q, want %q", response.proto, "HTTP/2.0")
	}

	http1Client := &http.Client{Transport: &http.Transport{}}

	response = doRequest(t, http1Client, "http://"+address+"/", "")
	if response.proto != "HTTP/1.1" {
		t.Errorf("proto: got %q, want %q", response.proto, "HTTP/1.1")
	}
}

func TestServesHttp1ByDefault(t *testing.T) {
	t.Parallel()

	service, err := New(service_config.WithEndpoints(noContentEndpoint()))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	address := serveListener(t, service)

	response := doRequest(t, &http.Client{Transport: &http.Transport{}}, "http://"+address+"/", "")
	if response.proto != "HTTP/1.1" {
		t.Errorf("proto: got %q, want %q", response.proto, "HTTP/1.1")
	}
	if response.statusCode != http.StatusNoContent {
		t.Errorf("status code: got %d, want %d", response.statusCode, http.StatusNoContent)
	}
}

// selfSignedCertificate makes a certificate for 127.0.0.1, with the pool that accepts it.
func selfSignedCertificate(t *testing.T) (*tls.Certificate, *x509.CertPool) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("x509 create certificate: %v", err)
	}

	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509 parse certificate: %v", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(certificate)

	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: privateKey, Leaf: certificate}, pool
}

// TestServesTls verifies that a service whose server carries a TLS configuration serves TLS,
// rather than serving the handshake as though it were a plain HTTP request.
func TestServesTls(t *testing.T) {
	t.Parallel()

	certificate, pool := selfSignedCertificate(t)

	service, err := New(service_config.WithEndpoints(noContentEndpoint()))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	service.Server.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{*certificate},
		MinVersion:   tls.VersionTLS12,
	}

	address := serveListener(t, service)

	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
	}

	response := doRequest(t, client, "https://"+address+"/", "")
	if response.statusCode != http.StatusNoContent {
		t.Errorf("status code: got %d, want %d", response.statusCode, http.StatusNoContent)
	}
	if !response.overTls {
		t.Error("the response was not served over tls")
	}
}
