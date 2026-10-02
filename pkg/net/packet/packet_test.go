package packet

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net/netip"
	"testing"
)

// The reference packets were produced by an independent encoder (gopacket, with lengths fixed and
// checksums computed): a SYN/ACK from port 40000 to 443 with an MSS option of 1460 and the
// payload "hi!".
const (
	referenceIpv4 = "4500002f0000000040068e8dc0000201c63364079c4001bb010203040a0b0c0d6012040066540000020405b4686921"
	referenceIpv6 = "60000000001b064020010db800000000000000000000000120010db80000000000000000000000079c4001bb" +
		"010203040a0b0c0d60120400f7160000020405b4686921"
)

func referenceTcp() *Tcp {
	return &Tcp{
		SourcePort:      40000,
		DestinationPort: 443,
		Sequence:        0x01020304,
		Acknowledgement: 0x0a0b0c0d,
		Flags:           TcpFlagSyn | TcpFlagAck,
		Window:          1024,
		Options:         []byte{2, 4, 0x05, 0xb4},
		Payload:         []byte("hi!"),
	}
}

func mustHex(t *testing.T, data string) []byte {
	t.Helper()

	decoded, err := hex.DecodeString(data)
	if err != nil {
		t.Fatalf("hex decode string: %v", err)
	}

	return decoded
}

func TestEncodeMatchesReference(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		version     int
		source      string
		destination string
		expected    string
	}{
		{name: "ipv4", version: 4, source: "192.0.2.1", destination: "198.51.100.7", expected: referenceIpv4},
		{name: "ipv6", version: 6, source: "2001:db8::1", destination: "2001:db8::7", expected: referenceIpv6},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := netip.MustParseAddr(testCase.source)
			destination := netip.MustParseAddr(testCase.destination)

			segment, err := AppendTcp(nil, referenceTcp(), source, destination)
			if err != nil {
				t.Fatalf("AppendTcp() = %v", err)
			}

			encoded, err := AppendIp(nil, &Ip{
				Version: testCase.version, Source: source, Destination: destination, Protocol: ProtocolTcp,
				Payload: segment,
			})
			if err != nil {
				t.Fatalf("AppendIp() = %v", err)
			}

			if expected := mustHex(t, testCase.expected); !bytes.Equal(encoded, expected) {
				t.Errorf("encoded =\n%x\nwant\n%x", encoded, expected)
			}
		})
	}
}

func TestDecodeReference(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		data        string
		version     int
		source      string
		destination string
	}{
		{name: "ipv4", data: referenceIpv4, version: 4, source: "192.0.2.1", destination: "198.51.100.7"},
		{name: "ipv6", data: referenceIpv6, version: 6, source: "2001:db8::1", destination: "2001:db8::7"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ip, err := ParseIp(mustHex(t, testCase.data))
			if err != nil {
				t.Fatalf("ParseIp() = %v", err)
			}

			if ip.Version != testCase.version || ip.Protocol != ProtocolTcp || ip.HopLimit != 64 || ip.Fragmented() {
				t.Errorf("ip = %+v", ip)
			}
			if ip.Source.String() != testCase.source || ip.Destination.String() != testCase.destination {
				t.Errorf("addresses = %v -> %v, want %s -> %s", ip.Source, ip.Destination, testCase.source, testCase.destination)
			}

			tcp, err := ParseTcp(ip.Payload)
			if err != nil {
				t.Fatalf("ParseTcp() = %v", err)
			}

			expected := referenceTcp()
			if tcp.SourcePort != expected.SourcePort || tcp.DestinationPort != expected.DestinationPort ||
				tcp.Sequence != expected.Sequence || tcp.Acknowledgement != expected.Acknowledgement ||
				tcp.Flags != expected.Flags || tcp.Window != expected.Window ||
				!bytes.Equal(tcp.Options, expected.Options) || !bytes.Equal(tcp.Payload, expected.Payload) {
				t.Errorf("tcp = %+v, want %+v", tcp, expected)
			}
		})
	}
}

// ipv6With builds an IPv6 packet whose payload is the given extension headers followed by a TCP
// segment from port 1234.
func ipv6With(t *testing.T, firstHeader uint8, extensionHeaders []byte) []byte {
	t.Helper()

	source := netip.MustParseAddr("2001:db8::1")
	destination := netip.MustParseAddr("2001:db8::2")

	segment, err := AppendTcp(nil, &Tcp{SourcePort: 1234, Flags: TcpFlagSyn}, source, destination)
	if err != nil {
		t.Fatalf("AppendTcp() = %v", err)
	}

	data, err := AppendIp(nil, &Ip{
		Version: 6, Source: source, Destination: destination, Protocol: firstHeader,
		Payload: append(append([]byte(nil), extensionHeaders...), segment...),
	})
	if err != nil {
		t.Fatalf("AppendIp() = %v", err)
	}

	return data
}

func TestParseIpv6ExtensionHeaders(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                  string
		firstHeader           uint8
		extensionHeaders      []byte
		expectedOffset        int
		expectedMoreFragments bool
	}{
		{
			name:        "hop-by-hop",
			firstHeader: ProtocolHopByHop,
			// Next header TCP, length 0 (eight bytes), PadN of four bytes.
			extensionHeaders: []byte{ProtocolTcp, 0, 1, 4, 0, 0, 0, 0},
		},
		{
			name:             "destination options of sixteen bytes",
			firstHeader:      ProtocolIpv6DestinationOptions,
			extensionHeaders: append([]byte{ProtocolTcp, 1, 1, 12}, make([]byte, 12)...),
		},
		{
			name:        "hop-by-hop then routing",
			firstHeader: ProtocolHopByHop,
			extensionHeaders: []byte{
				ProtocolIpv6Routing, 0, 1, 4, 0, 0, 0, 0,
				ProtocolTcp, 0, 0, 0, 0, 0, 0, 0,
			},
		},
		{
			name:        "authentication header",
			firstHeader: ProtocolAh,
			// Next header TCP, payload length 4: (4 + 2) * 4 = 24 bytes.
			extensionHeaders: append([]byte{ProtocolTcp, 4}, make([]byte, 22)...),
		},
		{
			name:        "first fragment",
			firstHeader: ProtocolIpv6Fragment,
			// Offset 0, M set.
			extensionHeaders:      []byte{ProtocolTcp, 0, 0, 1, 0, 0, 0, 1},
			expectedMoreFragments: true,
		},
		{
			name:        "later fragment",
			firstHeader: ProtocolIpv6Fragment,
			// Offset 185 eight-byte units (1480 bytes), M clear.
			extensionHeaders: []byte{ProtocolTcp, 0, 0x05, 0xc8, 0, 0, 0, 1},
			expectedOffset:   1480,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ip, err := ParseIp(ipv6With(t, testCase.firstHeader, testCase.extensionHeaders))
			if err != nil {
				t.Fatalf("ParseIp() = %v", err)
			}

			if ip.Protocol != ProtocolTcp {
				t.Errorf("Protocol = %d, want %d", ip.Protocol, ProtocolTcp)
			}
			if ip.FragmentOffset != testCase.expectedOffset || ip.MoreFragments != testCase.expectedMoreFragments {
				t.Errorf(
					"fragment = %d/%v, want %d/%v",
					ip.FragmentOffset, ip.MoreFragments, testCase.expectedOffset, testCase.expectedMoreFragments,
				)
			}

			tcp, err := ParseTcp(ip.Payload)
			if err != nil {
				t.Fatalf("ParseTcp() = %v", err)
			}
			if tcp.SourcePort != 1234 {
				t.Errorf("SourcePort = %d, want 1234: the extension headers were not skipped exactly", tcp.SourcePort)
			}
		})
	}
}

func TestParseIpv4(t *testing.T) {
	t.Parallel()

	reference := mustHex(t, referenceIpv4)

	withOptions := append([]byte(nil), reference[:20]...)
	withOptions[0] = 0x46 // IHL 6: four bytes of options
	withOptions[3] += 4   // total length
	withOptions = append(withOptions, 1, 1, 1, 0)
	withOptions = append(withOptions, reference[20:]...)

	padded := append(append([]byte(nil), reference...), make([]byte, 10)...)

	laterFragment := append([]byte(nil), reference...)
	laterFragment[6], laterFragment[7] = 0x00, 0xb9 // offset 185 units

	firstFragment := append([]byte(nil), reference...)
	firstFragment[6] = 0x20 // MF

	testCases := []struct {
		name                  string
		data                  []byte
		expectedPayloadLength int
		expectedOffset        int
		expectedMoreFragments bool
	}{
		{name: "options are skipped", data: withOptions, expectedPayloadLength: 27},
		{name: "link padding is trimmed", data: padded, expectedPayloadLength: 27},
		{name: "later fragment", data: laterFragment, expectedPayloadLength: 27, expectedOffset: 1480},
		{name: "first fragment", data: firstFragment, expectedPayloadLength: 27, expectedMoreFragments: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ip, err := ParseIp(testCase.data)
			if err != nil {
				t.Fatalf("ParseIp() = %v", err)
			}

			if len(ip.Payload) != testCase.expectedPayloadLength {
				t.Errorf("len(Payload) = %d, want %d", len(ip.Payload), testCase.expectedPayloadLength)
			}
			if ip.FragmentOffset != testCase.expectedOffset || ip.MoreFragments != testCase.expectedMoreFragments {
				t.Errorf(
					"fragment = %d/%v, want %d/%v",
					ip.FragmentOffset, ip.MoreFragments, testCase.expectedOffset, testCase.expectedMoreFragments,
				)
			}
			if tcp, err := ParseTcp(ip.Payload); err != nil || tcp.SourcePort != 40000 {
				t.Errorf("ParseTcp() = %+v, %v", tcp, err)
			}
		})
	}
}

func TestParseRejects(t *testing.T) {
	t.Parallel()

	reference := mustHex(t, referenceIpv4)
	referenceV6 := mustHex(t, referenceIpv6)

	shortIhl := append([]byte(nil), reference...)
	shortIhl[0] = 0x44

	longTotal := append([]byte(nil), reference...)
	longTotal[3] = 0xff

	totalUnderHeader := append([]byte(nil), reference...)
	totalUnderHeader[2], totalUnderHeader[3] = 0, 10

	longPayload := append([]byte(nil), referenceV6...)
	longPayload[5] = 0xff

	jumbogram := append([]byte(nil), referenceV6[:40]...)
	jumbogram[4], jumbogram[5], jumbogram[6] = 0, 0, ProtocolHopByHop

	truncatedExtension := ipv6With(t, ProtocolIpv6DestinationOptions, []byte{ProtocolTcp, 200, 0, 0, 0, 0, 0, 0})

	loop := ipv6With(t, ProtocolHopByHop, nil)
	loop = append(loop[:40], make([]byte, 8*(maxIpv6ExtensionHeaders+1))...)
	for i := 40; i < len(loop); i += 8 {
		loop[i] = ProtocolIpv6DestinationOptions
	}
	binary.BigEndian.PutUint16(loop[4:], uint16(len(loop)-40)) //nolint:gosec // A few hundred bytes.

	testCases := []struct {
		name     string
		data     []byte
		expected error
	}{
		{name: "empty", data: nil, expected: ErrMalformed},
		{name: "unknown version", data: []byte{0x95, 0, 0, 0}, expected: ErrMalformed},
		{name: "truncated ipv4", data: reference[:12], expected: ErrMalformed},
		{name: "ipv4 header length under twenty", data: shortIhl, expected: ErrMalformed},
		{name: "ipv4 total length beyond the data", data: longTotal, expected: ErrMalformed},
		{name: "ipv4 total length under the header", data: totalUnderHeader, expected: ErrMalformed},
		{name: "truncated ipv6", data: referenceV6[:30], expected: ErrMalformed},
		{name: "ipv6 payload length beyond the data", data: longPayload, expected: ErrMalformed},
		{name: "ipv6 jumbogram", data: jumbogram, expected: ErrUnsupported},
		{name: "ipv6 extension header beyond the data", data: truncatedExtension, expected: ErrMalformed},
		{name: "too many ipv6 extension headers", data: loop, expected: ErrUnsupported},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if ip, err := ParseIp(testCase.data); !errors.Is(err, testCase.expected) {
				t.Errorf("ParseIp() = %+v, %v, want %v", ip, err, testCase.expected)
			}
		})
	}
}

func TestParseTcpRejects(t *testing.T) {
	t.Parallel()

	segment := mustHex(t, referenceIpv4)[20:]

	shortOffset := append([]byte(nil), segment...)
	shortOffset[12] = 0x40

	longOffset := append([]byte(nil), segment[:24]...)
	longOffset[12] = 0xf0

	testCases := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "truncated", data: segment[:19]},
		{name: "data offset under five", data: shortOffset},
		{name: "data offset beyond the data", data: longOffset},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if tcp, err := ParseTcp(testCase.data); !errors.Is(err, ErrMalformed) {
				t.Errorf("ParseTcp() = %+v, %v, want %v", tcp, err, ErrMalformed)
			}
		})
	}
}

func TestAppendTcp(t *testing.T) {
	t.Parallel()

	ipv4 := netip.MustParseAddr("192.0.2.1")
	ipv6 := netip.MustParseAddr("2001:db8::1")

	t.Run("options are padded to a word", func(t *testing.T) {
		t.Parallel()

		segment, err := AppendTcp(nil, &Tcp{Options: []byte{1, 1, 1}}, ipv4, ipv4)
		if err != nil {
			t.Fatalf("AppendTcp() = %v", err)
		}

		tcp, err := ParseTcp(segment)
		if err != nil {
			t.Fatalf("ParseTcp() = %v", err)
		}
		if !bytes.Equal(tcp.Options, []byte{1, 1, 1, 0}) {
			t.Errorf("Options = %v, want [1 1 1 0]", tcp.Options)
		}
	})

	t.Run("appends after what b holds", func(t *testing.T) {
		t.Parallel()

		prefix := []byte{0xaa, 0xbb}
		segment, err := AppendTcp(prefix, referenceTcp(), ipv4, netip.MustParseAddr("198.51.100.7"))
		if err != nil {
			t.Fatalf("AppendTcp() = %v", err)
		}

		if !bytes.Equal(segment[2:], mustHex(t, referenceIpv4)[20:]) || segment[0] != 0xaa {
			t.Errorf("segment = %x", segment)
		}
	})

	testCases := []struct {
		name        string
		tcp         *Tcp
		source      netip.Addr
		destination netip.Addr
	}{
		{name: "nil", tcp: nil, source: ipv4, destination: ipv4},
		{name: "mixed versions", tcp: &Tcp{}, source: ipv4, destination: ipv6},
		{name: "mapped address against ipv4", tcp: &Tcp{}, source: netip.AddrFrom16(ipv4.As16()), destination: ipv4},
		{
			name: "two mapped addresses", tcp: &Tcp{},
			source: netip.AddrFrom16(ipv4.As16()), destination: netip.AddrFrom16(ipv4.As16()),
		},
		{name: "invalid address", tcp: &Tcp{}, source: netip.Addr{}, destination: ipv4},
		{name: "options too long", tcp: &Tcp{Options: make([]byte, 41)}, source: ipv4, destination: ipv4},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := AppendTcp(nil, testCase.tcp, testCase.source, testCase.destination); !errors.Is(err, ErrInvalidArgument) {
				t.Errorf("AppendTcp() = %v, want %v", err, ErrInvalidArgument)
			}
		})
	}
}

func TestAppendIpRejects(t *testing.T) {
	t.Parallel()

	ipv4 := netip.MustParseAddr("192.0.2.1")
	ipv6 := netip.MustParseAddr("2001:db8::1")

	testCases := []struct {
		name string
		ip   *Ip
	}{
		{name: "nil", ip: nil},
		{name: "unknown version", ip: &Ip{Version: 5, Source: ipv4, Destination: ipv4}},
		{name: "ipv4 with ipv6 addresses", ip: &Ip{Version: 4, Source: ipv6, Destination: ipv6}},
		{name: "ipv6 with ipv4 addresses", ip: &Ip{Version: 6, Source: ipv4, Destination: ipv4}},
		{name: "ipv4 payload too long", ip: &Ip{Version: 4, Source: ipv4, Destination: ipv4, Payload: make([]byte, 0xffff)}},
		{name: "ipv6 payload too long", ip: &Ip{Version: 6, Source: ipv6, Destination: ipv6, Payload: make([]byte, 0x10000)}},
		{name: "ipv4 fragment offset not a multiple of eight", ip: &Ip{Version: 4, Source: ipv4, Destination: ipv4, FragmentOffset: 3}},
		{name: "negative ipv4 fragment offset", ip: &Ip{Version: 4, Source: ipv4, Destination: ipv4, FragmentOffset: -8}},
		{name: "ipv4 fragment offset beyond the field", ip: &Ip{Version: 4, Source: ipv4, Destination: ipv4, FragmentOffset: 8 * 8192}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := AppendIp(nil, testCase.ip); !errors.Is(err, ErrInvalidArgument) {
				t.Errorf("AppendIp() = %v, want %v", err, ErrInvalidArgument)
			}
		})
	}
}

func TestAppendIpv4FragmentRoundTrip(t *testing.T) {
	t.Parallel()

	source := netip.MustParseAddr("192.0.2.1")

	data, err := AppendIp(nil, &Ip{
		Version: 4, Source: source, Destination: source, Protocol: ProtocolUdp,
		FragmentOffset: 1480, MoreFragments: true, Payload: []byte{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("AppendIp() = %v", err)
	}

	// A correct header checksum sums, with the checksum field in place, to zero.
	if sum := checksumFinish(checksumAdd(0, data[:20])); sum != 0 {
		t.Errorf("header checksum does not verify: %#04x", sum)
	}

	ip, err := ParseIp(data)
	if err != nil {
		t.Fatalf("ParseIp() = %v", err)
	}
	if ip.FragmentOffset != 1480 || !ip.MoreFragments || ip.Protocol != ProtocolUdp || !bytes.Equal(ip.Payload, []byte{1, 2, 3}) {
		t.Errorf("ip = %+v", ip)
	}
}

func TestTcpFlagsHas(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		flags    TcpFlags
		query    TcpFlags
		expected bool
	}{
		{name: "both set", flags: TcpFlagSyn | TcpFlagAck, query: TcpFlagSyn | TcpFlagAck, expected: true},
		{name: "one missing", flags: TcpFlagSyn, query: TcpFlagSyn | TcpFlagAck, expected: false},
		{name: "extra set", flags: TcpFlagSyn | TcpFlagAck | TcpFlagEce, query: TcpFlagSyn, expected: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.flags.Has(testCase.query); got != testCase.expected {
				t.Errorf("Has() = %v, want %v", got, testCase.expected)
			}
		})
	}
}

// A later fragment's data is from the middle of the datagram: the header its fragment header names
// is not behind it, and whatever bytes are there must not be parsed as one.
func TestParseIpv6LaterFragmentStopsAtTheFragmentHeader(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		data []byte
	}{
		// Read as a destination options header, this would claim 2048 bytes and fail as truncated.
		{name: "data that would read as a long extension header", data: []byte{ProtocolTcp, 0xff, 1, 2, 3, 4, 5, 6}},
		// Read as one, this would end the walk at UDP with the wrong payload.
		{name: "data that would read as a short extension header", data: []byte{ProtocolUdp, 0, 1, 2, 3, 4, 5, 6, 7, 8}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := netip.MustParseAddr("2001:db8::1")
			fragmentHeader := []byte{ProtocolIpv6DestinationOptions, 0, 0x05, 0xc8, 0, 0, 0, 1}

			data, err := AppendIp(nil, &Ip{
				Version: 6, Source: source, Destination: source, Protocol: ProtocolIpv6Fragment,
				Payload: append(fragmentHeader, testCase.data...),
			})
			if err != nil {
				t.Fatalf("AppendIp() = %v", err)
			}

			ip, err := ParseIp(data)
			if err != nil {
				t.Fatalf("ParseIp() = %v", err)
			}

			if ip.Protocol != ProtocolIpv6DestinationOptions || ip.FragmentOffset != 1480 || !bytes.Equal(ip.Payload, testCase.data) {
				t.Errorf("ip = %+v, want protocol %d, offset 1480 and payload %v", ip, ProtocolIpv6DestinationOptions, testCase.data)
			}
		})
	}
}
