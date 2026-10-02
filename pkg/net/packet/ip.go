package packet

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

const (
	ipv4MinimumHeaderLength = 20
	ipv6HeaderLength        = 40
	ipv4MoreFragmentsFlag   = 0x2000
	ipv4FragmentOffsetMask  = 0x1fff
	ipv6MoreFragmentsFlag   = 0x0001
	ipv6FragmentHeaderSize  = 8
	defaultHopLimit         = 64
	maxIpv6ExtensionHeaders = 32
)

// Ip is an IPv4 or IPv6 header and what it carries.
type Ip struct {
	// Version is 4 or 6.
	Version     int
	Source      netip.Addr
	Destination netip.Addr
	// Protocol is the protocol of Payload: for IPv6, the next header that follows the extension
	// headers, which decoding skips. For a fragment other than the first, Payload is a piece of the
	// original datagram's data and Protocol the first header that datagram carried, as the
	// fragment header names it; nothing behind the fragment header is parsed.
	Protocol uint8
	// HopLimit is the IPv4 TTL or the IPv6 hop limit. Zero encodes as 64.
	HopLimit uint8
	// FragmentOffset is the offset of Payload within the original datagram, in bytes. A packet that
	// is not a fragment, and the first fragment of one that is, have zero.
	FragmentOffset int
	// MoreFragments is set on every fragment but the last.
	MoreFragments bool
	// Payload is what follows the headers, trimmed to the length the header gives, so that link
	// padding never reaches it. It aliases the decoded data.
	Payload []byte
}

// Fragmented reports whether the packet is part of a fragmented datagram.
func (ip *Ip) Fragmented() bool {
	return ip.FragmentOffset != 0 || ip.MoreFragments
}

// ParseIp decodes an IPv4 or IPv6 packet, choosing the version by the first nibble of data.
func ParseIp(data []byte) (*Ip, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty", ErrMalformed)
	}

	switch version := data[0] >> 4; version {
	case 4:
		return parseIpv4(data)
	case 6:
		return parseIpv6(data)
	default:
		return nil, fmt.Errorf("%w: ip version %d", ErrMalformed, version)
	}
}

func parseIpv4(data []byte) (*Ip, error) {
	if len(data) < ipv4MinimumHeaderLength {
		return nil, fmt.Errorf("%w: ipv4 header truncated at %d bytes", ErrMalformed, len(data))
	}

	headerLength := int(data[0]&0x0f) * 4
	if headerLength < ipv4MinimumHeaderLength {
		return nil, fmt.Errorf("%w: ipv4 header length %d", ErrMalformed, headerLength)
	}

	totalLength := int(binary.BigEndian.Uint16(data[2:4]))
	if totalLength < headerLength {
		return nil, fmt.Errorf(
			"%w: ipv4 total length %d is shorter than its header (%d)", ErrMalformed, totalLength, headerLength,
		)
	}
	if totalLength > len(data) {
		return nil, fmt.Errorf(
			"%w: ipv4 total length %d exceeds the %d bytes available", ErrMalformed, totalLength, len(data),
		)
	}

	flagsAndOffset := binary.BigEndian.Uint16(data[6:8])

	return &Ip{
		Version:        4,
		Source:         netip.AddrFrom4([4]byte(data[12:16])),
		Destination:    netip.AddrFrom4([4]byte(data[16:20])),
		Protocol:       data[9],
		HopLimit:       data[8],
		FragmentOffset: int(flagsAndOffset&ipv4FragmentOffsetMask) * 8,
		MoreFragments:  flagsAndOffset&ipv4MoreFragmentsFlag != 0,
		Payload:        data[headerLength:totalLength],
	}, nil
}

func parseIpv6(data []byte) (*Ip, error) {
	if len(data) < ipv6HeaderLength {
		return nil, fmt.Errorf("%w: ipv6 header truncated at %d bytes", ErrMalformed, len(data))
	}

	payloadLength := int(binary.BigEndian.Uint16(data[4:6]))
	// A zero payload length is how a jumbogram (RFC 2675) announces that its length is in a
	// hop-by-hop option instead; a zero-length payload is otherwise indistinguishable from one.
	if payloadLength == 0 && data[6] == ProtocolHopByHop {
		return nil, fmt.Errorf("%w: ipv6 jumbogram", ErrUnsupported)
	}
	if ipv6HeaderLength+payloadLength > len(data) {
		return nil, fmt.Errorf(
			"%w: ipv6 payload length %d exceeds the %d bytes available",
			ErrMalformed, payloadLength, len(data)-ipv6HeaderLength,
		)
	}

	ip := &Ip{
		Version:     6,
		Source:      netip.AddrFrom16([16]byte(data[8:24])),
		Destination: netip.AddrFrom16([16]byte(data[24:40])),
		HopLimit:    data[7],
	}

	nextHeader := data[6]
	payload := data[ipv6HeaderLength : ipv6HeaderLength+payloadLength]

	for range maxIpv6ExtensionHeaders {
		var headerLength int

		switch nextHeader {
		case ProtocolHopByHop, ProtocolIpv6Routing, ProtocolIpv6DestinationOptions, ProtocolMobility,
			ProtocolHip, ProtocolShim6, ProtocolExperimental1, ProtocolExperimental2:
			// The generic extension header format: length in eight-byte units, not counting the first.
			if len(payload) < 2 {
				return nil, fmt.Errorf("%w: ipv6 extension header %d truncated", ErrMalformed, nextHeader)
			}
			headerLength = (int(payload[1]) + 1) * 8
		case ProtocolAh:
			// The authentication header counts four-byte units, not counting the first two (RFC 4302).
			if len(payload) < 2 {
				return nil, fmt.Errorf("%w: ipv6 authentication header truncated", ErrMalformed)
			}
			headerLength = (int(payload[1]) + 2) * 4
		case ProtocolIpv6Fragment:
			headerLength = ipv6FragmentHeaderSize
			if len(payload) < headerLength {
				return nil, fmt.Errorf("%w: ipv6 fragment header truncated", ErrMalformed)
			}

			offsetAndFlags := binary.BigEndian.Uint16(payload[2:4])
			ip.FragmentOffset = int(offsetAndFlags>>3) * 8
			ip.MoreFragments = offsetAndFlags&ipv6MoreFragmentsFlag != 0

			// A later fragment carries data from the middle of the original datagram, not the
			// headers its next header names (RFC 8200 section 4.5): the walk ends here.
			if ip.FragmentOffset != 0 {
				ip.Protocol = payload[0]
				ip.Payload = payload[headerLength:]

				return ip, nil
			}
		default:
			ip.Protocol = nextHeader
			ip.Payload = payload

			return ip, nil
		}

		if len(payload) < headerLength {
			return nil, fmt.Errorf("%w: ipv6 extension header %d truncated", ErrMalformed, nextHeader)
		}

		nextHeader = payload[0]
		payload = payload[headerLength:]
	}

	return nil, fmt.Errorf("%w: more than %d ipv6 extension headers", ErrUnsupported, maxIpv6ExtensionHeaders)
}

// AppendIp appends ip encoded as a header followed by its payload to b. An IPv4 header is written
// without options and with its checksum; an IPv6 header is written without extension headers, so
// Protocol is its next header and any extension headers wanted belong at the start of Payload.
// Source and Destination must both be of the version given.
func AppendIp(b []byte, ip *Ip) ([]byte, error) {
	if ip == nil {
		return nil, fmt.Errorf("%w: nil ip", ErrInvalidArgument)
	}

	hopLimit := ip.HopLimit
	if hopLimit == 0 {
		hopLimit = defaultHopLimit
	}

	switch ip.Version {
	case 4:
		if !ip.Source.Is4() || !ip.Destination.Is4() {
			return nil, fmt.Errorf("%w: ipv4 header with non-ipv4 addresses", ErrInvalidArgument)
		}

		totalLength := ipv4MinimumHeaderLength + len(ip.Payload)
		if totalLength > 0xffff {
			return nil, fmt.Errorf("%w: ipv4 total length %d", ErrInvalidArgument, totalLength)
		}
		if ip.FragmentOffset < 0 || ip.FragmentOffset%8 != 0 || ip.FragmentOffset/8 > ipv4FragmentOffsetMask {
			return nil, fmt.Errorf("%w: ipv4 fragment offset %d", ErrInvalidArgument, ip.FragmentOffset)
		}

		flagsAndOffset := uint16(ip.FragmentOffset / 8) //nolint:gosec // Range checked above.
		if ip.MoreFragments {
			flagsAndOffset |= ipv4MoreFragmentsFlag
		}

		start := len(b)
		b = append(b, 0x45, 0)
		b = binary.BigEndian.AppendUint16(b, uint16(totalLength)) //nolint:gosec // Range checked above.
		b = append(b, 0, 0)                                       // identification
		b = binary.BigEndian.AppendUint16(b, flagsAndOffset)
		b = append(b, hopLimit, ip.Protocol, 0, 0) // checksum, filled in below
		b = appendAddress(b, ip.Source)
		b = appendAddress(b, ip.Destination)
		binary.BigEndian.PutUint16(b[start+10:], checksumFinish(checksumAdd(0, b[start:])))
	case 6:
		if !ip.Source.Is6() || !ip.Destination.Is6() {
			return nil, fmt.Errorf("%w: ipv6 header with non-ipv6 addresses", ErrInvalidArgument)
		}
		if len(ip.Payload) > 0xffff {
			return nil, fmt.Errorf("%w: ipv6 payload length %d", ErrInvalidArgument, len(ip.Payload))
		}

		b = append(b, 0x60, 0, 0, 0)
		b = binary.BigEndian.AppendUint16(b, uint16(len(ip.Payload))) //nolint:gosec // Range checked above.
		b = append(b, ip.Protocol, hopLimit)
		b = appendAddress(b, ip.Source)
		b = appendAddress(b, ip.Destination)
	default:
		return nil, fmt.Errorf("%w: ip version %d", ErrInvalidArgument, ip.Version)
	}

	return append(b, ip.Payload...), nil
}
