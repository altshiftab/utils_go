// Package packet encodes and decodes the IPv4, IPv6 and TCP headers of single packets, as a raw
// socket sends them and a packet socket captures them.
//
// Decoding is a bounds-checked reading of the header fields, with no reassembly and no stream
// state: a packet is taken as it comes. Checksums are written on encode and not verified on
// decode, since a captured packet's checksum is often left to the network card to fill in.
// Errors carry no stack trace, so that a decoder fed every captured packet does not pay for one
// on each that does not parse.
package packet

import (
	"errors"
	"net/netip"
)

var (
	// ErrMalformed is reported for a packet whose headers do not parse: too short for what its
	// header says, or with a field no valid header carries.
	ErrMalformed = errors.New("malformed packet")
	// ErrUnsupported is reported for a packet that is well formed but that this package does not
	// handle, such as an IPv6 jumbogram.
	ErrUnsupported = errors.New("unsupported packet")
	// ErrInvalidArgument is reported when an encoder is given something it cannot encode.
	ErrInvalidArgument = errors.New("invalid argument")
)

// IP protocol numbers, as an IPv4 header's protocol field and an IPv6 next header field carry them.
const (
	ProtocolHopByHop               uint8 = 0
	ProtocolTcp                    uint8 = 6
	ProtocolUdp                    uint8 = 17
	ProtocolIpv6Routing            uint8 = 43
	ProtocolIpv6Fragment           uint8 = 44
	ProtocolEsp                    uint8 = 50
	ProtocolAh                     uint8 = 51
	ProtocolIpv6NoNextHeader       uint8 = 59
	ProtocolIpv6DestinationOptions uint8 = 60
	ProtocolMobility               uint8 = 135
	ProtocolHip                    uint8 = 139
	ProtocolShim6                  uint8 = 140
	ProtocolExperimental1          uint8 = 253
	ProtocolExperimental2          uint8 = 254
)

// checksumAdd folds data into the running one's-complement sum of 16-bit big-endian words, an odd
// trailing byte padded with zero.
func checksumAdd(sum uint32, data []byte) uint32 {
	for len(data) >= 2 {
		sum += uint32(data[0])<<8 | uint32(data[1])
		data = data[2:]
	}
	if len(data) == 1 {
		sum += uint32(data[0]) << 8
	}

	return sum
}

// checksumFinish folds the carries back into the low 16 bits and complements the result.
func checksumFinish(sum uint32) uint16 {
	for sum > 0xffff {
		sum = sum&0xffff + sum>>16
	}

	return ^uint16(sum)
}

// pseudoHeaderSum is the running sum of the pseudo-header a TCP or UDP checksum covers (RFC 9293
// section 3.1 for IPv4, RFC 8200 section 8.1 for IPv6).
func pseudoHeaderSum(source netip.Addr, destination netip.Addr, protocol uint8, length uint32) uint32 {
	sum := addressSum(0, source)
	sum = addressSum(sum, destination)
	sum += uint32(protocol)
	sum += length >> 16
	sum += length & 0xffff

	return sum
}

// addressSum folds the address's bytes into the running checksum sum: four for IPv4, sixteen
// otherwise.
func addressSum(sum uint32, address netip.Addr) uint32 {
	if address.Is4() {
		addressBytes := address.As4()
		return checksumAdd(sum, addressBytes[:])
	}

	addressBytes := address.As16()
	return checksumAdd(sum, addressBytes[:])
}

// appendAddress appends the address's bytes to b: four for IPv4, sixteen otherwise.
func appendAddress(b []byte, address netip.Addr) []byte {
	if address.Is4() {
		addressBytes := address.As4()
		return append(b, addressBytes[:]...)
	}

	addressBytes := address.As16()
	return append(b, addressBytes[:]...)
}
