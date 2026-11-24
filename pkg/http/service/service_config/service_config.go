package service_config

import (
	"log"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"slices"
	"syscall"
	"time"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

type Option func(*Config)

// Redirect is a host the service answers with a permanent redirect to somewhere else, rather than
// with what its mux serves.
type Redirect struct {
	Host string
	To   string
}

// DuplicatedEndpoint is an endpoint served at paths besides the one it was given at. A document
// that a frontend routes on its own is served at each of the routes it routes, so that a request
// for one arrives at the document that routes it rather than at a "not found".
type DuplicatedEndpoint struct {
	// Path is where the endpoint being duplicated is served.
	Path string
	// To are the paths it is served at as well.
	To []string
}

// DefaultSecurityTxtValidity is how long a security.txt says its information is to be considered
// valid, where it does not say itself. RFC 9116 requires the field and recommends less than a year.
const DefaultSecurityTxtValidity = 365 * 24 * time.Hour

const (
	// DefaultShutdownTimeout bounds how long the requests being handled are given to finish once the
	// service has been asked to stop. It leaves room within the ten seconds a hosting platform
	// commonly allows an instance between asking it to stop and killing it; a platform that allows
	// longer is accommodated with WithShutdownTimeout.
	DefaultShutdownTimeout = 9 * time.Second

	// DefaultReadHeaderTimeout bounds how long a client may take to send the request headers, so
	// that a connection opened and then left mid-headers does not occupy the service indefinitely.
	DefaultReadHeaderTimeout = 5 * time.Second
)

// DefaultSignals are the signals whose delivery makes Serve stop serving. They are the ones a
// process is asked to stop with: SIGTERM by a supervisor replacing or scaling in the instance,
// SIGINT by a user at a terminal.
var DefaultSignals = []os.Signal{syscall.SIGTERM, syscall.SIGINT}

type Config struct {
	Endpoints []*endpoint.Endpoint
	// DuplicatedEndpoints are the endpoints served at paths besides the ones they were given at.
	// They are duplicated before the service makes anything of what it serves, so that a sitemap
	// lists them as it lists the rest.
	DuplicatedEndpoints []*DuplicatedEndpoint
	// Host is the host the service answers for. A request for any other host is answered with
	// "421 Misdirected Request", rather than by the mux.
	Host string
	// TrustForwardedHost makes Host above compared against what the forwarded
	// headers name rather than against the request's own Host. See
	// WithTrustForwardedHost.
	TrustForwardedHost bool
	// TrustForwardedFor makes the client of a request the entry of X-Forwarded-For the proxy
	// directly in front wrote, or the one before it past TrustedProxies. See WithTrustForwardedFor.
	TrustForwardedFor bool
	// TrustClientHeaders makes the client of a request X-Client-Ip-Address, and the log record what
	// the other client headers say. See WithTrustClientHeaders.
	TrustClientHeaders bool
	// TrustedProxies are the address ranges of proxies further out than the one directly in front,
	// whose X-Forwarded-For entries are trusted to name who connected to them. See
	// WithTrustedProxies.
	TrustedProxies []netip.Prefix
	Redirects      []*Redirect
	// Profile is what the service was set up as, for the record. What it decided is in the fields
	// below, which an option applied after it may have overridden.
	Profile Profile
	// StrictTransportSecurity makes the service tell browsers to reach it over HTTPS only. It is
	// not answered with on localhost, where it would pin a development server no browser can reach
	// over HTTPS.
	StrictTransportSecurity bool
	// ApiContentSecurityPolicy answers every response with a content security policy that permits
	// nothing at all, for a service that serves no documents. A policy is worth as much to such a
	// service as to one that does; what a document is answered with would say nothing there, being
	// answered with documents only.
	ApiContentSecurityPolicy bool
	// RobotsTxt makes the service serve a robots.txt. It tells crawlers to keep out, except where
	// Sitemap says there is something worth indexing.
	RobotsTxt bool
	// Sitemap makes the service serve a sitemap.xml of the documents it serves statically, and
	// invites the crawlers that honour it.
	Sitemap bool
	// RenderableProblemDetails answers an XML problem detail as application/xml rather than as the
	// application/problem+xml RFC 9457 gives it, which a browser downloads as a file rather than
	// renders. It is on by default: every service answers a request it will not serve with a
	// problem detail, whether or not it serves documents otherwise.
	RenderableProblemDetails bool
	// ChromeXmlViewer permits the styles Chrome's XML viewer applies to an XML response, without
	// which what it renders comes out unstyled. It is on by default, for the same reason: the XML
	// a browser renders from a service is commonly the problem detail it answered an error with.
	ChromeXmlViewer bool
	// EdgePdfViewer permits the styles Edge's PDF viewer applies to a PDF response. It is off by
	// default, a service that serves no PDF having no use for it.
	EdgePdfViewer bool
	// TrustedTypes are the trusted types policies the scripts of the service's documents are
	// required to use.
	TrustedTypes []string
	// FedCmProviders are the identity providers the service's documents may ask who the user is
	// through, using the browser's federated credential management.
	FedCmProviders []*url.URL
	// Reporting makes the service ask browsers to report what they block on its documents --
	// content security policy violations and integrity violations -- and serve the endpoints the
	// reports go to, along with the ones a page's own JavaScript reports its errors to.
	Reporting bool
	// IntegrityPolicyEnforced makes the service refuse to load a script its documents do not give
	// integrity metadata for, rather than only reporting one. It is on by default, a script whose
	// contents nothing vouches for being what the policy exists to keep out, and is turned off for
	// documents a browser fails to attach the metadata of: one that loses it blocks every script
	// and renders nothing, which report-only turns back into a report. See
	// WithIntegrityPolicyEnforced. It says nothing unless Reporting is on, the policy naming an
	// endpoint the reports go to.
	IntegrityPolicyEnforced bool
	// SecurityTxt makes the service say how a vulnerability in it is reported. It is on by default,
	// the host being enough to derive both what the file says and which of the two forms the
	// service serves; see WithSecurityTxt.
	SecurityTxt bool
	// SecurityTxtContent is what the service's security.txt says, for what is not to be derived.
	// The fields left unset are filled in from the host.
	SecurityTxtContent *altshiftHttpTypes.SecurityTxt
	// SecurityTxtUrl is where the service's security.txt is served instead of by the service
	// itself: both /security.txt and /.well-known/security.txt redirect there. It takes precedence
	// over SecurityTxtContent and over what the host would otherwise decide.
	SecurityTxtUrl    *url.URL
	Address           string
	ShutdownTimeout   time.Duration
	Signals           []os.Signal
	ReadHeaderTimeout time.Duration
	Protocols         *http.Protocols
	UnencryptedHttp2  bool
	// GeneralOptionsHandler enables the standard library's own handler for `OPTIONS *`, which
	// answers with a bare 200 and no Allow header. It is disabled by default, so that a handler
	// that says something more useful about what the service supports is reached instead.
	GeneralOptionsHandler bool
	ErrorLog              *log.Logger
	CertificateFile       string
	KeyFile               string
}

func New(options ...Option) *Config {
	config := &Config{
		RenderableProblemDetails: true,
		ChromeXmlViewer:          true,
		IntegrityPolicyEnforced:  true,
		SecurityTxt:              true,
		ShutdownTimeout:          DefaultShutdownTimeout,
		Signals:                  DefaultSignals,
		ReadHeaderTimeout:        DefaultReadHeaderTimeout,
	}

	for _, option := range options {
		if option != nil {
			option(config)
		}
	}

	return config
}

// WithEndpoints adds endpoints to the mux the service serves with.
func WithEndpoints(endpoints ...*endpoint.Endpoint) Option {
	return func(config *Config) {
		config.Endpoints = append(config.Endpoints, endpoints...)
	}
}

// WithDuplicatedEndpoint serves the endpoint given at path at each of the other paths as well. A
// document that a frontend routes on its own is served at each of the routes it routes, so that a
// request for one arrives at the document that routes it rather than at a "not found".
//
// The service reports a path it was given nothing at, rather than serving nothing there.
func WithDuplicatedEndpoint(path string, to ...string) Option {
	return func(config *Config) {
		config.DuplicatedEndpoints = append(
			config.DuplicatedEndpoints,
			&DuplicatedEndpoint{Path: path, To: to},
		)
	}
}

// WithHost sets the host the service answers for. A request for any other host is answered with
// "421 Misdirected Request", so that a service reached by a name it does not serve says so rather
// than serving whatever the name was meant to reach.
func WithHost(host string) Option {
	return func(config *Config) {
		config.Host = host
	}
}

// WithTrustForwardedHost makes the service decide which host a request is for from the `Forwarded`
// and `X-Forwarded-Host` headers, falling back to the request's own Host where they say nothing.
//
// It is needed behind a proxy that rewrites Host to an address of its own -- Firebase Hosting
// rewriting to a run.app URL, say -- where without it every request arrives for a host the service
// does not answer for and is refused with "421 Misdirected Request".
//
// It is off by default, and it should stay off unless BOTH hold: a proxy in front overwrites these
// headers on every request, and nothing can reach the service except through that proxy. Where the
// second does not hold, the headers are the client's to write, and the 421 stops being a check on
// the host and becomes only routing -- anything deciding on the host must then be able to answer
// for a host the client chose. A Cloud Run service reached at its run.app address with
// `X-Forwarded-Host: films.vvvp.se` or `Forwarded: host=films.vvvp.se` is served as films.vvvp.se.
//
// WithTrustedProxies makes the second hold again: with any trusted proxies, the forwarded host is
// believed only of a request whose rightmost X-Forwarded-For entry is inside them -- one Firebase
// Hosting forwarded, Google's front end having appended the edge's address -- and a request sent to
// the run.app address directly, which the front end records with the caller's address there, is
// looked up by its own Host and refused. Without trusted proxies the forwarded host is believed of
// every request, as described above.
func WithTrustForwardedHost(trustForwardedHost bool) Option {
	return func(config *Config) {
		config.TrustForwardedHost = trustForwardedHost
	}
}

// WithTrustForwardedFor makes the service log the client of a request as X-Forwarded-For names it,
// read from the right, rather than as the address the connection came from.
//
// It says that the proxy directly in front of the service appends the address it accepted the
// connection from as the last entry of X-Forwarded-For, or replaces the header with that address
// alone. The last entry is then the only one nobody but that proxy can write, and it is the client,
// unless it is a trusted proxy's address (WithTrustedProxies), in which case the client is the entry
// before it, and so on leftwards.
//
// Google's front end appends, for a Cloud Run service reached at its run.app address or through
// Firebase Hosting: a request sent with `X-Forwarded-For: 192.0.2.21` arrives with
// `192.0.2.21,<caller>`, so that the first entry is whatever the client chose. Through Firebase
// Hosting a request arrives with `<client>,<Firebase edge>`, Firebase replacing whatever the client
// sent and the front end appending the edge's address; the client is read past the edge only with
// the edges' range among the trusted proxies, and is the edge without it. reverse_proxy replaces the
// header with the client's address alone. A forward proxy passes on the client's own headers, and
// nothing in front of it is to be trusted with this.
//
// It is off by default. Being a fact about the deployment rather than about the service, it is
// commonly read by the service from its environment -- a TRUST_FORWARDED_FOR its deployment sets
// where such a proxy is directly in front -- and passed here.
func WithTrustForwardedFor(trustForwardedFor bool) Option {
	return func(config *Config) {
		config.TrustForwardedFor = trustForwardedFor
	}
}

// WithTrustedProxies sets the address ranges of the proxies further out than the one directly in
// front of the service whose X-Forwarded-For entries are trusted to name who connected to them:
// Firebase Hosting's edges, which Google's front end records as the last entry of a request Firebase
// forwarded (seen from 66.249.93.77 and 66.249.93.133). The client of a request is read past their
// entries, with WithTrustForwardedFor, and the forwarded host is believed only of a request that came
// through one, with WithTrustForwardedHost.
//
// A range is trusted with everything sent from it, so it is to be no wider than the proxies' own: a
// range shared with anything that sends requests with headers its user chose lets that user name
// any client, and any host. Each call replaces what an earlier one set; the prefixes are copied.
func WithTrustedProxies(prefixes ...netip.Prefix) Option {
	return func(config *Config) {
		config.TrustedProxies = slices.Clone(prefixes)
	}
}

// WithTrustClientHeaders makes the service log the client of a request as X-Client-Ip-Address,
// ahead of X-Forwarded-For, and, where that header names the client, record what X-Client-Port,
// X-Server-Port, X-Client-Geo-Country-Iso-Code, X-Client-Geo-Region-Iso-Code,
// X-Client-Geo-City-Name, X-Tls-Ja3-Fingerprint and X-Tls-Ja4-Fingerprint say of the client and the
// connection it made, each where it is on one line. X-Client-Geo-Location is not among them, the
// intil load balancer not setting it.
//
// It is safe only where BOTH hold: a load balancer in front sets these headers on every request,
// replacing what the client sent -- custom request headers with replace on a Google Cloud load
// balancer, as the intil load balancer sets them -- and nothing reaches the service except through
// it, its ingress being restricted to the load balancer. A header the load balancer does not set is
// the client's to write even then. Nothing sets them in front of Firebase Hosting or reverse_proxy,
// where they are always the client's.
//
// It is off by default. Being a fact about the deployment rather than about the service, it is
// commonly read by the service from its environment -- a TRUST_CLIENT_HEADERS its deployment sets
// where such a load balancer is the only way in -- and passed here.
func WithTrustClientHeaders(trustClientHeaders bool) Option {
	return func(config *Config) {
		config.TrustClientHeaders = trustClientHeaders
	}
}

// WithRedirects adds hosts the service answers with a permanent redirect rather than with what its
// mux serves. A host is required for them to be told apart from it.
func WithRedirects(redirects ...*Redirect) Option {
	return func(config *Config) {
		config.Redirects = append(config.Redirects, redirects...)
	}
}

// WithStrictTransportSecurity makes the service tell browsers to reach it over HTTPS only. It is
// not answered with on localhost, whatever is configured here.
func WithStrictTransportSecurity(strictTransportSecurity bool) Option {
	return func(config *Config) {
		config.StrictTransportSecurity = strictTransportSecurity
	}
}

// WithApiContentSecurityPolicy answers every response with a content security policy that permits
// nothing at all, for a service that serves no documents. It replaces the policy a document would
// otherwise be answered with, a browser enforcing every policy it is sent rather than the last.
func WithApiContentSecurityPolicy(apiContentSecurityPolicy bool) Option {
	return func(config *Config) {
		config.ApiContentSecurityPolicy = apiContentSecurityPolicy
	}
}

// WithRenderableProblemDetails answers an XML problem detail as application/xml rather than as the
// application/problem+xml RFC 9457 gives it. Chrome downloads the latter as a file instead of
// rendering it, as it does any XML media type it does not know; an error a person is looking at is
// worth more rendered than saved to disk. What it costs is that a client that asked for
// application/problem+xml is answered with a laxer type than it asked for.
func WithRenderableProblemDetails(renderableProblemDetails bool) Option {
	return func(config *Config) {
		config.RenderableProblemDetails = renderableProblemDetails
	}
}

// WithChromeXmlViewer permits the styles Chrome's XML viewer applies to an XML response.
func WithChromeXmlViewer(chromeXmlViewer bool) Option {
	return func(config *Config) {
		config.ChromeXmlViewer = chromeXmlViewer
	}
}

// WithEdgePdfViewer permits the styles Edge's PDF viewer applies to a PDF response, for a service
// that serves one.
func WithEdgePdfViewer(edgePdfViewer bool) Option {
	return func(config *Config) {
		config.EdgePdfViewer = edgePdfViewer
	}
}

// WithTrustedTypes requires the named trusted types policies of the scripts the service's documents
// run, so that what reaches a sink that would otherwise take a string has been through one of them.
func WithTrustedTypes(policies ...string) Option {
	return func(config *Config) {
		config.TrustedTypes = append(config.TrustedTypes, policies...)
	}
}

// WithFedCm lets the service's documents ask the named identity providers who the user is, through
// the browser's federated credential management. The providers are whichever the service federates
// to; nothing here is particular to any of them.
func WithFedCm(providerUrls ...*url.URL) Option {
	return func(config *Config) {
		config.FedCmProviders = append(config.FedCmProviders, providerUrls...)
	}
}

// WithRobotsTxt makes the service serve a robots.txt telling crawlers to keep out -- except where
// WithSitemap says otherwise, in which case the crawlers that honour sitemaps are invited.
func WithRobotsTxt(robotsTxt bool) Option {
	return func(config *Config) {
		config.RobotsTxt = robotsTxt
	}
}

// WithSitemap makes the service serve a sitemap.xml of the documents it serves statically. A host
// is required, the sitemap protocol wanting absolute locations.
func WithSitemap(sitemap bool) Option {
	return func(config *Config) {
		config.Sitemap = sitemap
	}
}

// WithReporting makes the service ask browsers to report what they block on its documents, and
// serve the endpoints the reports go to.
func WithReporting(reporting bool) Option {
	return func(config *Config) {
		config.Reporting = reporting
	}
}

// WithIntegrityPolicyEnforced says whether the service refuses to load a script its documents give
// no integrity metadata for, which it does by default, or only reports one.
//
// Turn it off for documents a browser fails to attach the metadata of. Safari 26 loses the
// metadata an import map and the preload scanner carry, so it blocks every chunk a split build
// imports and renders a blank page; report-only says the same thing without breaking it. See
// https://webkit.org/blog/17967/news-from-wwdc26-webkit-in-safari-27-beta/, where the preload
// scanner part of it is fixed.
func WithIntegrityPolicyEnforced(integrityPolicyEnforced bool) Option {
	return func(config *Config) {
		config.IntegrityPolicyEnforced = integrityPolicyEnforced
	}
}

// WithSecurityTxt makes the service say how a vulnerability in it is reported, which it does by
// default. The host decides which of the two forms it serves: a service on a registered domain
// serves a security.txt of its own, and a service on a subdomain of one redirects to the registered
// domain's, which is where a reporter looks first.
//
// What the file says is derived from the host too, unless WithSecurityTxtContent says otherwise:
// the contact is the security mailbox RFC 2142 reserves at the registered domain, the canonical URI
// is the service's own, and the information is valid for DefaultSecurityTxtValidity.
//
// A service with no host, or one on localhost, serves no security.txt unless a contact is
// configured: there is neither a domain to derive one from nor a reporter to derive it for.
func WithSecurityTxt(securityTxt bool) Option {
	return func(config *Config) {
		config.SecurityTxt = securityTxt
	}
}

// WithSecurityTxtContent says what the service's security.txt says, for the parts that are not to
// be derived from the host. It does not decide which form is served: a service on a subdomain
// redirects to the registered domain's whatever is set here.
func WithSecurityTxtContent(securityTxtContent *altshiftHttpTypes.SecurityTxt) Option {
	return func(config *Config) {
		config.SecurityTxtContent = securityTxtContent
	}
}

// WithSecurityTxtUrl points at a security.txt served somewhere the host does not lead to, rather
// than at what would be derived. Both paths redirect there.
func WithSecurityTxtUrl(securityTxtUrl *url.URL) Option {
	return func(config *Config) {
		config.SecurityTxtUrl = securityTxtUrl
	}
}

// WithAddress sets the address the service listens on, as accepted by net.Listen, e.g. ":8080" or
// "127.0.0.1:8080". It is required by Serve and ServeContext, which do the listening themselves,
// and unused by ServeListener, which is handed a listener.
func WithAddress(address string) Option {
	return func(config *Config) {
		config.Address = address
	}
}

// WithShutdownTimeout bounds how long the requests being handled are given to finish once the
// service has been asked to stop.
func WithShutdownTimeout(shutdownTimeout time.Duration) Option {
	return func(config *Config) {
		config.ShutdownTimeout = shutdownTimeout
	}
}

// WithSignals sets the signals whose delivery makes Serve stop serving.
func WithSignals(signals ...os.Signal) Option {
	return func(config *Config) {
		config.Signals = signals
	}
}

func WithReadHeaderTimeout(readHeaderTimeout time.Duration) Option {
	return func(config *Config) {
		config.ReadHeaderTimeout = readHeaderTimeout
	}
}

// WithProtocols sets the protocols the service speaks, in full. Setting them replaces the standard
// library's default set (HTTP/1 and HTTP/2 over TLS) rather than adding to it.
func WithProtocols(protocols *http.Protocols) Option {
	return func(config *Config) {
		config.Protocols = protocols
	}
}

// WithUnencryptedHttp2 makes the service speak HTTP/2 without TLS, to clients that begin with prior
// knowledge, alongside the protocols it speaks otherwise. A load balancer that terminates TLS and
// then talks to its backend in the clear commonly does so this way. It is harmless to plain
// HTTP/1.1 clients, which are served as before.
func WithUnencryptedHttp2(unencryptedHttp2 bool) Option {
	return func(config *Config) {
		config.UnencryptedHttp2 = unencryptedHttp2
	}
}

// WithGeneralOptionsHandler enables the standard library's own handler for `OPTIONS *`.
func WithGeneralOptionsHandler(generalOptionsHandler bool) Option {
	return func(config *Config) {
		config.GeneralOptionsHandler = generalOptionsHandler
	}
}

// WithErrorLog sets the logger the HTTP server reports its own errors to. Unless one is set, the
// errors are logged at error level through the slog default in effect when the service is made.
func WithErrorLog(errorLog *log.Logger) Option {
	return func(config *Config) {
		config.ErrorLog = errorLog
	}
}

// WithCertificateFiles makes the service serve TLS, using the certificate and key in the named
// files. A service whose server carries a TLS configuration that produces certificates by other
// means (http.Server.TLSConfig) serves TLS without them.
func WithCertificateFiles(certificateFile string, keyFile string) Option {
	return func(config *Config) {
		config.CertificateFile = certificateFile
		config.KeyFile = keyFile
	}
}
