// Package forwarded_headers reads what the proxies in front of a service say
// about the request the client actually made: RFC 7239's `Forwarded`, the
// older `X-Forwarded-*` headers it replaced, and the `X-Client-*` headers a
// load balancer sets.
//
// It is one package rather than a helper in each caller because which header
// answers which question is decided once, and two copies of the decision are
// two chances to answer the same request differently. The scheme is read from
// `Forwarded` first, then from X-Forwarded-Proto; the host from
// X-Forwarded-Host first, then from `Forwarded`, or from X-Forwarded-Host
// alone where trusted proxies gate it, for the reasons Authority and
// ResolveAuthority give; the client's address is not read from `Forwarded` at
// all, for the reason ClientAddress gives.
package forwarded_headers

import (
	"context"
	"iter"
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
//
// With trustForwarded, X-Forwarded-Host answers first, then the host of
// `Forwarded`, then the request's own Host. X-Forwarded-Host is the one the
// proxies in front write: Firebase Hosting overwrites it and passes a client's
// `Forwarded: host=` through unchanged -- as measured, a client's
// `Forwarded: host=evil.example` sent through Firebase made a service that read
// `Forwarded` first route on evil.example -- and reverse_proxy writes both
// alike. `Forwarded` answers only where nothing in front set X-Forwarded-Host,
// for a proxy that writes `Forwarded` alone.
func Authority(request *http.Request, trustForwarded bool) string {
	if request == nil {
		return ""
	}

	if !trustForwarded {
		return strings.TrimSpace(request.Host)
	}

	return forwardedAuthority(request, true)
}

// forwardedAuthority returns the authority X-Forwarded-Host names, else, with
// readForwarded, the one the host of `Forwarded` names, else the request's own.
func forwardedAuthority(request *http.Request, readForwarded bool) string {
	if host := firstListItem(request.Header, "X-Forwarded-Host"); host != "" {
		return host
	}

	if readForwarded {
		for _, element := range elements(request.Header) {
			if element == nil {
				continue
			}

			if host := strings.TrimSpace(element.Host); host != "" {
				return host
			}
		}
	}

	return strings.TrimSpace(request.Host)
}

// ResolveAuthority reports the authority a request is for as a vhost mux with
// these settings decides it, and whether the forwarded host was believed of the
// request (ForwardedHostTrusted), the request's own Host answering otherwise.
//
// Where it is believed and trustedProxies gate it, the authority is read from
// X-Forwarded-Host alone, never from `Forwarded`: the request came through a
// trusted proxy, and Firebase Hosting, the proxy the gate is for, overwrites
// X-Forwarded-Host and passes `Forwarded` on as the client wrote it. Where it is
// believed with no trusted proxies, it is read as Authority reads it.
func ResolveAuthority(
	request *http.Request,
	trustForwardedHost bool,
	trustForwardedFor bool,
	trustedProxies []netip.Prefix,
) (string, bool) {
	if request == nil {
		return "", false
	}

	if !ForwardedHostTrusted(request.Header, trustForwardedHost, trustForwardedFor, trustedProxies) {
		return strings.TrimSpace(request.Host), false
	}

	return forwardedAuthority(request, len(trustedProxies) == 0), true
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
// way to learn the name the client used, unless ResolveAuthority decides it for
// each request from the trusted proxies it came through.
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

// maxTrustedProxyEntries bounds how many entries of trusted proxies the walk
// along X-Forwarded-For steps over before it gives up. A request passes through
// a proxy or two; a chain longer than any deployment has is not one to read on
// into.
const maxTrustedProxyEntries = 16

// forwardedForFromRight yields the entries of X-Forwarded-For from the
// rightmost leftwards, each trimmed, across every line of the header as though
// the lines were joined with commas: a proxy may add a line of its own rather
// than append to one. An empty entry is yielded as one. The header is not split
// up front, its length being the client's to choose.
func forwardedForFromRight(header http.Header) iter.Seq[string] {
	return func(yield func(string) bool) {
		lines := header.Values("X-Forwarded-For")
		for i := len(lines) - 1; i >= 0; i-- {
			line := lines[i]
			for {
				index := strings.LastIndexByte(line, ',')
				if !yield(strings.TrimSpace(line[index+1:])) {
					return
				}
				if index < 0 {
					break
				}
				line = line[:index]
			}
		}
	}
}

// inPrefixes says whether an address is inside one of the prefixes.
func inPrefixes(address netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}

	return false
}

// forwardedForClient reads the entry of X-Forwarded-For hops from the right,
// the rightmost being 1, then walks leftwards from it past the entries that are
// addresses of trusted proxies, and returns the first entry that is not one.
// The entries to the right of it are the fixed proxies' in front, and are not
// read. A header with fewer entries than hops answers nothing, as does an entry
// reached that is empty or not an address: it is what a proxy in front said
// connected to it, and skipping it would let the entry before it, which the
// client may have written, answer instead. A hops below 1 answers nothing.
func forwardedForClient(header http.Header, hops int, trustedProxies []netip.Prefix) (netip.Addr, bool) {
	if hops < 1 {
		return netip.Addr{}, false
	}

	position := 0
	trustedEntries := 0
	for entry := range forwardedForFromRight(header) {
		if position++; position < hops {
			continue
		}

		address, ok := parseClientAddress(entry)
		if !ok {
			return netip.Addr{}, false
		}

		if !inPrefixes(address, trustedProxies) {
			return address, true
		}

		if trustedEntries++; trustedEntries > maxTrustedProxyEntries {
			return netip.Addr{}, false
		}
	}

	return netip.Addr{}, false
}

// ClientHeader returns the value of one of the headers a load balancer in
// front sets of the client -- X-Client-Ip-Address, X-Client-Port,
// X-Server-Port, X-Client-Geo-Country-Iso-Code, X-Client-Geo-Region-Iso-Code,
// X-Client-Geo-City-Name, X-Tls-Ja3-Fingerprint and X-Tls-Ja4-Fingerprint --
// where it is given on exactly one line, and the empty
// string otherwise. A load balancer that sets one of them replacing what the
// client sent leaves one line of it; a second line is one it did not write, and
// which of the two it wrote cannot be told from the request.
//
// It says nothing of whether the header is to be believed. Read it only where
// the HTTP context says so (HttpContext.TrustClientHeaders), which is where
// ClientAddress named the client from X-Client-Ip-Address.
func ClientHeader(header http.Header, name string) string {
	if lines := header.Values(name); len(lines) == 1 {
		return lines[0]
	}

	return ""
}

// clientIpAddress returns X-Client-Ip-Address as the load balancer sent it,
// trimmed and otherwise as it is, where it is on one line and is an IP address
// with no zone, and the empty string otherwise. It is not normalized, so that
// what is recorded of a client is what the load balancer said.
func clientIpAddress(header http.Header) string {
	value := strings.TrimSpace(ClientHeader(header, "X-Client-Ip-Address"))
	if address, err := netip.ParseAddr(value); err != nil || address.Zone() != "" {
		return ""
	}

	return value
}

// ClientAddress returns the address of the client that made the request, as
// the proxies in front report it and the caller says they are to be believed,
// and whether it was X-Client-Ip-Address that named it. The address is the
// empty string where nothing believed names one, and the client is then the
// connection's own peer, which the caller reads from there.
//
// With trustClientHeaders, X-Client-Ip-Address answers first, as it was sent
// (trimmed), where it is on exactly one line and is an IP address. Set it only
// where a load balancer in front sets that header on every request, replacing
// what the client sent, AND nothing reaches the service except through it --
// the intil load balancer, which sets X-Client-Ip-Address, X-Client-Port,
// X-Server-Port, the client's country, region and city and the TLS
// fingerprints with replace, and is the only way in. Only where it answers are the other client
// headers believed (see ClientHeader); where it does not, they are not, even
// though trusted.
//
// With trustForwardedFor, X-Forwarded-For answers next, read from the right.
// Set it only where the proxy directly in front of the service appends the
// address it accepted the connection from as the last entry, or replaces the
// header with that address alone: Google's front end on Cloud Run appends, and
// reverse_proxy replaces. The rightmost entry is then the only one nobody but
// that proxy can write.
//
// The client is the entry forwardedForHops from the right, the rightmost being
// 1: forwardedForHops is how many entries the proxies in front always write,
// each naming who connected to it. As measured on Cloud Run, a request arrives
//
//   - sent to the run.app address, as `<whatever the caller wrote…>,<caller>`:
//     Google's front end merges the lines of the header into one and appends the
//     caller's address, so that every entry before it is the caller's to write;
//   - through Firebase Hosting, as `<client>,<Firebase edge>`: Firebase replaces
//     whatever the client sent with the address that connected to it, and the
//     front end appends the edge's.
//
// So forwardedForHops is 1 behind Google's front end alone and behind
// reverse_proxy, and 2 behind Firebase Hosting. With 2, a request sent to the
// run.app address directly with `X-Forwarded-For: 192.0.2.21` arrives as
// `192.0.2.21,<caller>` and is taken for 192.0.2.21's: nothing in the header
// tells it from one Firebase forwarded, and whoever reaches the run.app
// address names any client it likes. trustedProxies is what tells them apart.
//
// Where the entry forwardedForHops from the right is an address inside
// trustedProxies -- a proxy further out, whose own entry is trusted to name who
// connected to it -- the client is the entry before it, and so on leftwards.
// With forwardedForHops 1 and the edges' range among trustedProxies, a request
// through Firebase Hosting is its first entry's, and one sent to the run.app
// address directly is the caller's, whatever it wrote: `<anything>,<edge>,<caller>`
// is the caller's.
//
// An entry the walk reaches that is empty or not an address -- `unknown`, an
// obfuscated identifier, a zone, anything else -- answers nothing rather than
// letting another entry answer, the entry forwardedForHops from the right
// included. So does a header with fewer entries than forwardedForHops, one
// whose every entry from there is a trusted proxy's, one with more than 16 of
// them there, and a forwardedForHops below 1. An entry is read as a proxy
// writes one: bare, with a port, or bracketed, an IPv4-mapped address being the
// IPv4 address it maps; the address returned is in its normal form.
//
// A range among trustedProxies is believed of everything sent from it. A range
// shared with anything that sends requests with headers its user chose is not
// a proxy's to trust.
//
// `Forwarded` is not read for the address at all, whatever is trusted: neither
// proxy above writes it, and both pass on what the client wrote there.
func ClientAddress(
	header http.Header,
	trustForwardedFor bool,
	trustClientHeaders bool,
	forwardedForHops int,
	trustedProxies []netip.Prefix,
) (string, bool) {
	if trustClientHeaders {
		if address := clientIpAddress(header); address != "" {
			return address, true
		}
	}

	if trustForwardedFor {
		if address, ok := forwardedForClient(header, forwardedForHops, trustedProxies); ok {
			return address.String(), false
		}
	}

	return "", false
}

// ForwardedHostTrusted says whether the host the forwarded headers name is to
// be believed for a request; ResolveAuthority reads the host as it decides.
//
// Without trustForwardedHost it is not. With it and no trustedProxies it is,
// on every request. With both, it is only where trustForwardedFor holds as well
// and the rightmost X-Forwarded-For entry is an address inside trustedProxies:
// where the proxy directly in front recorded that the request came from a
// trusted proxy further out. Without trustForwardedFor nothing says that proxy
// wrote the entry, which is then the client's to write, and the forwarded host
// is believed of no request. The rightmost entry is read whatever the hops
// ClientAddress is given, being the only one nobody but that proxy can write.
//
// A request sent to a Cloud Run service's run.app address directly, with an
// X-Forwarded-Host of its own, is recorded with the caller's address there, and
// is answered for its own Host -- with a 421 by a vhost mux that does not
// answer for the run.app name -- rather than for the host it named. Through
// Firebase Hosting the rightmost entry is the edge's, and the X-Forwarded-Host
// Firebase wrote is believed.
func ForwardedHostTrusted(
	header http.Header,
	trustForwardedHost bool,
	trustForwardedFor bool,
	trustedProxies []netip.Prefix,
) bool {
	if !trustForwardedHost {
		return false
	}

	if len(trustedProxies) == 0 {
		return true
	}

	if !trustForwardedFor {
		return false
	}

	lines := header.Values("X-Forwarded-For")
	if len(lines) == 0 {
		return false
	}

	line := lines[len(lines)-1]
	address, ok := parseClientAddress(line[strings.LastIndexByte(line, ',')+1:])

	return ok && inPrefixes(address, trustedProxies)
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
