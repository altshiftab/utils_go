package portmap

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"
)

// gateway is a scripted NAT-PMP gateway on loopback. Each request is answered by respond, whose
// nil answer means silence.
type gateway struct {
	conn     net.PacketConn
	mutex    sync.Mutex
	requests [][]byte
	respond  func(request []byte) [][]byte
}

func newGateway(t *testing.T, respond func(request []byte) [][]byte) *gateway {
	t.Helper()

	conn, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen packet: %v", err)
	}

	server := &gateway{conn: conn, respond: respond}

	t.Cleanup(func() { _ = conn.Close() })

	go func() {
		buffer := make([]byte, 64)

		for {
			n, from, err := conn.ReadFrom(buffer)
			if err != nil {
				return
			}

			request := append([]byte(nil), buffer[:n]...)

			server.mutex.Lock()
			server.requests = append(server.requests, request)
			respond := server.respond
			server.mutex.Unlock()

			for _, answer := range respond(request) {
				_, _ = conn.WriteTo(answer, from)
			}
		}
	}()

	return server
}

func (server *gateway) address() netip.AddrPort {
	return server.conn.LocalAddr().(*net.UDPAddr).AddrPort()
}

func (server *gateway) setRespond(respond func(request []byte) [][]byte) {
	server.mutex.Lock()
	defer server.mutex.Unlock()

	server.respond = respond
}

func (server *gateway) requestCount() int {
	server.mutex.Lock()
	defer server.mutex.Unlock()

	return len(server.requests)
}

func mappingAnswer(request []byte, code uint16, external uint16, lifetime uint32) []byte {
	answer := make([]byte, 16)
	answer[1] = 128 + request[1]
	binary.BigEndian.PutUint16(answer[2:4], code)
	copy(answer[8:10], request[4:6])
	binary.BigEndian.PutUint16(answer[10:12], external)
	binary.BigEndian.PutUint32(answer[12:16], lifetime)

	return answer
}

func grant(external uint16, lifetime uint32) func(request []byte) [][]byte {
	return func(request []byte) [][]byte {
		if len(request) < 2 || request[1] == opExternalAddress {
			return [][]byte{{0, 128, 0, 0, 0, 0, 0, 1, 149, 50, 216, 238}}
		}

		return [][]byte{mappingAnswer(request, 0, external, lifetime)}
	}
}

func silent(_ []byte) [][]byte { return nil }

func newTestClient(t *testing.T, server *gateway) *Client {
	t.Helper()

	conn, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen packet: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	client, err := NewClient(conn, server.address())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	client.firstWait = 20 * time.Millisecond

	return client
}

var protonRequest = &Request{Protocol: TCP, InternalPort: 0, SuggestedExternalPort: 1, Lifetime: 60 * time.Second}

func TestMap(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		respond      func(request []byte) [][]byte
		expectedPort uint16
		expectedLife time.Duration
		expectedErr  error
	}{
		{name: "granted", respond: grant(40000, 60), expectedPort: 40000, expectedLife: 60 * time.Second},
		{
			name: "an answer to another opcode first is ignored",
			respond: func(request []byte) [][]byte {
				other := append([]byte(nil), request...)
				other[1] = byte(UDP)

				return [][]byte{mappingAnswer(other, 0, 1111, 60), mappingAnswer(request, 0, 40000, 60)}
			},
			expectedPort: 40000,
			expectedLife: 60 * time.Second,
		},
		{
			name:        "refused",
			respond:     func(request []byte) [][]byte { return [][]byte{mappingAnswer(request, 2, 0, 0)} },
			expectedErr: ErrResult,
		},
		{name: "silent", respond: silent, expectedErr: ErrNoAnswer},
		{
			name:        "a short answer is no answer",
			respond:     func(request []byte) [][]byte { return [][]byte{mappingAnswer(request, 0, 40000, 60)[:12]} },
			expectedErr: ErrNoAnswer,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := newGateway(t, testCase.respond)
			client := newTestClient(t, server)

			mapping, err := client.Map(t.Context(), protonRequest)

			if testCase.expectedErr != nil {
				if !errors.Is(err, testCase.expectedErr) {
					t.Fatalf("%s: got %v, want %v", testCase.name, err, testCase.expectedErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("%s: map: %v", testCase.name, err)
			}

			if mapping.ExternalPort != testCase.expectedPort || mapping.Lifetime != testCase.expectedLife {
				t.Errorf("%s: got port %d for %s", testCase.name, mapping.ExternalPort, mapping.Lifetime)
			}
		})
	}
}

// TestMapSendsProtonsRequest checks the bytes on the wire against the request Proton documents
// (natpmpc -a 1 0 tcp 60): version 0, opcode 2, internal port 0, suggested port 1, 60 seconds.
func TestMapSendsProtonsRequest(t *testing.T) {
	t.Parallel()

	server := newGateway(t, grant(40000, 60))
	client := newTestClient(t, server)

	if _, err := client.Map(t.Context(), protonRequest); err != nil {
		t.Fatalf("map: %v", err)
	}

	server.mutex.Lock()
	request := server.requests[0]
	server.mutex.Unlock()

	expected := []byte{0, 2, 0, 0, 0, 0, 0, 1, 0, 0, 0, 60}
	if string(request) != string(expected) {
		t.Errorf("request: got % x, want % x", request, expected)
	}
}

func TestExternalAddress(t *testing.T) {
	t.Parallel()

	server := newGateway(t, grant(40000, 60))
	client := newTestClient(t, server)

	address, err := client.ExternalAddress(t.Context())
	if err != nil {
		t.Fatalf("external address: %v", err)
	}

	if address != netip.MustParseAddr("149.50.216.238") {
		t.Errorf("got %s", address)
	}
}

func TestMapRefusesBadRequests(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		request *Request
	}{
		{name: "nil", request: nil},
		{name: "an unknown protocol", request: &Request{Protocol: 9, Lifetime: time.Minute}},
		{name: "a negative lifetime", request: &Request{Protocol: TCP, Lifetime: -time.Second}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// A client of its own: one is not safe for concurrent use.
			client := newTestClient(t, newGateway(t, grant(40000, 60)))

			if _, err := client.Map(t.Context(), testCase.request); err == nil {
				t.Errorf("%s: expected an error", testCase.name)
			}
		})
	}
}

type change struct {
	mapping *Mapping
	at      time.Time
}

func runKeeper(t *testing.T, server *gateway) (<-chan change, context.CancelFunc) {
	t.Helper()

	return runKeeperAsking(t, server, false)
}

// runKeeperAsking runs a keeper against the gateway, asking for the public address when address is
// set, and returns what it reports.
func runKeeperAsking(t *testing.T, server *gateway, address bool) (<-chan change, context.CancelFunc) {
	t.Helper()

	client := newTestClient(t, server)
	// Four attempts from 100 ms take 1.5 s: longer than the lapse may be late, so a keeper that
	// let an attempt outlive its lease would fail the expiry test.
	client.firstWait = 100 * time.Millisecond
	changes := make(chan change, 16)

	ctx, cancel := context.WithCancel(t.Context())

	keeper := &Keeper{
		Client:     client,
		Request:    protonRequest,
		Changed:    func(mapping *Mapping) { changes <- change{mapping: mapping, at: time.Now()} },
		Address:    address,
		RetryDelay: 50 * time.Millisecond,
	}

	done := make(chan struct{})

	go func() {
		defer close(done)

		if err := keeper.Run(ctx); err != nil {
			t.Errorf("run: %v", err)
		}
	}()

	t.Cleanup(func() {
		cancel()
		<-done
	})

	return changes, cancel
}

func next(t *testing.T, changes <-chan change, within time.Duration) change {
	t.Helper()

	select {
	case got := <-changes:
		return got
	case <-time.After(within):
		t.Fatalf("no change within %s", within)

		return change{}
	}
}

// TestKeeperReportsALapseAtExpiry is the case the retry count used to hide: renewals stop being
// answered, and the port must stop being advertised when its lease runs out, not some retries
// later.
func TestKeeperReportsALapseAtExpiry(t *testing.T) {
	t.Parallel()

	server := newGateway(t, grant(40000, 1))
	changes, _ := runKeeper(t, server)

	first := next(t, changes, 2*time.Second)
	if first.mapping == nil || first.mapping.ExternalPort != 40000 {
		t.Fatalf("first change: got %+v, want port 40000", first.mapping)
	}

	server.setRespond(silent)

	lapse := next(t, changes, 3*time.Second)
	if lapse.mapping != nil {
		t.Fatalf("expected a lapse, got port %d", lapse.mapping.ExternalPort)
	}

	if late := lapse.at.Sub(first.mapping.Expires()); late > 300*time.Millisecond {
		t.Errorf("the lapse was reported %s after the lease expired", late)
	}

	server.setRespond(grant(40001, 1))

	again := next(t, changes, 2*time.Second)
	if again.mapping == nil || again.mapping.ExternalPort != 40001 {
		t.Fatalf("after the lapse: got %+v, want port 40001", again.mapping)
	}
}

func TestKeeperFollowsAChangedPort(t *testing.T) {
	t.Parallel()

	server := newGateway(t, grant(40000, 1))
	changes, _ := runKeeper(t, server)

	if first := next(t, changes, 2*time.Second); first.mapping == nil || first.mapping.ExternalPort != 40000 {
		t.Fatalf("first change: got %+v", first.mapping)
	}

	server.setRespond(grant(40002, 1))

	if changed := next(t, changes, 2*time.Second); changed.mapping == nil || changed.mapping.ExternalPort != 40002 {
		t.Fatalf("got %+v, want port 40002", changed.mapping)
	}
}

func TestKeeperRenewsQuietly(t *testing.T) {
	t.Parallel()

	server := newGateway(t, grant(40000, 1))
	changes, _ := runKeeper(t, server)

	next(t, changes, 2*time.Second)

	time.Sleep(1600 * time.Millisecond)

	if count := server.requestCount(); count < 3 {
		t.Errorf("%d requests in 1.6 s; a 1-second lease is renewed every half second", count)
	}

	select {
	case got := <-changes:
		t.Errorf("an unchanged renewal reported a change: %+v", got.mapping)
	default:
	}
}

func TestKeeperWithdrawsTheMappingWhenStopped(t *testing.T) {
	t.Parallel()

	server := newGateway(t, grant(40000, 60))
	changes, cancel := runKeeper(t, server)

	next(t, changes, 2*time.Second)
	cancel()

	if stopped := next(t, changes, 2*time.Second); stopped.mapping != nil {
		t.Errorf("expected a withdrawal on stop, got port %d", stopped.mapping.ExternalPort)
	}
}

// TestKeeperAsksForTheAddress gives each new port the gateway's public address, which a renewal
// keeps, and still reports the port when the gateway does not say where it is reached.
func TestKeeperAsksForTheAddress(t *testing.T) {
	t.Parallel()

	public := netip.MustParseAddr("149.50.216.238")

	mappingsOnly := func(request []byte) [][]byte {
		if len(request) < 2 || request[1] == opExternalAddress {
			return nil
		}

		return [][]byte{mappingAnswer(request, 0, 40000, 60)}
	}

	testCases := []struct {
		name     string
		respond  func(request []byte) [][]byte
		address  bool
		expected netip.Addr
	}{
		{name: "asked and answered", respond: grant(40000, 60), address: true, expected: public},
		{name: "asked and not answered", respond: mappingsOnly, address: true},
		{name: "not asked", respond: grant(40000, 60), address: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := newGateway(t, testCase.respond)
			changes, _ := runKeeperAsking(t, server, testCase.address)

			first := next(t, changes, 3*time.Second)
			if first.mapping == nil || first.mapping.ExternalPort != 40000 {
				t.Fatalf("%s: got %+v, want port 40000", testCase.name, first.mapping)
			}

			if first.mapping.ExternalAddress != testCase.expected {
				t.Errorf("%s: address %s, want %s", testCase.name, first.mapping.ExternalAddress, testCase.expected)
			}
		})
	}
}

// TestKeeperKeepsTheAddressAcrossRenewals is what Renewed sees: the address found with the port,
// without asking again.
func TestKeeperKeepsTheAddressAcrossRenewals(t *testing.T) {
	t.Parallel()

	server := newGateway(t, grant(40000, 1))
	client := newTestClient(t, server)
	renewals := make(chan *Mapping, 16)

	ctx, cancel := context.WithCancel(t.Context())

	keeper := &Keeper{
		Client:  client,
		Request: protonRequest,
		Changed: func(*Mapping) {},
		Renewed: func(mapping *Mapping) {
			select {
			case renewals <- mapping:
			default:
			}
		},
		Address:    true,
		RetryDelay: 50 * time.Millisecond,
	}

	done := make(chan struct{})

	go func() {
		defer close(done)

		_ = keeper.Run(ctx)
	}()

	t.Cleanup(func() {
		cancel()
		<-done
	})

	public := netip.MustParseAddr("149.50.216.238")

	for index := range 2 {
		select {
		case mapping := <-renewals:
			if mapping.ExternalAddress != public {
				t.Errorf("renewal %d: address %s, want %s", index, mapping.ExternalAddress, public)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("no renewal %d", index)
		}
	}

	addressRequests := 0

	server.mutex.Lock()
	for _, request := range server.requests {
		if len(request) >= 2 && request[1] == opExternalAddress {
			addressRequests++
		}
	}
	server.mutex.Unlock()

	if addressRequests != 1 {
		t.Errorf("%d address requests over two renewals of one port, want 1", addressRequests)
	}
}
