// Package forwarded_headers reads what the proxies in front of a service say
// about the request the client actually made: RFC 7239's `Forwarded`, the
// older `X-Forwarded-*` headers it replaced, and the `X-Client-*` headers a
// load balancer sets.
//
// It is one package rather than a helper in each caller because which header
// answers which question is decided once, and two copies of the decision are
// two chances to answer the same request differently. The scheme and the host
// are read from `Forwarded` first, then from `X-Forwarded-*`; the client's
// address is not read from `Forwarded` at all, for the reason ClientAddress
// gives.
package forwarded_headers

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"

	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/forwarded"
)

// normalizeScheme reduces a proto to the two a browser can have reached a
// service with, and discards anything else. The value arrives in a header, so
// it is the client's to choose whenever the request did not come through a
// proxy that overwrites it, and it ends up in a URL: `javascript` is a proto a
// header can carry and must not be one this returns.
func normalizeScheme(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "https":
		return "https"
	case "http":
		return "http"
	default:
		return ""
	}
}

// isPort says whether a value is a port as an authority may carry one: digits
// and nothing else, possibly none (RFC 3986, section 3.2.3).
func isPort(value string) bool {
	for i := range len(value) {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}

	return true
}

// normalizeHost drops a port and lowercases what is left, so that a host read
// from a header and a host written in a configuration compare equal.
//
// An authority whose port is not a port yields no host at all. SplitHostPort
// takes whatever follows the last colon as the port, so without the check an
// authority could carry any text after a host that matches, pass every
// comparison made on the host, and be repeated as it was wherever the
// authority is: in the forwarded headers a proxy sends upstream, in a URL
// built back to the service.
func normalizeHost(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	bracketed := strings.HasPrefix(value, "[")

	if host, port, err := net.SplitHostPort(value); err == nil {
		if !isPort(port) {
			return ""
		}
		value = host
	} else if bracketed && strings.HasSuffix(value, "]") {
		// An IPv6 literal carries brackets in an authority and cannot in a
		// name. SplitHostPort takes them off where there is a port to split;
		// with no port there is nothing to split, and "[::1]" would otherwise
		// compare unequal to the "::1" the same address is configured as.
		value = value[1 : len(value)-1]
	}

	// Brackets hold an IPv6 address and nothing else. A name written in them
	// would otherwise compare equal to the name, and travel on in a form
	// neither a URL parser nor a server behind would read as that name.
	if bracketed {
		if address, err := netip.ParseAddr(value); err != nil || !address.Is6() {
			return ""
		}
	}

	return strings.ToLower(value)
}

// firstListItem returns the leftmost item of a list-valued field, across every
// field line it was given on. The leftmost is the one added closest to the
// client, and the only one that describes what the client itself did.
func firstListItem(header http.Header, name string) string {
	for _, line := range header.Values(name) {
		for item := range strings.SplitSeq(line, ",") {
			if item = strings.TrimSpace(item); item != "" {
				return item
			}
		}
	}

	return ""
}

// elements returns the parsed `Forwarded` elements, or nothing at all where the
// header is absent or does not parse. A proxy emitting a malformed header is
// treated as one that said nothing, rather than as a reason to refuse the
// request: the X-Forwarded-* fallback, or the connection itself, still answers
// the question.
func elements(header http.Header) []*altshiftHttpTypes.ForwardedElement {
	// Proxies may each add their own field line rather than appending to one.
	// For a list-valued field those forms mean the same thing, and joining them
	// is what a recipient is permitted to do -- without it, a `proto` or `host`
	// stated by the second proxy is invisible behind a first that stated
	// neither.
	value := strings.Join(header.Values("Forwarded"), ", ")
	if value == "" {
		return nil
	}

	parsed, err := forwarded.Parse([]byte(value))
	if err != nil || parsed == nil {
		return nil
	}

	return parsed.Elements
}

// Scheme reports the scheme the client used, as the proxies in front describe
// it. An empty return says only that nothing in front said anything usable,
// which the caller answers for -- it is the caller that knows whether a request
// without a proto is one to refuse or one to read off the connection.
func Scheme(header http.Header) string {
	// Elements are appended as the request passes through, so the leftmost is
	// the one added closest to the client and the only one that saw what the
	// client actually spoke.
	for _, element := range elements(header) {
		if element == nil {
			continue
		}

		if scheme := normalizeScheme(element.Proto); scheme != "" {
			return scheme
		}
	}

	return normalizeScheme(firstListItem(header, "X-Forwarded-Proto"))
}

// Authority reports the authority the client addressed -- the host as it was
// given, port and all -- selected the same way Host selects, and normalized no
// further. It is what belongs in a URL built to point back at this service: a
// development server reached on localhost:8080 is not reachable on localhost.
//
// Host is the same value with the port dropped and the case folded, which is
// what belongs in a comparison against a configured name.
func Authority(request *http.Request, trustForwarded bool) string {
	if request == nil {
		return ""
	}

	if trustForwarded {
		for _, element := range elements(request.Header) {
			if element == nil {
				continue
			}

			if host := strings.TrimSpace(element.Host); host != "" {
				return host
			}
		}

		if host := firstListItem(request.Header, "X-Forwarded-Host"); host != "" {
			return host
		}
	}

	return strings.TrimSpace(request.Host)
}

// Host reports the host the client asked for.
//
// With trustForwarded false it is the request's own Host and nothing else,
// which is the only answer worth having when the service is reachable
// directly: the forwarded headers are then the client's to write, and a service
// that believes them believes whatever it is told.
//
// With it true the forwarded headers win where they say anything. Set it only
// where a proxy in front overwrites them on every request AND nothing can reach
// the service except through that proxy. Where the second does not hold -- a
// Cloud Run service that must accept public traffic to be reachable by the
// proxy at all, say -- this stops being a check on the host and becomes only a
// way to learn the name the client used.
func Host(request *http.Request, trustForwarded bool) string {
	return HostFromAuthority(Authority(request, trustForwarded))
}

// HostFromAuthority drops the port from an authority and folds its case, so
// that a host read from a request and a host written in a configuration
// compare equal. It is separate from Host so that a caller needing both does
// not resolve the authority twice. An authority whose port is not numeric
// yields the empty string, which matches no configured host.
func HostFromAuthority(authority string) string {
	return normalizeHost(authority)
}

// parseClientAddress reads an address as a proxy writes a client's: bare, with
// a port, or bracketed, in any of the forms an IPv6 address takes. An
// IPv4-mapped IPv6 address is the IPv4 address it maps, so that one client is
// logged as one address. A zone names an interface of the machine that wrote
// it, which no proxy in front has any business naming, so an address with one
// is not an address of a client.
func parseClientAddress(value string) (netip.Addr, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return netip.Addr{}, false
	}

	address, err := netip.ParseAddr(value)
	if err != nil {
		if addressPort, portErr := netip.ParseAddrPort(value); portErr == nil {
			address = addressPort.Addr()
		} else if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			// Brackets hold an IPv6 address and nothing else, as in an
			// authority.
			bracketed, bracketedErr := netip.ParseAddr(value[1 : len(value)-1])
			if bracketedErr != nil || !bracketed.Is6() {
				return netip.Addr{}, false
			}
			address = bracketed
		} else {
			return netip.Addr{}, false
		}
	}

	if address.Zone() != "" {
		return netip.Addr{}, false
	}

	return address.Unmap(), true
}

// ClientAddress returns the address of the client that made the request, as
// the proxies in front report it and the caller says they are to be believed,
// or the empty string where nothing believed reports one -- in which case the
// client is the connection's own peer, and the caller reads it from there.
//
// With trustClientHeaders, X-Client-Ip-Address answers first. Set it only where
// a load balancer in front sets that header on every request, replacing what
// the client sent, AND nothing reaches the service except through it. A header
// given on more than one line was not replaced by one proxy, and answers
// nothing.
//
// With trustForwardedFor, the first entry of X-Forwarded-For answers next,
// across every line of the header. Set it only where every path to the service
// passes a proxy that rewrites the header so that its first entry is the
// address that connected to it: Google's front end on Cloud Run, reached at the
// run.app address or through Firebase Hosting, or a reverse proxy that replaces
// the header. A proxy that appends to the header -- an external Application
// Load Balancer, or a forward proxy passing on what the client sent -- leaves
// the first entry the client's to write.
//
// Only the first entry is read, an empty list element being none (RFC 9110,
// section 5.6.1). One that is not an address -- `unknown`, an obfuscated
// identifier, anything else -- answers nothing, rather than letting a later
// entry answer instead: the later entries are the proxies the request passed
// through, and none of them is the client.
//
// `Forwarded` is not read for the address at all, whatever is trusted. A proxy
// that rewrites X-Forwarded-For need not touch `Forwarded`, and one that does
// not passes on what the client wrote there, which would then answer for the
// client as though a proxy had.
func ClientAddress(header http.Header, trustForwardedFor bool, trustClientHeaders bool) string {
	if trustClientHeaders {
		if lines := header.Values("X-Client-Ip-Address"); len(lines) == 1 {
			if address, ok := parseClientAddress(lines[0]); ok {
				return address.String()
			}
		}
	}

	if trustForwardedFor {
		if address, ok := parseClientAddress(firstListItem(header, "X-Forwarded-For")); ok {
			return address.String()
		}
	}

	return ""
}

type authorityContextType struct{}

// AuthorityContextKey names the authority a mux in front resolved for the
// request. Prefer NewContext and AuthorityFromContext to reading it directly.
var AuthorityContextKey authorityContextType

// NewContext returns a context carrying the authority the client addressed.
//
// It is set by whatever decided which host the request is for -- the vhost mux
// -- so that code further in does not have to decide again. Two places reading
// the headers separately is two places that can be configured to disagree,
// and a service that routes on one host while building URLs from another is
// broken in a way neither half reports.
func NewContext(ctx context.Context, authority string) context.Context {
	return context.WithValue(ctx, AuthorityContextKey, authority)
}

// AuthorityFromContext returns the authority a mux in front resolved, and
// whether one was resolved at all. A false means nothing in front decided,
// which is the ordinary case for a service that is not behind a vhost mux; the
// caller should fall back to the request rather than treat it as an error.
func AuthorityFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}

	authority, ok := ctx.Value(AuthorityContextKey).(string)
	if !ok || authority == "" {
		return "", false
	}

	return authority, true
}
