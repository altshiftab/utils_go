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
		{name: "a negative lifetime under a second", request: &Request{Protocol: TCP, Lifetime: -500 * time.Millisecond}},
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

// addressOf answers mapping requests with the port, for a second, and address requests with the
// address the gateway is set to, or with nothing while it is the zero address.
type addressOf struct {
	mutex   sync.Mutex
	address netip.Addr
}

func (state *addressOf) set(address netip.Addr) {
	state.mutex.Lock()
	defer state.mutex.Unlock()

	state.address = address
}

func (state *addressOf) respond(port uint16) func(request []byte) [][]byte {
	return func(request []byte) [][]byte {
		if len(request) < 2 {
			return nil
		}

		if request[1] != opExternalAddress {
			return [][]byte{mappingAnswer(request, 0, port, 1)}
		}

		state.mutex.Lock()
		address := state.address
		state.mutex.Unlock()

		if !address.IsValid() {
			return nil
		}

		bytes := address.As4()

		return [][]byte{{0, 128, 0, 0, 0, 0, 0, 1, bytes[0], bytes[1], bytes[2], bytes[3]}}
	}
}

// TestKeeperFollowsTheAddress asks the address at every renewal: one the gateway stops giving is
// kept, and a new one, with the same port, as when a tunnel reconnects to another exit, is a
// change.
func TestKeeperFollowsTheAddress(t *testing.T) {
	t.Parallel()

	first := netip.MustParseAddr("149.50.216.238")
	second := netip.MustParseAddr("149.50.216.239")

	state := &addressOf{address: first}
	server := newGateway(t, state.respond(40000))

	client := newTestClient(t, server)
	changes := make(chan change, 16)
	renewals := make(chan *Mapping, 64)

	ctx, cancel := context.WithCancel(t.Context())

	keeper := &Keeper{
		Client:  client,
		Request: protonRequest,
		Changed: func(mapping *Mapping) { changes <- change{mapping: mapping, at: time.Now()} },
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

	if got := next(t, changes, 3*time.Second); got.mapping == nil || got.mapping.ExternalAddress != first {
		t.Fatalf("first change: %+v, want %s", got.mapping, first)
	}

	// Unanswered, the address stays what it was, and nothing is reported.
	state.set(netip.Addr{})

	for index := range 3 {
		select {
		case mapping := <-renewals:
			if index > 0 && mapping.ExternalAddress != first {
				t.Fatalf("renewal %d with the address unanswered: %s, want %s", index, mapping.ExternalAddress, first)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("no renewal %d", index)
		}
	}

	select {
	case got := <-changes:
		t.Fatalf("an unanswered address question reported a change: %+v", got.mapping)
	default:
	}

	state.set(second)

	if got := next(t, changes, 3*time.Second); got.mapping == nil || got.mapping.ExternalPort != 40000 || got.mapping.ExternalAddress != second {
		t.Fatalf("after the exit changed: %+v, want port 40000 at %s", got.mapping, second)
	}
}

// TestKeeperRenewsTheMappingItIsGiven starts from a mapping the caller holds: it is renewed at half
// its lifetime, not at once, nothing is reported while it holds, and its lapse is reported when
// the gateway falls silent, as for a mapping the keeper got itself.
func TestKeeperRenewsTheMappingItIsGiven(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		respond func(request []byte) [][]byte
		lapses  bool
	}{
		{name: "renewed", respond: grant(40000, 1)},
		{name: "the gateway silent from the start", respond: silent, lapses: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := newGateway(t, testCase.respond)
			client := newTestClient(t, server)
			client.firstWait = 100 * time.Millisecond
			changes := make(chan change, 16)

			held := &Mapping{Protocol: TCP, ExternalPort: 40000, Lifetime: time.Second, Obtained: time.Now()}

			ctx, cancel := context.WithCancel(t.Context())

			keeper := &Keeper{
				Client:     client,
				Request:    protonRequest,
				Current:    held,
				Changed:    func(mapping *Mapping) { changes <- change{mapping: mapping, at: time.Now()} },
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

			time.Sleep(300 * time.Millisecond)

			if count := server.requestCount(); count != 0 {
				t.Fatalf("%s: %d requests before half the lease had passed", testCase.name, count)
			}

			if !testCase.lapses {
				time.Sleep(1200 * time.Millisecond)

				select {
				case got := <-changes:
					t.Fatalf("%s: a renewal of the held port reported %+v", testCase.name, got.mapping)
				default:
				}

				if count := server.requestCount(); count < 2 {
					t.Errorf("%s: %d requests in 1.5 s of a 1-second lease", testCase.name, count)
				}

				return
			}

			lapse := next(t, changes, 3*time.Second)
			if lapse.mapping != nil {
				t.Fatalf("%s: expected a lapse, got %+v", testCase.name, lapse.mapping)
			}

			if late := lapse.at.Sub(held.Expires()); late > 300*time.Millisecond || late < 0 {
				t.Errorf("%s: the lapse came %s after the held lease expired", testCase.name, late)
			}
		})
	}
}

// TestMapDiscardsQueuedAnswers is the second answer to a retransmit, left on the socket: the next
// request must not take it for its own, though nothing answers that one.
func TestMapDiscardsQueuedAnswers(t *testing.T) {
	t.Parallel()

	twice := func(request []byte) [][]byte {
		answer := mappingAnswer(request, 0, 40000, 60)

		return [][]byte{answer, answer}
	}

	server := newGateway(t, twice)
	client := newTestClient(t, server)

	if _, err := client.Map(t.Context(), protonRequest); err != nil {
		t.Fatalf("map: %v", err)
	}

	// Both answers have arrived by now; the second is still queued.
	time.Sleep(50 * time.Millisecond)

	server.setRespond(silent)

	if mapping, err := client.Map(t.Context(), protonRequest); !errors.Is(err, ErrNoAnswer) {
		t.Fatalf("a silent gateway's renewal gave %+v, %v; want no answer", mapping, err)
	}
}

// TestMapMatchesTheInternalPort takes only the answer for the port asked for by number, and any
// answer for a request for any port, as Proton is asked.
func TestMapMatchesTheInternalPort(t *testing.T) {
	t.Parallel()

	answerFor := func(request []byte, internal uint16, external uint16) []byte {
		answer := mappingAnswer(request, 0, external, 60)
		binary.BigEndian.PutUint16(answer[8:10], internal)

		return answer
	}

	testCases := []struct {
		name     string
		request  *Request
		expected uint16
	}{
		{name: "a port asked for by number", request: &Request{Protocol: TCP, InternalPort: 5000, Lifetime: time.Minute}, expected: 45000},
		{name: "any port", request: protonRequest, expected: 46000},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := newGateway(t, func(request []byte) [][]byte {
				return [][]byte{answerFor(request, 6000, 46000), answerFor(request, 5000, 45000)}
			})
			client := newTestClient(t, server)

			mapping, err := client.Map(t.Context(), testCase.request)
			if err != nil {
				t.Fatalf("%s: map: %v", testCase.name, err)
			}

			if mapping.ExternalPort != testCase.expected {
				t.Errorf("%s: got port %d, want %d", testCase.name, mapping.ExternalPort, testCase.expected)
			}
		})
	}
}

// TestMapLifetimes sends a lifetime under a second as one, never as zero, which deletes, and takes
// a granted lifetime of zero for no mapping.
func TestMapLifetimes(t *testing.T) {
	t.Parallel()

	t.Run("under a second", func(t *testing.T) {
		t.Parallel()

		server := newGateway(t, grant(40000, 60))
		client := newTestClient(t, server)

		if _, err := client.Map(t.Context(), &Request{Protocol: TCP, SuggestedExternalPort: 1, Lifetime: 500 * time.Millisecond}); err != nil {
			t.Fatalf("map: %v", err)
		}

		server.mutex.Lock()
		request := server.requests[0]
		server.mutex.Unlock()

		if lifetime := binary.BigEndian.Uint32(request[8:12]); lifetime != 1 {
			t.Errorf("half a second was asked for as %d seconds", lifetime)
		}
	})

	grants := []struct {
		name     string
		port     uint16
		lifetime uint32
	}{
		{name: "granted no lifetime", port: 40000, lifetime: 0},
		{name: "granted port 0", port: 0, lifetime: 60},
	}

	for _, granted := range grants {
		t.Run(granted.name, func(t *testing.T) {
			t.Parallel()

			server := newGateway(t, grant(granted.port, granted.lifetime))
			client := newTestClient(t, server)

			if mapping, err := client.Map(t.Context(), protonRequest); !errors.Is(err, ErrResult) {
				t.Errorf("%s: gave %+v, %v; want a refusal", granted.name, mapping, err)
			}
		})
	}
}

// TestKeeperRefusesARequestForNoLifetime is a keeper that would renew a deletion as fast as it can.
func TestKeeperRefusesARequestForNoLifetime(t *testing.T) {
	t.Parallel()

	server := newGateway(t, grant(40000, 60))

	keeper := &Keeper{
		Client:  newTestClient(t, server),
		Request: &Request{Protocol: TCP, SuggestedExternalPort: 1},
		Changed: func(*Mapping) {},
	}

	if err := keeper.Run(t.Context()); err == nil {
		t.Error("a keeper of a zero lifetime ran")
	}
}
