package netbind

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/net/outbound_guard"
)

const defaultTimeout = 30 * time.Second

// Binder creates sockets bound to one local address.
type Binder struct {
	address        netip.Addr
	timeout        time.Duration
	dnsServer      netip.AddrPort
	systemResolver bool
	resolver       *net.Resolver
}

// Option configures a Binder.
type Option func(*Binder)

// WithTimeout bounds every connect, and an HTTP client's whole request. It defaults to 30 seconds.
func WithTimeout(timeout time.Duration) Option {
	return func(binder *Binder) {
		binder.timeout = timeout
	}
}

// WithResolver resolves names through the DNS server at server, over sockets bound to the Binder's
// address, so that a lookup travels the same way as the connection it is for. A VPN's resolver
// inside its tunnel is the usual choice.
func WithResolver(server netip.AddrPort) Option {
	return func(binder *Binder) {
		binder.dnsServer = server
	}
}

// WithSystemResolver resolves names through the host's resolver, unbound. The names the program
// looks up are then visible to whatever that resolver reaches, though the connections stay bound;
// it is for a program that accepts that. Without it or WithResolver, a name is refused, and only
// address literals can be dialled.
func WithSystemResolver() Option {
	return func(binder *Binder) {
		binder.systemResolver = true
	}
}

// New returns a Binder for the given local address.
//
// The address must be a specific IPv4 address. The unspecified address is refused rather than
// treated as "any": it is precisely the value that would route traffic the wrong way, and a
// configuration that leaves it empty should fail at start-up rather than at the first connection.
func New(address netip.Addr, options ...Option) (*Binder, error) {
	if !address.IsValid() {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("bind address")),
		)
	}

	if address.IsUnspecified() {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: refusing to bind the unspecified address; name the address to send from",
				altshiftErrors.ErrValidationError,
			),
			address,
		)
	}

	if !address.Is4() {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: the bind address must be IPv4, got %s", altshiftErrors.ErrValidationError, address),
			address,
		)
	}

	binder := &Binder{address: address, timeout: defaultTimeout}

	for _, option := range options {
		if option != nil {
			option(binder)
		}
	}

	if binder.timeout <= 0 {
		binder.timeout = defaultTimeout
	}

	switch {
	case binder.dnsServer.IsValid() && binder.systemResolver:
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: a Binder takes a bound resolver or the system resolver, not both",
				altshiftErrors.ErrValidationError,
			),
		)
	case binder.dnsServer.IsValid():
		if !binder.dnsServer.Addr().Is4() || binder.dnsServer.Port() == 0 {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf(
					"%w: the DNS server must be an IPv4 address and port, got %s",
					altshiftErrors.ErrValidationError,
					binder.dnsServer,
				),
				binder.dnsServer,
			)
		}

		binder.resolver = &net.Resolver{PreferGo: true, StrictErrors: true, Dial: binder.dialDNS}
	case binder.systemResolver:
		binder.resolver = net.DefaultResolver
	}

	return binder, nil
}

// Address returns the local address every socket is bound to.
func (binder *Binder) Address() netip.Addr {
	if binder == nil {
		return netip.Addr{}
	}

	return binder.address
}

// DialContext connects to a TCP address from the bound local address. A host that is a name is
// resolved through the Binder's resolver, and refused when it has none; each IPv4 answer is tried
// in turn.
//
// Naming the network "tcp4" rather than "tcp" is not redundant with the bound address: it keeps a
// dual-stack name from costing a connect timeout on an AAAA answer an IPv4 source could never reach.
func (binder *Binder) DialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	if binder == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("binder"))
	}

	switch network {
	case "tcp", "tcp4":
	default:
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: unsupported network %q; only TCP over IPv4 is dialled", altshiftErrors.ErrValidationError, network),
			network,
		)
	}

	host, portString, err := net.SplitHostPort(address)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: net split host port: %w", altshiftErrors.ErrParseError, err),
			address,
		)
	}

	port, err := strconv.ParseUint(portString, 10, 16)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: port %q: %w", altshiftErrors.ErrParseError, portString, err),
			address,
		)
	}

	addresses, err := binder.Resolve(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve: %w", err)
	}

	var dialErrors []error

	for _, resolved := range addresses {
		conn, err := binder.dial(ctx, "tcp4", netip.AddrPortFrom(resolved, uint16(port)))
		if err == nil {
			return conn, nil
		}

		dialErrors = append(dialErrors, err)
	}

	return nil, altshiftErrors.New(fmt.Errorf("dial: %w", errors.Join(dialErrors...)), address)
}

// dial connects to one address over network ("tcp4" or "udp4") and verifies the local end.
func (binder *Binder) dial(ctx context.Context, network string, remote netip.AddrPort) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: binder.timeout}

	switch network {
	case "tcp4":
		dialer.LocalAddr = &net.TCPAddr{IP: binder.address.AsSlice()}
	case "udp4":
		dialer.LocalAddr = &net.UDPAddr{IP: binder.address.AsSlice()}
	default:
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: unsupported network %q", altshiftErrors.ErrValidationError, network),
			network,
		)
	}

	conn, err := dialer.DialContext(ctx, network, remote.String())
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("dialer dial context: %w", err), remote)
	}

	return verified(binder, conn.LocalAddr(), conn)
}

// dialDNS is the bound resolver's dialler. It goes to the configured DNS server whatever address
// the resolver asks for: the resolver takes that from the host's resolv.conf, which names the
// unbound system resolver this exists to avoid.
func (binder *Binder) dialDNS(ctx context.Context, network string, _ string) (net.Conn, error) {
	switch network {
	case "udp", "udp4":
		network = "udp4"
	case "tcp", "tcp4":
		network = "tcp4"
	default:
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: unsupported resolver network %q", altshiftErrors.ErrValidationError, network),
			network,
		)
	}

	return binder.dial(ctx, network, binder.dnsServer)
}

// Listen accepts TCP connections on the bound address.
//
// A secondary address can still be coming up at boot, and the bind then fails with EADDRNOTAVAIL.
// That is the correct outcome: the unit restarts and tries again. Falling back to the wildcard
// would "work" and send everything the wrong way.
func (binder *Binder) Listen(ctx context.Context, port int) (net.Listener, error) {
	if binder == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("binder"))
	}

	local, err := binder.localAddrPort(port)
	if err != nil {
		return nil, fmt.Errorf("local addr port: %w", err)
	}

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp4", local.String())
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("listen config listen: %w", err), local)
	}

	return verified(binder, listener.Addr(), listener)
}

// ListenPacket opens a UDP socket on the bound address.
func (binder *Binder) ListenPacket(ctx context.Context, port int) (net.PacketConn, error) {
	if binder == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("binder"))
	}

	local, err := binder.localAddrPort(port)
	if err != nil {
		return nil, fmt.Errorf("local addr port: %w", err)
	}

	conn, err := (&net.ListenConfig{}).ListenPacket(ctx, "udp4", local.String())
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("listen config listen packet: %w", err), local)
	}

	return verified(binder, conn.LocalAddr(), conn)
}

// verified returns socket when its local address is the bound one, and closes it otherwise.
func verified[T io.Closer](binder *Binder, local net.Addr, socket T) (T, error) {
	if err := binder.verify(local); err != nil {
		var zero T

		if closeErr := socket.Close(); closeErr != nil {
			return zero, altshiftErrors.New(fmt.Errorf("verify: %w (and close: %w)", err, closeErr))
		}

		return zero, altshiftErrors.New(fmt.Errorf("verify: %w", err))
	}

	return socket, nil
}

// localAddrPort is the bound address with a port, 0 asking the kernel to choose one.
func (binder *Binder) localAddrPort(port int) (netip.AddrPort, error) {
	if port < 0 || port > 65535 {
		return netip.AddrPort{}, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: port %d is out of range", altshiftErrors.ErrValidationError, port),
			port,
		)
	}

	return netip.AddrPortFrom(binder.address, uint16(port)), nil
}

// HTTPClient returns a client whose every connection is made by DialContext.
//
// Proxy is nil rather than the default: http.ProxyFromEnvironment would send the requests to
// whatever HTTP_PROXY named, from that proxy's source address, defeating the binding entirely.
// DialTLS is left unset so that HTTPS goes through the same bound dialler.
func (binder *Binder) HTTPClient() (*http.Client, error) {
	if binder == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("binder"))
	}

	return &http.Client{
		Timeout: binder.timeout,
		Transport: &http.Transport{
			DialContext:           binder.DialContext,
			Proxy:                 nil,
			ForceAttemptHTTP2:     false,
			MaxIdleConnsPerHost:   2,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   binder.timeout,
			ResponseHeaderTimeout: binder.timeout,
		},
	}, nil
}

// verify checks that a socket really ended up on the address it was asked to use.
func (binder *Binder) verify(address net.Addr) error {
	if address == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("local address"))
	}

	var actual netip.Addr

	switch typed := address.(type) {
	case *net.TCPAddr:
		actual, _ = netip.AddrFromSlice(typed.IP)
	case *net.UDPAddr:
		actual, _ = netip.AddrFromSlice(typed.IP)
	default:
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: cannot verify the local address of a %T", altshiftErrors.ErrVerificationError, address),
		)
	}

	actual = actual.Unmap()

	if actual != binder.address {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: the socket is bound to %s, expected %s",
				altshiftErrors.ErrVerificationError,
				actual,
				binder.address,
			),
			actual,
		)
	}

	return nil
}

// Resolve looks up the IPv4 addresses of a host: a literal as it is, a name through the Binder's
// resolver. A name is refused when the Binder has none, rather than quietly asking the system
// resolver, which would leak it outside the tunnel.
//
// Only A records are returned, since a socket bound to an IPv4 address could not reach an AAAA
// answer, and returning one would only buy a connect timeout.
func (binder *Binder) Resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if binder == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("binder"))
	}

	if host == "" {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("host")),
		)
	}

	// A literal needs no lookup, and sending one to a resolver would leak a name that was never a
	// name.
	if address, err := netip.ParseAddr(host); err == nil {
		address = address.Unmap()
		if !address.Is4() {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %s is not an IPv4 address", altshiftErrors.ErrValidationError, address),
				host,
			)
		}

		return []netip.Addr{address}, nil
	}

	if binder.resolver == nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: %q is a name, and no resolver is configured; dial an address, or give the Binder a resolver",
				altshiftErrors.ErrValidationError,
				host,
			),
			host,
		)
	}

	addresses, err := binder.resolver.LookupNetIP(ctx, "ip4", host)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("resolver lookup net ip: %w", err), host)
	}

	resolved := make([]netip.Addr, 0, len(addresses))

	for _, address := range addresses {
		if address = address.Unmap(); address.Is4() {
			resolved = append(resolved, address)
		}
	}

	if len(resolved) == 0 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: no IPv4 address for %s", altshiftErrors.ErrVerificationError, host),
			host,
		)
	}

	return resolved, nil
}

// Routable reports whether an address is one a program should dial when a peer or a stranger
// names it: an IPv4 address on the public internet. It keeps such a program from probing the local
// network, the VPN's own gateway or loopback on someone else's say-so.
func Routable(address netip.Addr) bool {
	return address.Is4() && outbound_guard.IsPublic(address)
}
