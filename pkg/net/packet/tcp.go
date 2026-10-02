package packet

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

const (
	tcpMinimumHeaderLength = 20
	tcpMaximumHeaderLength = 60
)

// TcpFlags are the control bits of a TCP header, in the positions they occupy in its thirteenth
// byte.
type TcpFlags uint8

const (
	TcpFlagFin TcpFlags = 1 << iota
	TcpFlagSyn
	TcpFlagRst
	TcpFlagPsh
	TcpFlagAck
	TcpFlagUrg
	TcpFlagEce
	TcpFlagCwr
)

// Has reports whether every flag in flags is set.
func (tcpFlags TcpFlags) Has(flags TcpFlags) bool {
	return tcpFlags&flags == flags
}

// Tcp is a TCP segment.
type Tcp struct {
	SourcePort      uint16
	DestinationPort uint16
	Sequence        uint32
	Acknowledgement uint32
	Flags           TcpFlags
	Window          uint16
	UrgentPointer   uint16
	// Options are the raw option bytes. Encoding pads them with zeros (end of option list) to a
	// multiple of four.
	Options []byte
	// Payload is the data the segment carries. Decoded options and payload alias the decoded data.
	Payload []byte
}

// ParseTcp decodes a TCP segment.
func ParseTcp(data []byte) (*Tcp, error) {
	if len(data) < tcpMinimumHeaderLength {
		return nil, fmt.Errorf("%w: tcp header truncated at %d bytes", ErrMalformed, len(data))
	}

	headerLength := int(data[12]>>4) * 4
	if headerLength < tcpMinimumHeaderLength {
		return nil, fmt.Errorf("%w: tcp data offset %d", ErrMalformed, headerLength)
	}
	if headerLength > len(data) {
		return nil, fmt.Errorf(
			"%w: tcp header length %d exceeds the %d bytes available", ErrMalformed, headerLength, len(data),
		)
	}

	return &Tcp{
		SourcePort:      binary.BigEndian.Uint16(data[0:2]),
		DestinationPort: binary.BigEndian.Uint16(data[2:4]),
		Sequence:        binary.BigEndian.Uint32(data[4:8]),
		Acknowledgement: binary.BigEndian.Uint32(data[8:12]),
		Flags:           TcpFlags(data[13]),
		Window:          binary.BigEndian.Uint16(data[14:16]),
		UrgentPointer:   binary.BigEndian.Uint16(data[18:20]),
		Options:         data[tcpMinimumHeaderLength:headerLength],
		Payload:         data[headerLength:],
	}, nil
}

// AppendTcp appends tcp encoded as a segment to b, with its checksum computed over the
// pseudo-header of source and destination, which must be of the same IP version. An IPv4-mapped
// IPv6 address is rejected, since no packet carries one; unmap it for an IPv4 checksum.
func AppendTcp(b []byte, tcp *Tcp, source netip.Addr, destination netip.Addr) ([]byte, error) {
	if tcp == nil {
		return nil, fmt.Errorf("%w: nil tcp", ErrInvalidArgument)
	}

	if !source.IsValid() || !destination.IsValid() || source.Is4() != destination.Is4() {
		return nil, fmt.Errorf(
			"%w: checksum addresses %v and %v are not of one ip version", ErrInvalidArgument, source, destination,
		)
	}

	// No packet on the wire carries an IPv4-mapped address: an IPv4 packet's pseudo-header holds
	// four-byte addresses, so a checksum over the mapped form would match nothing sent.
	if source.Is4In6() || destination.Is4In6() {
		return nil, fmt.Errorf(
			"%w: ipv4-mapped checksum address in %v and %v; unmap it", ErrInvalidArgument, source, destination,
		)
	}

	paddedOptionsLength := (len(tcp.Options) + 3) &^ 3
	headerLength := tcpMinimumHeaderLength + paddedOptionsLength
	if headerLength > tcpMaximumHeaderLength {
		return nil, fmt.Errorf("%w: %d bytes of tcp options", ErrInvalidArgument, len(tcp.Options))
	}

	start := len(b)
	b = binary.BigEndian.AppendUint16(b, tcp.SourcePort)
	b = binary.BigEndian.AppendUint16(b, tcp.DestinationPort)
	b = binary.BigEndian.AppendUint32(b, tcp.Sequence)
	b = binary.BigEndian.AppendUint32(b, tcp.Acknowledgement)
	b = append(b, byte(headerLength/4)<<4, byte(tcp.Flags)) //nolint:gosec // At most 60, checked above.
	b = binary.BigEndian.AppendUint16(b, tcp.Window)
	b = append(b, 0, 0) // checksum, filled in below
	b = binary.BigEndian.AppendUint16(b, tcp.UrgentPointer)
	b = append(b, tcp.Options...)
	b = append(b, make([]byte, paddedOptionsLength-len(tcp.Options))...)
	b = append(b, tcp.Payload...)

	segment := b[start:]
	// The segment is at most the 60-byte header and whatever payload the caller gave; a length
	// beyond 32 bits is not one any IP packet carries.
	sum := pseudoHeaderSum(source, destination, ProtocolTcp, uint32(len(segment))) //nolint:gosec // See above.
	binary.BigEndian.PutUint16(segment[16:], checksumFinish(checksumAdd(sum, segment)))

	return b, nil
}
