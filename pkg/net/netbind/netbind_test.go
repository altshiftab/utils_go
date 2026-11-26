package netbind

import (
	"context"
	"encoding/binary"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"testing"
	"time"
)

var loopback = netip.MustParseAddr("127.0.0.1")

func TestNewRejectsWhatWouldLeak(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		address     netip.Addr
		options     []Option
		expectError bool
	}{
		{name: "a specific address", address: netip.MustParseAddr("192.168.2.230")},
		{name: "loopback", address: loopback},
		{name: "a bound resolver", address: loopback, options: []Option{WithResolver(netip.MustParseAddrPort("10.2.0.1:53"))}},
		{name: "the system resolver", address: loopback, options: []Option{WithSystemResolver()}},
		{name: "the zero address", address: netip.Addr{}, expectError: true},
		{name: "the unspecified address", address: netip.MustParseAddr("0.0.0.0"), expectError: true},
		{name: "ipv6", address: netip.MustParseAddr("::1"), expectError: true},
		{name: "the unspecified ipv6 address", address: netip.MustParseAddr("::"), expectError: true},
		{
			name:        "both resolvers",
			address:     loopback,
			options:     []Option{WithResolver(netip.MustParseAddrPort("10.2.0.1:53")), WithSystemResolver()},
			expectError: true,
		},
		{
			name:        "an ipv6 dns server",
			address:     loopback,
			options:     []Option{WithResolver(netip.MustParseAddrPort("[2a07:b944::2:1]:53"))},
			expectError: true,
		},
		{
			name:        "a dns server without a port",
			address:     loopback,
			options:     []Option{WithResolver(netip.AddrPortFrom(netip.MustParseAddr("10.2.0.1"), 0))},
			expectError: true,
		},
		{name: "the limited broadcast address", address: netip.MustParseAddr("255.255.255.255"), expectError: true},
		{name: "a multicast address", address: netip.MustParseAddr("224.0.0.1"), expectError: true},
		{
			// A connection to 0.0.0.0 reaches the host itself on Linux, so the queries would go to
			// the local resolver.
			name:        "the unspecified dns server",
			address:     loopback,
			options:     []Option{WithResolver(netip.MustParseAddrPort("0.0.0.0:53"))},
			expectError: true,
		},
		{name: "a zero dns server", address: loopback, options: []Option{WithResolver(netip.AddrPort{})}, expectError: true},
		{
			name:        "a zero dns server beside the system resolver",
			address:     loopback,
			options:     []Option{WithResolver(netip.AddrPort{}), WithSystemResolver()},
			expectError: true,
		},
		{
			name:        "a broadcast dns server",
			address:     loopback,
			options:     []Option{WithResolver(netip.MustParseAddrPort("255.255.255.255:53"))},
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			binder, err := New(testCase.address, testCase.options...)

			if testCase.expectError {
				if err == nil {
					t.Fatalf("%s: expected an error", testCase.name)
				}

				return
			}

			if err != nil {
				t.Fatalf("%s: new: %v", testCase.name, err)
			}

			if binder.Address() != testCase.address {
				t.Errorf("%s: address: got %s, want %s", testCase.name, binder.Address(), testCase.address)
			}
		})
	}
}

// TestBoundSocketsReportTheBoundAddress exercises the actual bind and connect, since the guarantee
// this package makes is about what the kernel did, not about what was asked for.
func TestBoundSocketsReportTheBoundAddress(t *testing.T) {
	t.Parallel()

	binder, err := New(loopback, WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	listener, err := binder.Listen(t.Context(), 0)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	defer closeOrError(t, listener.Close)

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address: got %T, want *net.TCPAddr", listener.Addr())
	}

	accepted := acceptOne(listener)

	conn, err := binder.DialContext(t.Context(), "tcp", address.String())
	if err != nil {
		t.Fatalf("dial context: %v", err)
	}

	defer closeOrError(t, conn.Close)

	if local := conn.LocalAddr().(*net.TCPAddr); !local.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Errorf("dialled from %s, want 127.0.0.1", local.IP)
	}

	server, ok := <-accepted
	if !ok {
		t.Fatal("accept failed")
	}

	closeOrError(t, server.Close)
}

func TestListenPacketBinds(t *testing.T) {
	t.Parallel()

	binder, err := New(loopback)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	conn, err := binder.ListenPacket(t.Context(), 0)
	if err != nil {
		t.Fatalf("listen packet: %v", err)
	}

	defer closeOrError(t, conn.Close)

	if address := conn.LocalAddr().(*net.UDPAddr); !address.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Errorf("bound to %s, want 127.0.0.1", address.IP)
	}
}

// TestListenFailsOnAnAddressTheHostDoesNotHave is what a secondary address still coming up at boot
// should produce: a clear failure and a unit restart, never a quiet fallback to the wildcard.
func TestListenFailsOnAnAddressTheHostDoesNotHave(t *testing.T) {
	t.Parallel()

	binder, err := New(netip.MustParseAddr("192.0.2.1"), WithTimeout(time.Second))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	listener, err := binder.Listen(t.Context(), 0)
	if err == nil {
		closeOrError(t, listener.Close)
		t.Fatal("expected the bind to fail on an address the host does not hold")
	}
}

func TestListenRefusesPortsOutOfRange(t *testing.T) {
	t.Parallel()

	binder, err := New(loopback)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	for _, port := range []int{-1, 65536} {
		if listener, err := binder.Listen(t.Context(), port); err == nil {
			closeOrError(t, listener.Close)
			t.Errorf("port %d: expected an error", port)
		}

		if conn, err := binder.ListenPacket(t.Context(), port); err == nil {
			closeOrError(t, conn.Close)
			t.Errorf("port %d: expected an error", port)
		}
	}
}

func TestDialContextRefuses(t *testing.T) {
	t.Parallel()

	withoutResolver, err := New(loopback, WithTimeout(time.Second))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	testCases := []struct {
		name    string
		network string
		address string
	}{
		{name: "udp", network: "udp", address: "127.0.0.1:9"},
		{name: "unix", network: "unix", address: "127.0.0.1:9"},
		{name: "tcp6", network: "tcp6", address: "127.0.0.1:9"},
		{name: "an empty network", network: "", address: "127.0.0.1:9"},
		{name: "a name without a resolver", network: "tcp", address: "example.com:443"},
		{name: "an ipv6 literal", network: "tcp", address: "[::1]:9"},
		{name: "a named port", network: "tcp", address: "127.0.0.1:http"},
		{name: "no port", network: "tcp", address: "127.0.0.1"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			conn, err := withoutResolver.DialContext(t.Context(), testCase.network, testCase.address)
			if err == nil {
				closeOrError(t, conn.Close)
				t.Fatalf("%s: expected an error", testCase.name)
			}
		})
	}
}

// TestBoundResolverAsksItsOwnServer is the guarantee WithResolver exists for: a name goes to the
// configured server, from the bound address, and a dial by name lands where it answered.
func TestBoundResolverAsksItsOwnServer(t *testing.T) {
	t.Parallel()

	target, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	defer closeOrError(t, target.Close)

	accepted := acceptOne(target)

	server, queries := fakeDNS(t, netip.MustParseAddr("127.0.0.1"))

	binder, err := New(loopback, WithResolver(server), WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	addresses, err := binder.Resolve(t.Context(), "peer.netbind.test")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if len(addresses) != 1 || addresses[0] != loopback {
		t.Fatalf("resolve: got %v, want [127.0.0.1]", addresses)
	}

	port := target.Addr().(*net.TCPAddr).Port

	conn, err := binder.DialContext(t.Context(), "tcp", net.JoinHostPort("peer.netbind.test", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("dial context by name: %v", err)
	}

	closeOrError(t, conn.Close)

	if server, ok := <-accepted; ok {
		closeOrError(t, server.Close)
	} else {
		t.Error("the dial by name did not reach the address the server gave")
	}

	select {
	case query := <-queries:
		if query.name != "peer.netbind.test" {
			t.Errorf("the server was asked for %q", query.name)
		}

		if query.from.Addr() != loopback {
			t.Errorf("the query came from %s, want the bound address", query.from)
		}
	default:
		t.Error("the configured server was never asked")
	}
}

func TestResolveLiterals(t *testing.T) {
	t.Parallel()

	binder, err := New(loopback)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	testCases := []struct {
		name        string
		host        string
		expected    netip.Addr
		expectError bool
	}{
		{name: "an ipv4 literal", host: "193.180.81.1", expected: netip.MustParseAddr("193.180.81.1")},
		{name: "an ipv4-mapped literal", host: "::ffff:193.180.81.1", expected: netip.MustParseAddr("193.180.81.1")},
		{name: "an ipv6 literal", host: "2001:db8::1", expectError: true},
		{name: "empty", host: "", expectError: true},
		{name: "a name, without a resolver", host: "example.com", expectError: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			addresses, err := binder.Resolve(t.Context(), testCase.host)

			if testCase.expectError {
				if err == nil {
					t.Fatalf("%s: expected an error, got %v", testCase.name, addresses)
				}

				return
			}

			if err != nil {
				t.Fatalf("%s: resolve: %v", testCase.name, err)
			}

			if len(addresses) != 1 || addresses[0] != testCase.expected {
				t.Errorf("%s: got %v, want [%s]", testCase.name, addresses, testCase.expected)
			}
		})
	}
}

// TestHTTPClientIgnoresTheProxyEnvironment guards the one way a correctly bound dialler still sends
// requests from somewhere else.
func TestHTTPClientIgnoresTheProxyEnvironment(t *testing.T) {
	t.Parallel()

	binder, err := New(loopback, WithTimeout(time.Second))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	client, err := binder.HTTPClient()
	if err != nil {
		t.Fatalf("http client: %v", err)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport: got %T, want *http.Transport", client.Transport)
	}

	if transport.Proxy != nil {
		t.Error("the transport has a proxy function; requests could leave from the proxy's address")
	}

	if transport.DialContext == nil {
		t.Error("the transport does not use the bound dialler")
	}

	if client.Timeout == 0 {
		t.Error("the client has no timeout")
	}
}

func TestRoutable(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		address  netip.Addr
		expected bool
	}{
		{name: "a public address", address: netip.MustParseAddr("193.180.81.1"), expected: true},
		{name: "another public address", address: netip.MustParseAddr("8.8.8.8"), expected: true},
		{name: "just past carrier grade nat", address: netip.MustParseAddr("100.128.0.1"), expected: true},
		{name: "just before carrier grade nat", address: netip.MustParseAddr("100.63.255.255"), expected: true},
		{name: "private ten, a vpn gateway", address: netip.MustParseAddr("10.2.0.1")},
		{name: "private one seven two", address: netip.MustParseAddr("172.16.0.1")},
		{name: "private one nine two", address: netip.MustParseAddr("192.168.1.1")},
		{name: "loopback", address: loopback},
		{name: "unspecified", address: netip.MustParseAddr("0.0.0.0")},
		{name: "this network", address: netip.MustParseAddr("0.1.2.3")},
		{name: "link local", address: netip.MustParseAddr("169.254.169.254")},
		{name: "carrier grade nat low", address: netip.MustParseAddr("100.64.0.1")},
		{name: "carrier grade nat high", address: netip.MustParseAddr("100.127.255.254")},
		{name: "documentation", address: netip.MustParseAddr("192.0.2.1")},
		{name: "multicast", address: netip.MustParseAddr("224.0.0.1")},
		{name: "reserved", address: netip.MustParseAddr("240.0.0.1")},
		{name: "broadcast", address: netip.MustParseAddr("255.255.255.255")},
		{name: "ipv6", address: netip.MustParseAddr("2606:4700::1111")},
		{name: "ipv4 mapped ipv6", address: netip.MustParseAddr("::ffff:8.8.8.8")},
		{name: "the zero address", address: netip.Addr{}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := Routable(testCase.address); got != testCase.expected {
				t.Errorf("%s: got %t, want %t", testCase.name, got, testCase.expected)
			}
		})
	}
}

func TestNilBinderIsInert(t *testing.T) {
	t.Parallel()

	var binder *Binder

	if binder.Address().IsValid() {
		t.Error("a nil binder should have no address")
	}

	if _, err := binder.DialContext(context.Background(), "tcp", "127.0.0.1:9"); err == nil {
		t.Error("dial context: expected an error from a nil binder")
	}

	if _, err := binder.Listen(context.Background(), 0); err == nil {
		t.Error("listen: expected an error from a nil binder")
	}

	if _, err := binder.ListenPacket(context.Background(), 0); err == nil {
		t.Error("listen packet: expected an error from a nil binder")
	}

	if _, err := binder.HTTPClient(); err == nil {
		t.Error("http client: expected an error from a nil binder")
	}

	if _, err := binder.Resolve(context.Background(), "127.0.0.1"); err == nil {
		t.Error("resolve: expected an error from a nil binder")
	}
}

// TestAZeroBinderOpensNothing refuses a Binder not made by New before any socket exists: its zero
// address would bind the wildcard and send from the default source.
func TestAZeroBinderOpensNothing(t *testing.T) {
	t.Parallel()

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	defer closeOrError(t, listener.Close)

	accepted := acceptOne(listener)
	binder := new(Binder)

	if _, err := binder.DialContext(t.Context(), "tcp4", listener.Addr().String()); err == nil {
		t.Error("dial context: a zero binder dialled")
	}

	// With a timeout, so that only the missing address stands in the way.
	if _, err := (&Binder{timeout: time.Second}).DialContext(t.Context(), "tcp4", listener.Addr().String()); err == nil {
		t.Error("dial context: a binder without an address dialled")
	}

	if _, err := binder.Listen(t.Context(), 0); err == nil {
		t.Error("listen: a zero binder listened")
	}

	if _, err := binder.ListenPacket(t.Context(), 0); err == nil {
		t.Error("listen packet: a zero binder listened")
	}

	select {
	case conn := <-accepted:
		if conn != nil {
			_ = conn.Close()

			t.Error("a zero binder's dial reached the listener")
		}
	case <-time.After(200 * time.Millisecond):
	}
}

// TestDialShare splits what is left of the timeout among the addresses still to try, with a floor,
// so that one address that drops packets silently cannot spend all of it.
func TestDialShare(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		remaining time.Duration
		left      int
		want      time.Duration
	}{
		{name: "one address takes it all", remaining: 30 * time.Second, left: 1, want: 30 * time.Second},
		{name: "three addresses share it", remaining: 30 * time.Second, left: 3, want: 10 * time.Second},
		{name: "a share below the floor gets the floor", remaining: 5 * time.Second, left: 10, want: 2 * time.Second},
		{name: "less than the floor left", remaining: time.Second, left: 4, want: time.Second},
		{name: "nothing left", remaining: 0, left: 2, want: 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := dialShare(testCase.remaining, testCase.left); got != testCase.want {
				t.Errorf("%s: %s, want %s", testCase.name, got, testCase.want)
			}
		})
	}
}

func closeOrError(t *testing.T, closeFunc func() error) {
	t.Helper()

	if err := closeFunc(); err != nil {
		t.Errorf("close: %v", err)
	}
}

func acceptOne(listener net.Listener) <-chan net.Conn {
	accepted := make(chan net.Conn, 1)

	go func() {
		defer close(accepted)

		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	return accepted
}

type dnsQuery struct {
	name string
	from netip.AddrPort
}

// fakeDNS answers every A query on a loopback UDP socket with answer, and reports each query.
func fakeDNS(t *testing.T, answer netip.Addr) (netip.AddrPort, <-chan dnsQuery) {
	t.Helper()

	conn, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen packet: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	queries := make(chan dnsQuery, 16)

	go func() {
		buffer := make([]byte, 1500)

		for {
			n, from, err := conn.ReadFrom(buffer)
			if err != nil {
				return
			}

			response, name, ok := answerA(buffer[:n], answer)
			if !ok {
				continue
			}

			select {
			case queries <- dnsQuery{name: name, from: from.(*net.UDPAddr).AddrPort()}:
			default:
			}

			_, _ = conn.WriteTo(response, from)
		}
	}()

	return conn.LocalAddr().(*net.UDPAddr).AddrPort(), queries
}

// answerA builds the response to a one-question query, with one A record for an A question and
// none for any other.
func answerA(query []byte, answer netip.Addr) ([]byte, string, bool) {
	if len(query) < 12 || binary.BigEndian.Uint16(query[4:6]) != 1 {
		return nil, "", false
	}

	offset := 12

	var name []byte

	for offset < len(query) && query[offset] != 0 {
		length := int(query[offset])
		if offset+1+length > len(query) {
			return nil, "", false
		}

		if len(name) > 0 {
			name = append(name, '.')
		}

		name = append(name, query[offset+1:offset+1+length]...)
		offset += 1 + length
	}

	if offset+5 > len(query) {
		return nil, "", false
	}

	questionEnd := offset + 5
	questionType := binary.BigEndian.Uint16(query[offset+1 : offset+3])

	response := make([]byte, 0, questionEnd+16)
	response = append(response, query[0:2]...)
	response = append(response, 0x81, 0x80, 0, 1)

	if questionType == 1 {
		response = append(response, 0, 1)
	} else {
		response = append(response, 0, 0)
	}

	response = append(response, 0, 0, 0, 0)
	response = append(response, query[12:questionEnd]...)

	if questionType == 1 {
		address := answer.As4()
		response = append(response, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4)
		response = append(response, address[:]...)
	}

	return response, string(name), true
}
