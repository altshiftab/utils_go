// Package public_fetch downloads a file someone names by its address -- a manual on a maker's site, a
// photo -- for a service to keep, from the public internet alone and within limits.
//
// The address is the caller's choice, so every connection is held to outbound_guard: one to a loopback,
// private, link-local or cloud metadata address is refused where it is made, after the name is resolved,
// which a redirect, or a name that resolves to such an address only when dialled, does not get past. The
// address and every redirect must be https, a redirect is followed only so many times, no proxy is used
// (the address the guard checks must be the one connected to), no cookie is kept or sent, and the
// download is given up after a timeout. The body is read through a limit one byte past the most a file
// may be, so a file too large is refused rather than cut short.
//
// What the file is, is the caller's to read off its bytes: the site's Content-Type is never given, being
// the site's word for it. A download refused for what the address or the site did -- not https, not
// public, a 404, too large, too slow -- is a *RefusalError, which is a validation error: the caller's to
// put right, with a message to show them, and not the service's failure. A failure of the service's own
// -- its caller's context ending, its sockets or memory running out -- is a plain error.
//
// The transport is made whole here rather than cloned from http.DefaultTransport, so that nothing set on
// that process-wide transport -- a DialTLSContext, which https would take instead of the guarded dialer,
// or a TLSClientConfig that skips verification -- reaches it.
package public_fetch

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"syscall"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	"github.com/altshiftab/utils_go/pkg/net/outbound_guard"
)

const (
	// DefaultTimeout is how long a download may take, from the first connection to the last byte.
	DefaultTimeout = 60 * time.Second
	// DefaultMaxRedirects is how many redirects are followed.
	DefaultMaxRedirects = 5

	dialTimeout           = 30 * time.Second
	keepAlive             = 30 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	idleConnectionTimeout = 90 * time.Second
	maxIdleConnections    = 10
)

var (
	// ErrInvalidUrl is an address that is no absolute URL of a host.
	ErrInvalidUrl = errors.New("invalid URL")
	// ErrNotHttps is an address, or a redirect, that is not https.
	ErrNotHttps = errors.New("not an https URL")
	// ErrUserInfo is an address with a user name or a password in it, which would be sent to the site.
	ErrUserInfo = errors.New("URL with user info")
	// ErrTooManyRedirects is a redirect past the most followed.
	ErrTooManyRedirects = errors.New("too many redirects")
	// ErrUnreachable is a site that could not be connected to, or did not answer.
	ErrUnreachable = errors.New("site unreachable")
)

// RefusalError is a download refused for what the address or the site did. It is a validation error
// (altshiftErrors.ErrValidationError) and its cause, one of this package's errors,
// outbound_guard.ErrNotPublic, altshiftHttpErrors.ErrResponseBodyTooLarge or a
// *altshiftHttpErrors.Non2xxStatusCodeError.
type RefusalError struct {
	// Message says what was wrong, in a sentence fit to show whoever gave the address.
	Message string
	Cause   error
}

func (e *RefusalError) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return e.Message + ": " + e.Cause.Error()
}

func (e *RefusalError) Unwrap() []error {
	return []error{altshiftErrors.ErrValidationError, e.Cause}
}

func refuse(cause error, format string, arguments ...any) *RefusalError {
	return &RefusalError{Message: fmt.Sprintf(format, arguments...), Cause: cause}
}

// File is a file downloaded.
type File struct {
	Data []byte
	// Name is what the site calls the file: the filename of its Content-Disposition, or else the last part
	// of the path of the address it came from; "" where neither names one. It is the site's word, the last
	// part of a path only, and a caller keeping it holds it to its own rules.
	Name string
	// Url is the address the file came from, after any redirects.
	Url string
}

// Fetcher downloads files. It is safe for concurrent use.
type Fetcher struct {
	client    *http.Client
	userAgent string
	maxBytes  int64
}

type config struct {
	timeout      time.Duration
	maxRedirects int
	dialControl  func(network string, address string, rawConn syscall.RawConn) error
	tlsConfig    *tls.Config
}

// Option changes how a Fetcher downloads.
type Option func(*config)

// WithTimeout sets how long a download may take, DefaultTimeout unless set; New refuses one of zero or
// less, which would be none.
func WithTimeout(timeout time.Duration) Option {
	return func(c *config) {
		c.timeout = timeout
	}
}

// WithMaxRedirects sets how many redirects are followed, DefaultMaxRedirects unless set; 0 follows none.
func WithMaxRedirects(maxRedirects int) Option {
	return func(c *config) {
		c.maxRedirects = maxRedirects
	}
}

// WithDialControl replaces outbound_guard.DialControl, which every connection is held to unless replaced.
// It is for tests, whose servers listen on loopback; a server given an address by its users must keep the
// guard.
func WithDialControl(control func(network string, address string, rawConn syscall.RawConn) error) Option {
	return func(c *config) {
		c.dialControl = control
	}
}

// WithTlsConfig sets the TLS configuration of the connections: for tests, to trust a test server's
// certificate.
func WithTlsConfig(tlsConfig *tls.Config) Option {
	return func(c *config) {
		c.tlsConfig = tlsConfig
	}
}

// New is a Fetcher sending userAgent -- the service and what it fetches for, never whose -- and refusing
// a file of more than maxBytes.
func New(userAgent string, maxBytes int64, options ...Option) (*Fetcher, error) {
	if strings.TrimSpace(userAgent) == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user agent"))
	}
	// One byte past the most is read, so the most must leave room for it.
	if maxBytes <= 0 || maxBytes == math.MaxInt64 {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: the most bytes is %d", altshiftErrors.ErrValidationError, maxBytes))
	}

	c := &config{timeout: DefaultTimeout, maxRedirects: DefaultMaxRedirects, dialControl: outbound_guard.DialControl}
	for _, option := range options {
		if option != nil {
			option(c)
		}
	}
	if c.dialControl == nil {
		c.dialControl = outbound_guard.DialControl
	}
	if c.timeout <= 0 {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: a timeout of %s", altshiftErrors.ErrValidationError, c.timeout))
	}

	dialer := &net.Dialer{Timeout: dialTimeout, KeepAlive: keepAlive, Control: c.dialControl}
	transport := &http.Transport{
		// No proxy: the guard checks the address connected to, which would otherwise be the proxy's. And
		// no DialTLSContext, DialTLS or Dial: every connection, an https one's included, is this dialer's.
		Proxy:               nil,
		DialContext:         dialer.DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        maxIdleConnections,
		IdleConnTimeout:     idleConnectionTimeout,
		TLSHandshakeTimeout: tlsHandshakeTimeout,
	}
	if c.tlsConfig != nil {
		transport.TLSClientConfig = c.tlsConfig.Clone()
	}

	maxRedirects := max(c.maxRedirects, 0)
	client := &http.Client{
		Transport: transport,
		Timeout:   c.timeout,
		// No Jar: no cookie a site sets is kept, so none is sent, a redirect's included.
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return refuse(ErrTooManyRedirects, "The address redirects more than %d times", maxRedirects)
			}
			if request.URL == nil || !strings.EqualFold(request.URL.Scheme, "https") {
				return refuse(ErrNotHttps, "The address redirects to one that is not https")
			}
			if request.URL.User != nil {
				return refuse(ErrUserInfo, "The address redirects to one with a user name or a password in it")
			}
			return nil
		},
	}

	return &Fetcher{client: client, userAgent: userAgent, maxBytes: maxBytes}, nil
}

// Fetch downloads the file at the address. A download refused for what the address or the site did is a
// *RefusalError; any other error is the caller's context ending, or a failure of the Fetcher's own.
func (f *Fetcher) Fetch(ctx context.Context, address string) (*File, error) {
	parsed, err := url.Parse(strings.TrimSpace(address))
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return nil, refuse(ErrInvalidUrl, "The address is not a URL")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return nil, refuse(ErrNotHttps, "Only an https address is fetched")
	}
	if parsed.User != nil {
		return nil, refuse(ErrUserInfo, "An address with a user name or a password in it is not fetched")
	}
	parsed.Fragment, parsed.RawFragment = "", ""

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, refuse(fmt.Errorf("%w: %w", ErrInvalidUrl, err), "The address is not a URL")
	}
	request.Header.Set("User-Agent", f.userAgent)
	request.Header.Set("Accept", "*/*")

	response, err := f.client.Do(request) //nolint:gosec // the address is a user's to choose; outbound_guard keeps it public
	if err != nil {
		return nil, f.explain(ctx, err)
	}
	if response == nil || response.Body == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("http response"))
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, refuse(
			&altshiftHttpErrors.Non2xxStatusCodeError{StatusCode: response.StatusCode},
			"The site answered %s", strings.TrimSpace(fmt.Sprintf("%d %s", response.StatusCode, http.StatusText(response.StatusCode))),
		)
	}
	if response.ContentLength > f.maxBytes {
		return nil, f.tooLarge()
	}
	// One byte past the limit tells a file that fits exactly from one that does not.
	data, err := io.ReadAll(io.LimitReader(response.Body, f.maxBytes+1))
	if err != nil {
		return nil, f.explain(ctx, err)
	}
	if int64(len(data)) > f.maxBytes {
		return nil, f.tooLarge()
	}

	final := parsed
	if response.Request != nil && response.Request.URL != nil {
		final = response.Request.URL
	}
	return &File{Data: data, Name: nameOf(response.Header.Get("Content-Disposition"), final), Url: final.String()}, nil
}

func (f *Fetcher) tooLarge() *RefusalError {
	const mebibyte = 1024 * 1024
	if f.maxBytes%mebibyte == 0 {
		return refuse(altshiftHttpErrors.ErrResponseBodyTooLarge, "The file is larger than %d MiB", f.maxBytes/mebibyte)
	}
	return refuse(altshiftHttpErrors.ErrResponseBodyTooLarge, "The file is larger than %d bytes", f.maxBytes)
}

// explain is the refusal a failed request or read is, or the error itself where the caller's context
// ended.
func (f *Fetcher) explain(ctx context.Context, err error) error {
	if refusal, ok := errors.AsType[*RefusalError](err); ok {
		return refusal
	}
	if errors.Is(err, outbound_guard.ErrNotPublic) {
		return refuse(err, "The address leads to a host outside the public internet, which is not fetched")
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("fetch: %w", ctxErr)
	}
	// The service's own sockets, ports or memory running out is no fault of the site's.
	for _, local := range localErrors {
		if errors.Is(err, local) {
			return fmt.Errorf("fetch: %w", err)
		}
	}
	if dnsError, ok := errors.AsType[*net.DNSError](err); ok && dnsError != nil {
		if dnsError.IsNotFound {
			return refuse(fmt.Errorf("%w: %w", ErrUnreachable, err), "The address's host name does not exist")
		}
		return refuse(fmt.Errorf("%w: %w", ErrUnreachable, err), "The address's host name could not be looked up")
	}
	// Which timer fired -- the connection's, the TLS handshake's, the whole download's -- the site was too
	// slow.
	if netError, ok := errors.AsType[net.Error](err); ok && netError != nil && netError.Timeout() {
		return refuse(fmt.Errorf("%w: %w", ErrUnreachable, err), "The site took too long to answer")
	}
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return refuse(fmt.Errorf("%w: %w", ErrUnreachable, err), "The site's certificate is not valid")
	}
	return refuse(fmt.Errorf("%w: %w", ErrUnreachable, err), "The site could not be reached")
}

// localErrors are the failures of the service's own resources a connection can meet, rather than the
// site's.
var localErrors = []error{syscall.EMFILE, syscall.ENFILE, syscall.ENOBUFS, syscall.ENOMEM, syscall.EADDRNOTAVAIL}

// nameOf is what a site calls the file it sent: the filename of the Content-Disposition, or else the last
// part of the address's path; "" where neither names one.
func nameOf(disposition string, address *url.URL) string {
	if disposition != "" {
		if _, parameters, err := mime.ParseMediaType(disposition); err == nil {
			if name := lastPart(parameters["filename"]); name != "" {
				return name
			}
		}
	}
	if address != nil {
		return lastPart(address.Path)
	}
	return ""
}

// lastPart is the last part of a path, either slash, trimmed; "" for none.
func lastPart(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, `\`, "/"))
	if name == "" {
		return ""
	}
	base := strings.TrimSpace(path.Base(name))
	if base == "/" || base == "." || base == ".." {
		return ""
	}
	return base
}
