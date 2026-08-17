package flow_tuple

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // Community ID v1 mandates SHA-1 (non-cryptographic flow fingerprint)
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net"

	altshiftNet "github.com/altshiftab/utils_go/pkg/net"
)

var icmpV4PortEquivalents = map[uint8]uint8{
	8:  0,  // Echo Request -> Echo Reply
	0:  8,  // Echo Reply -> Echo Request
	13: 14, // Timestamp Request -> Timestamp Reply
	14: 13, // Timestamp Reply -> Timestamp Request
	15: 16, // Information Request -> Information Reply
	16: 15, // Information Reply -> Information Request
	10: 9,  // Router Solicitation -> Router Advertisement
	9:  10, // Router Advertisement -> Router Solicitation
	17: 18, // Address Mask Request -> Address Mask Reply
	18: 17, // Address Mask Reply -> Address Mask Request
}

var icmpV6PortEquivalents = map[uint8]uint8{
	128: 129, // ICMPv6 Echo Request -> ICMPv6 Echo Reply
	129: 128, // ICMPv6 Echo Reply -> ICMPv6 Echo Request
	133: 134, // ICMPv6 Router Solicitation -> ICMPv6 Router Advertisement
	134: 133, // ICMPv6 Router Advertisement -> ICMPv6 Router Solicitation
	135: 136, // ICMPv6 Neighbor Solicitation -> ICMPv6 Neighbor Advertisement
	136: 135, // ICMPv6 Neighbor Advertisement -> ICMPv6 Neighbor Solicitation
	130: 131, // ICMPv6 MLDv1 Multicast Listener Query Message -> ICMPv6 MLDv1 Multicast Listener Report Message
	131: 130, // ICMPv6 MLDv1 Multicast Listener Report Message -> ICMPv6 MLDv1 Multicast Listener Query Message
	144: 145, // ICMPv6 ICMP Node Information Query -> ICMPv6 ICMP Node Information Response
	145: 144, // ICMPv6 ICMP Node Information Response -> ICMPv6 ICMP Node Information Query
}

type Tuple struct {
	SourceIp        net.IP
	DestinationIp   net.IP
	SourcePort      uint16
	DestinationPort uint16
	Protocol        uint8
	IsOneWay        bool
}

// Hash computes the Community ID v1 hash for the flow tuple.
func (flowTuple *Tuple) Hash() string {
	const (
		maxParameterSizeIPv6 = 40
		maxParameterSizeIPv4 = 16
	)
	buffer := make([]byte, 2, maxParameterSizeIPv4)

	// The seed is the default value i.e. `0`.
	binary.BigEndian.PutUint16(buffer, 0)

	if v4SrcAddress := flowTuple.SourceIp.To4(); v4SrcAddress != nil {
		buffer = append(buffer, v4SrcAddress...)
	} else if v6SrcAddress := flowTuple.SourceIp.To16(); v6SrcAddress != nil {
		// As we are now dealing with IPv6, grow the buffer once to fit both addresses.
		buffer = append(make([]byte, 0, maxParameterSizeIPv6), buffer...)
		buffer = append(buffer, v6SrcAddress...)
	}

	if v4DstAddress := flowTuple.DestinationIp.To4(); v4DstAddress != nil {
		buffer = append(buffer, v4DstAddress...)
	} else if v6DstAddress := flowTuple.DestinationIp.To16(); v6DstAddress != nil {
		buffer = append(buffer, v6DstAddress...)
	}

	buffer = append(buffer, flowTuple.Protocol, 0)
	buffer = binary.BigEndian.AppendUint16(buffer, flowTuple.SourcePort)
	buffer = binary.BigEndian.AppendUint16(buffer, flowTuple.DestinationPort)

	h := sha1.New() //nolint:gosec // Community ID v1 mandates SHA-1 (non-cryptographic flow fingerprint)
	h.Write(buffer)

	return fmt.Sprintf("1:%s", base64.StdEncoding.EncodeToString(h.Sum(nil)))
}

// getIcmpV4PortEquivalents returns ICMPv4 codes mapped back to pseudo port
// numbers, as well as a bool indicating whether a communication is one-way.
func getIcmpV4PortEquivalents(p1, p2 uint8) (uint16, uint16, bool) {
	if val, ok := icmpV4PortEquivalents[p1]; ok {
		return uint16(p1), uint16(val), false
	}
	return uint16(p1), uint16(p2), true
}

// getIcmpv6PortEquivalents returns ICMPv6 codes mapped back to pseudo port
// numbers, as well as a bool indicating whether a communication is one-way.
func getIcmpv6PortEquivalents(p1, p2 uint8) (uint16, uint16, bool) {
	if val, ok := icmpV6PortEquivalents[p1]; ok {
		return uint16(p1), uint16(val), false
	}
	return uint16(p1), uint16(p2), true
}

func isOrdered(addr1, addr2 []byte, port1, port2 uint16) bool {
	addrCmp := bytes.Compare(addr1, addr2)
	return addrCmp < 0 || (addrCmp == 0 && port1 <= port2)
}

func New(
	sourceIp net.IP,
	destinationIp net.IP,
	sourcePort uint16,
	destinationPort uint16,
	protocol uint8,
) *Tuple {
	var oneWay bool

	switch protocol {
	case altshiftNet.ProtocolIcmp:
		sourcePort, destinationPort, oneWay = getIcmpV4PortEquivalents(uint8(sourcePort), uint8(destinationPort)) //nolint:gosec // ICMP type/code is 8-bit
	case altshiftNet.ProtocolIcmp6:
		sourcePort, destinationPort, oneWay = getIcmpv6PortEquivalents(uint8(sourcePort), uint8(destinationPort)) //nolint:gosec // ICMP type/code is 8-bit
	}

	if !oneWay && !isOrdered(sourceIp, destinationIp, sourcePort, destinationPort) {
		sourceIp, destinationIp = destinationIp, sourceIp
		sourcePort, destinationPort = destinationPort, sourcePort
	}

	return &Tuple{
		SourceIp:        sourceIp,
		DestinationIp:   destinationIp,
		SourcePort:      sourcePort,
		DestinationPort: destinationPort,
		Protocol:        protocol,
		IsOneWay:        oneWay,
	}
}
