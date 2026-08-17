package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftMux "github.com/altshiftab/utils_go/pkg/http/mux"
	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	"github.com/altshiftab/utils_go/pkg/http/service/service_config"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	altshiftNet "github.com/altshiftab/utils_go/pkg/net"
	"github.com/altshiftab/utils_go/pkg/net/types/domain_parts"
)

// Service is a mux and the server that serves it. The server is made for being stopped: asking it
// to stop lets the requests it is handling finish rather than ending them where they are.
type Service struct {
	Server *http.Server
	Mux    *altshiftMux.Mux

	shutdownTimeout time.Duration
	signals         []os.Signal
	certificateFile string
	keyFile         string
}

// Serve serves until the process is asked to stop, and then lets the requests it is handling
// finish. A process is asked to stop whenever the instance it runs on is replaced or scaled in, and
// a request killed midway leaves whatever it was doing half done, so the signal is handled rather
// than left to end the process.
func (service *Service) Serve() error {
	signals := service.signals
	if len(signals) == 0 {
		signals = service_config.DefaultSignals
	}

	ctx, stop := signal.NotifyContext(context.Background(), signals...)
	defer stop()

	if err := service.ServeContext(ctx); err != nil {
		return fmt.Errorf("service serve context: %w", err)
	}

	return nil
}

// ServeContext serves on the configured address until ctx is cancelled, and then lets the requests
// it is handling finish. Unlike Serve, it leaves what makes the service stop to the caller.
func (service *Service) ServeContext(ctx context.Context) error {
	server := service.Server
	if server == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("http server"))
	}

	address := server.Addr
	if address == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("http server address"))
	}

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", address)
	if err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("listen config listen: %w", err), address)
	}

	if err := service.serve(ctx, listener); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

// ServeListener serves on listener until ctx is cancelled, and then lets the requests it is
// handling finish. It serves where the listener was obtained by other means than the service
// making it itself: a port picked by the operating system, a socket passed in by a supervisor.
func (service *Service) ServeListener(ctx context.Context, listener net.Listener) error {
	if listener == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("listener"))
	}

	if err := service.serve(ctx, listener); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

func (service *Service) serve(ctx context.Context, listener net.Listener) error {
	server := service.Server
	if server == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("http server"))
	}

	if listener == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("listener"))
	}

	serveTls := server.TLSConfig != nil || service.certificateFile != "" || service.keyFile != ""

	served := make(chan error, 1)
	go func() {
		var err error
		if serveTls {
			err = server.ServeTLS(listener, service.certificateFile, service.keyFile)
		} else {
			err = server.Serve(listener)
		}

		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			served <- altshiftErrors.NewWithTrace(fmt.Errorf("http server serve: %w", err))
			return
		}

		served <- nil
	}()

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}

	shutdownTimeout := service.shutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = service_config.DefaultShutdownTimeout
	}

	// The context serving was done under is cancelled by now; the shutdown is given one that keeps
	// its values -- what identifies the run, for whatever the requests still being handled log --
	// but not its cancellation.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	// Requests still being handled are given the remaining time; those that do not finish are ended
	// when the process is killed anyway.
	if err := server.Shutdown(shutdownCtx); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("http server shutdown: %w", err))
	}

	return nil
}

// withDuplicatedEndpoints returns the endpoints with each duplication's own added: what is served
// at a path, served at the other paths as well. It is done before the mux is made, so that what the
// service makes of what it serves -- a sitemap above all -- is made of the duplicates too.
func withDuplicatedEndpoints(
	endpoints []*endpointPkg.Endpoint,
	duplicatedEndpoints []*service_config.DuplicatedEndpoint,
) ([]*endpointPkg.Endpoint, error) {
	if len(duplicatedEndpoints) == 0 {
		return endpoints, nil
	}

	for _, duplicatedEndpoint := range duplicatedEndpoints {
		if duplicatedEndpoint == nil {
			continue
		}

		path := duplicatedEndpoint.Path
		if path == "" {
			return nil, altshiftErrors.NewWithTrace(empty_error.NewWithInstance("path", "duplicated endpoint"))
		}

		// Every endpoint at the path is duplicated, a path being answered by one per method.
		var duplicates []*endpointPkg.Endpoint
		for _, endpoint := range endpoints {
			if endpoint != nil && endpoint.Path == path {
				duplicates = append(duplicates, endpointPkg.Duplicate(endpoint, duplicatedEndpoint.To...)...)
			}
		}

		if len(duplicates) == 0 {
			// Serving nothing at the paths asked for would be found by a request arriving at a
			// "not found"; saying so here is found by starting the service.
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: no endpoint to duplicate", altshiftErrors.ErrValidationError),
				path,
			)
		}

		endpoints = append(endpoints, duplicates...)
	}

	return endpoints, nil
}

// makeBaseUrl is where the service is reached, derived from the host it answers for: a host that
// resolves on the machine itself is reached over HTTP, anything else over HTTPS.
func makeBaseUrl(host string) *url.URL {
	if host == "" {
		return nil
	}

	scheme := "https"
	if altshiftNet.IsLocalhost(host) {
		scheme = "http"
	}

	return &url.URL{Scheme: scheme, Host: host}
}

// securityTxtWithDefaults fills in what the service knows and the configuration left out: where a
// vulnerability is reported, how long the information is to be considered valid, and where the file
// is expected to be found.
func securityTxtWithDefaults(
	securityTxt *altshiftHttpTypes.SecurityTxt,
	baseUrl *url.URL,
	registeredDomain string,
) *altshiftHttpTypes.SecurityTxt {
	// The configured security.txt is the caller's; it is filled in as a copy.
	var filled altshiftHttpTypes.SecurityTxt
	if securityTxt != nil {
		filled = *securityTxt
	}

	// RFC 2142 reserves a security mailbox at every domain for exactly what a security.txt names a
	// contact for.
	if len(filled.Contacts) == 0 && registeredDomain != "" {
		filled.Contacts = []string{"mailto:security@" + registeredDomain}
	}

	if filled.Expires.IsZero() {
		filled.Expires = time.Now().Add(service_config.DefaultSecurityTxtValidity)
	}

	if len(filled.Canonical) == 0 && baseUrl != nil {
		if canonicalUrl := baseUrl.JoinPath(wellKnownSecurityTxtPath); canonicalUrl != nil {
			filled.Canonical = []string{canonicalUrl.String()}
		}
	}

	return &filled
}

// patchServiceSecurityTxt serves the service's security.txt, in whichever of its two forms the host
// calls for: a service on a registered domain says how a vulnerability in it is reported, and a
// service on a subdomain of one points at what the registered domain says, which is where a
// reporter looks first.
func patchServiceSecurityTxt(mux *altshiftMux.Mux, config *service_config.Config, baseUrl *url.URL) error {
	if mux == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("mux"))
	}

	if config == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("service config"))
	}

	if !config.SecurityTxt {
		return nil
	}

	if securityTxtUrl := config.SecurityTxtUrl; securityTxtUrl != nil {
		if err := patchSecurityTxtUrl(mux, securityTxtUrl.String()); err != nil {
			return fmt.Errorf("patch security txt url: %w", err)
		}

		return nil
	}

	content := config.SecurityTxtContent

	// A service with no host of its own, or one on the machine it runs on, has no domain to derive
	// a contact from and no reporter to derive it for. What is said outright is still served.
	if baseUrl == nil || altshiftNet.IsLocalhost(baseUrl.Hostname()) {
		if content == nil || len(content.Contacts) == 0 {
			return nil
		}

		if err := patchSecurityTxt(mux, securityTxtWithDefaults(content, baseUrl, "")); err != nil {
			return fmt.Errorf("patch security txt: %w", err)
		}

		return nil
	}

	hostname := baseUrl.Hostname()

	domainParts := domain_parts.New(hostname)
	if domainParts == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("domain parts"), hostname)
	}

	registeredDomain := domainParts.RegisteredDomain
	if registeredDomain == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("registered domain"), hostname)
	}

	if !strings.EqualFold(hostname, registeredDomain) {
		registeredBaseUrl := &url.URL{Scheme: baseUrl.Scheme, Host: registeredDomain}

		securityTxtUrl := registeredBaseUrl.JoinPath(wellKnownSecurityTxtPath)
		if securityTxtUrl == nil {
			return altshiftErrors.NewWithTrace(nil_error.New("security txt url"), registeredDomain)
		}

		if err := patchSecurityTxtUrl(mux, securityTxtUrl.String()); err != nil {
			return fmt.Errorf("patch security txt url: %w", err)
		}

		return nil
	}

	if err := patchSecurityTxt(mux, securityTxtWithDefaults(content, baseUrl, registeredDomain)); err != nil {
		return fmt.Errorf("patch security txt: %w", err)
	}

	return nil
}

// patchMux makes the mux answer with what the service was configured to answer with beyond what its
// endpoints were written to answer.
func patchMux(mux *altshiftMux.Mux, config *service_config.Config) error {
	if mux == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("mux"))
	}

	if config == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("service config"))
	}

	host := config.Host
	baseUrl := makeBaseUrl(host)

	// Which policy the service answers with settles first, so that what follows patches the policy
	// that is actually answered with rather than one that is about to be dropped.
	if config.ApiContentSecurityPolicy {
		if err := patchApiContentSecurityPolicy(mux); err != nil {
			return fmt.Errorf("patch api content security policy: %w", err)
		}
	}

	if config.RenderableProblemDetails {
		if err := patchRenderableProblemDetails(mux); err != nil {
			return fmt.Errorf("patch renderable problem details: %w", err)
		}
	}

	if err := patchViewerStyleHashes(mux, config.ChromeXmlViewer, config.EdgePdfViewer); err != nil {
		return fmt.Errorf("patch viewer style hashes: %w", err)
	}

	if err := patchTrustedTypes(mux, config.TrustedTypes...); err != nil {
		return fmt.Errorf("patch trusted types: %w", err)
	}

	if err := patchFedCm(mux, config.FedCmProviders...); err != nil {
		return fmt.Errorf("patch fed cm: %w", err)
	}

	if config.Reporting {
		if err := patchReporting(mux, config.IntegrityPolicyEnforced); err != nil {
			return fmt.Errorf("patch reporting: %w", err)
		}
	}

	// A browser told to reach localhost over HTTPS only cannot reach a development server at all,
	// and remembers so for as long as the max-age says.
	if config.StrictTransportSecurity && !altshiftNet.IsLocalhost(host) {
		if err := patchStrictTransportSecurity(mux); err != nil {
			return fmt.Errorf("patch strict transport security: %w", err)
		}
	}

	var sitemapUrl string
	if config.Sitemap {
		if baseUrl == nil {
			return altshiftErrors.NewWithTrace(empty_error.NewWithInstance("host", "sitemap"))
		}

		var err error
		sitemapUrl, err = patchSitemap(mux, baseUrl)
		if err != nil {
			return fmt.Errorf("patch sitemap: %w", err)
		}
	}

	if config.RobotsTxt {
		if err := patchRobotsTxt(mux, sitemapUrl); err != nil {
			return fmt.Errorf("patch robots txt: %w", err)
		}
	}

	if err := patchServiceSecurityTxt(mux, config, baseUrl); err != nil {
		return fmt.Errorf("patch service security txt: %w", err)
	}

	return nil
}

// makeHandler makes what the server serves with: the mux itself, or -- where the service answers
// for a host of its own -- a vhost mux that tells that host apart from the ones redirected away.
func makeHandler(
	serviceMux *altshiftMux.Mux,
	host string,
	redirects []*service_config.Redirect,
) (http.Handler, error) {
	if serviceMux == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("mux"))
	}

	if host == "" {
		if len(redirects) != 0 {
			// Without a host of its own, the service has nothing to tell the redirected hosts from.
			return nil, altshiftErrors.NewWithTrace(empty_error.NewWithInstance("host", "redirects"))
		}

		return serviceMux, nil
	}

	hostToSpecification := map[string]*altshiftMux.VhostMuxSpecification{host: {Mux: serviceMux}}

	for _, redirect := range redirects {
		if redirect == nil {
			continue
		}

		if redirect.Host == "" {
			return nil, altshiftErrors.NewWithTrace(empty_error.NewWithInstance("host", "redirect"))
		}

		if redirect.To == "" {
			return nil, altshiftErrors.NewWithTrace(
				empty_error.NewWithInstance("to", "redirect"),
				redirect.Host,
			)
		}

		hostToSpecification[redirect.Host] = &altshiftMux.VhostMuxSpecification{RedirectTo: redirect.To}
	}

	vhostMux := &altshiftMux.VhostMux{HostToSpecification: hostToSpecification}
	// The mux's default headers are the vhost mux's too, so that a redirected host is answered with
	// what the service is configured to answer with, and not with less.
	vhostMux.DefaultHeaders = serviceMux.DefaultHeaders

	return vhostMux, nil
}

func New(options ...service_config.Option) (*Service, error) {
	config := service_config.New(options...)
	if config == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("service config"))
	}

	if profile := config.Profile; profile != "" && !profile.IsValid() {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: service profile", altshiftErrors.ErrValidationError),
			profile,
		)
	}

	endpoints, err := withDuplicatedEndpoints(config.Endpoints, config.DuplicatedEndpoints)
	if err != nil {
		return nil, fmt.Errorf("with duplicated endpoints: %w", err)
	}

	serviceMux := altshiftMux.New(endpoints...)

	if err := patchMux(serviceMux, config); err != nil {
		return nil, fmt.Errorf("patch mux: %w", err)
	}

	handler, err := makeHandler(serviceMux, config.Host, config.Redirects)
	if err != nil {
		return nil, fmt.Errorf("make handler: %w", err)
	}

	protocols := config.Protocols
	if config.UnencryptedHttp2 {
		if protocols == nil {
			protocols = new(http.Protocols)
			protocols.SetHTTP1(true)
			protocols.SetHTTP2(true)
		} else {
			// The configured protocols belong to the caller; they are added to as a copy.
			protocolsCopy := *protocols
			protocols = &protocolsCopy
		}
		protocols.SetUnencryptedHTTP2(true)
	}

	errorLog := config.ErrorLog
	if errorLog == nil {
		errorLog = slog.NewLogLogger(slog.Default().Handler(), slog.LevelError)
	}

	server := &http.Server{
		Addr:                         config.Address,
		Handler:                      handler,
		Protocols:                    protocols,
		ReadHeaderTimeout:            config.ReadHeaderTimeout,
		DisableGeneralOptionsHandler: !config.GeneralOptionsHandler,
		ErrorLog:                     errorLog,
	}

	return &Service{
		Server:          server,
		Mux:             serviceMux,
		shutdownTimeout: config.ShutdownTimeout,
		signals:         config.Signals,
		certificateFile: config.CertificateFile,
		keyFile:         config.KeyFile,
	}, nil
}
