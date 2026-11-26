// Package portmap asks a NAT-PMP gateway (RFC 6886) for a forwarded port, and keeps it.
//
// Proton VPN forwards one port per tunnel this way, on its P2P servers: the gateway at 10.2.0.1
// assigns a public port on request, forwards it to the same port number on the client, and drops
// the mapping when it is not renewed within its lifetime. The port is random and changes when the
// tunnel reconnects, so a program that advertises it must follow it, and stop advertising it the
// moment the lease lapses.
package portmap

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

// Protocol is the transport a mapping forwards.
type Protocol uint8

const (
	UDP Protocol = 1
	TCP Protocol = 2
)

func (protocol Protocol) String() string {
	switch protocol {
	case UDP:
		return "UDP"
	case TCP:
		return "TCP"
	default:
		return fmt.Sprintf("protocol %d", uint8(protocol))
	}
}

const (
	version             = 0
	opExternalAddress   = 0
	responseBit         = 128
	requestSize         = 12
	mappingResponseSize = 16
	addressResponseSize = 12
)

// ErrResult is a gateway's refusal, with the result code it gave.
var ErrResult = errors.New("the gateway refused")

var resultNames = map[uint16]string{
	1: "unsupported version",
	2: "not authorized or refused",
	3: "network failure",
	4: "out of resources",
	5: "unsupported opcode",
}

// ErrNoAnswer is a gateway that did not answer any attempt.
var ErrNoAnswer = errors.New("the gateway did not answer")

// Request is a mapping to ask for. Proton documents internal port 0 with a suggested external
// port of 1, for "any port", and a lifetime of 60 seconds.
type Request struct {
	Protocol              Protocol
	InternalPort          uint16
	SuggestedExternalPort uint16
	Lifetime              time.Duration
}

// Mapping is what the gateway granted.
type Mapping struct {
	Protocol     Protocol
	InternalPort uint16
	ExternalPort uint16
	Lifetime     time.Duration
	Obtained     time.Time

	// ExternalAddress is the gateway's public address, where peers reach the mapping: filled in
	// by a Keeper asked for it, and the zero address otherwise or when the gateway did not say.
	ExternalAddress netip.Addr
}

// Expires is when the mapping lapses unless it is renewed.
func (mapping *Mapping) Expires() time.Time {
	return mapping.Obtained.Add(mapping.Lifetime)
}

// Client talks to one gateway over a socket the caller bound. It is not safe for concurrent use:
// answers are matched to requests by their opcode alone, as RFC 6886 has no request id.
type Client struct {
	conn    net.PacketConn
	gateway netip.AddrPort

	// firstWait is how long the first attempt waits for an answer; each later one waits twice as
	// long as the one before, as RFC 6886 asks, for attempts tries in all.
	firstWait time.Duration
	attempts  int
	now       func() time.Time
}

// NewClient returns a client for the gateway at gateway, sending from conn. The caller binds conn,
// so that the requests leave the way the forwarded traffic will come in.
func NewClient(conn net.PacketConn, gateway netip.AddrPort) (*Client, error) {
	if conn == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("packet conn"))
	}

	if !gateway.IsValid() || gateway.Port() == 0 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: the gateway must be an address and port, got %s", altshiftErrors.ErrValidationError, gateway),
			gateway,
		)
	}

	// Four attempts, 250 ms doubling: 3.75 s in all, well inside a 60-second lease, where the
	// RFC's nine (64 s) would outlast it.
	return &Client{conn: conn, gateway: gateway, firstWait: 250 * time.Millisecond, attempts: 4, now: time.Now}, nil
}

// Map asks for a mapping, or renews the one it describes.
func (client *Client) Map(ctx context.Context, request *Request) (*Mapping, error) {
	if client == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("client"))
	}

	if request == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("request"))
	}

	if request.Protocol != UDP && request.Protocol != TCP {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: unsupported protocol %s", altshiftErrors.ErrValidationError, request.Protocol),
		)
	}

	lifetime := request.Lifetime / time.Second
	if lifetime < 0 || lifetime > 1<<32-1 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: lifetime %s is out of range", altshiftErrors.ErrValidationError, request.Lifetime),
		)
	}

	message := make([]byte, requestSize)
	message[0] = version
	message[1] = byte(request.Protocol)
	binary.BigEndian.PutUint16(message[4:6], request.InternalPort)
	binary.BigEndian.PutUint16(message[6:8], request.SuggestedExternalPort)
	binary.BigEndian.PutUint32(message[8:12], uint32(lifetime))

	answer, err := client.exchange(ctx, message, byte(request.Protocol), mappingResponseSize)
	if err != nil {
		return nil, fmt.Errorf("exchange: %w", err)
	}

	return &Mapping{
		Protocol:     request.Protocol,
		InternalPort: binary.BigEndian.Uint16(answer[8:10]),
		ExternalPort: binary.BigEndian.Uint16(answer[10:12]),
		Lifetime:     time.Duration(binary.BigEndian.Uint32(answer[12:16])) * time.Second,
		Obtained:     client.now(),
	}, nil
}

// ExternalAddress asks the gateway for its public address: the one peers reach the mapping at.
func (client *Client) ExternalAddress(ctx context.Context) (netip.Addr, error) {
	if client == nil {
		return netip.Addr{}, altshiftErrors.NewWithTrace(nil_error.New("client"))
	}

	answer, err := client.exchange(ctx, []byte{version, opExternalAddress}, opExternalAddress, addressResponseSize)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("exchange: %w", err)
	}

	return netip.AddrFrom4([4]byte(answer[8:12])), nil
}

// exchange sends message and returns the first well-formed answer to it, retrying with a doubling
// wait. An answer from another address, or to another opcode, is ignored rather than trusted.
func (client *Client) exchange(ctx context.Context, message []byte, opcode byte, size int) ([]byte, error) {
	gateway := net.UDPAddrFromAddrPort(client.gateway)
	buffer := make([]byte, 64)
	wait := client.firstWait

	for range client.attempts {
		if err := ctx.Err(); err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("context: %w", err))
		}

		if _, err := client.conn.WriteTo(message, gateway); err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("conn write to: %w", err), client.gateway)
		}

		deadline := client.now().Add(wait)
		if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
			deadline = contextDeadline
		}

		if err := client.conn.SetReadDeadline(deadline); err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("conn set read deadline: %w", err))
		}

		for {
			n, from, err := client.conn.ReadFrom(buffer)
			if err != nil {
				if netError, ok := errors.AsType[net.Error](err); ok && netError != nil && netError.Timeout() {
					break
				}

				return nil, altshiftErrors.NewWithTrace(fmt.Errorf("conn read from: %w", err))
			}

			if udp, ok := from.(*net.UDPAddr); !ok || udp.AddrPort().Addr().Unmap() != client.gateway.Addr().Unmap() {
				continue
			}

			answer := buffer[:n]
			if n < 4 || answer[0] != version || answer[1] != responseBit+opcode {
				continue
			}

			if code := binary.BigEndian.Uint16(answer[2:4]); code != 0 {
				name, known := resultNames[code]
				if !known {
					name = "unknown result"
				}

				return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s (%d)", ErrResult, name, code), code)
			}

			if n < size {
				continue
			}

			return append([]byte(nil), answer[:size]...), nil
		}

		wait *= 2
	}

	return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w after %d attempts", ErrNoAnswer, client.attempts), client.gateway)
}

// Keeper holds a mapping for as long as its context lives.
type Keeper struct {
	Client  *Client
	Request *Request

	// Changed is called with each new mapping whose external port differs from the last one, and
	// with nil when the lease lapsed unrenewed. A program advertises the port it is given and
	// stops advertising on nil.
	Changed func(mapping *Mapping)
	// Renewed, if set, is called after every successful renewal, changed or not.
	Renewed func(mapping *Mapping)

	// Address asks the gateway for its public address whenever the port changes, into the
	// mapping's ExternalAddress, which a renewal carries over. The question is asked of the same
	// client, between renewals, since the client is not safe for concurrent use; an unanswered one
	// leaves the address zero and the mapping reported all the same.
	Address bool

	// RetryDelay is the pause after a failed attempt before the next; 2 s when zero.
	RetryDelay time.Duration
}

// externalAddress asks for the gateway's public address within the mapping's lease, and gives the
// zero address when it is not told.
func (keeper *Keeper) externalAddress(ctx context.Context, mapping *Mapping) netip.Addr {
	addressContext, cancel := context.WithDeadline(ctx, mapping.Expires())
	defer cancel()

	address, err := keeper.Client.ExternalAddress(addressContext)
	if err != nil {
		return netip.Addr{}
	}

	return address
}

// Run renews at half of each granted lifetime, retries while the lease is still live, and reports
// a lapse as soon as the lease's lifetime has passed with no renewal: whatever the retry count,
// the port is not advertised past its expiry. It returns when ctx ends.
func (keeper *Keeper) Run(ctx context.Context) error {
	if keeper == nil || keeper.Client == nil || keeper.Request == nil || keeper.Changed == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("keeper, its client, request or Changed"))
	}

	retryDelay := keeper.RetryDelay
	if retryDelay <= 0 {
		retryDelay = 2 * time.Second
	}

	var current *Mapping

	for {
		// An attempt may not outlive the lease it renews: its retries would otherwise keep a
		// lapsed port advertised for as long as they take.
		attemptContext, cancel := ctx, context.CancelFunc(func() {})
		if current != nil {
			attemptContext, cancel = context.WithDeadline(ctx, current.Expires())
		}

		mapping, err := keeper.Client.Map(attemptContext, keeper.Request)
		cancel()

		now := keeper.Client.now()

		var wait time.Duration

		switch {
		case err == nil:
			if current == nil || mapping.ExternalPort != current.ExternalPort {
				if keeper.Address {
					mapping.ExternalAddress = keeper.externalAddress(ctx, mapping)
				}

				keeper.Changed(mapping)
			} else {
				mapping.ExternalAddress = current.ExternalAddress
			}

			if keeper.Renewed != nil {
				keeper.Renewed(mapping)
			}

			current = mapping
			wait = max(mapping.Lifetime/2, retryDelay/4)
		case ctx.Err() != nil:
			if current != nil {
				keeper.Changed(nil)
			}

			return nil
		default:
			if current != nil && !now.Before(current.Expires()) {
				current = nil
				keeper.Changed(nil)
			}

			wait = retryDelay
			if current != nil {
				wait = min(wait, max(current.Expires().Sub(now), time.Millisecond))
			}
		}

		timer := time.NewTimer(wait)

		select {
		case <-ctx.Done():
			timer.Stop()

			if current != nil {
				keeper.Changed(nil)
			}

			return nil
		case <-timer.C:
		}
	}
}
