package net

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftNetErrors "github.com/altshiftab/utils_go/pkg/net/errors"
)

const (
	ProtocolIcmp  = 1
	ProtocolTcp   = 6
	ProtocolUdp   = 17
	ProtocolIcmp6 = 58
)

const Localhost = "localhost"

// IsLocalhost reports whether the hostname names the machine it is resolved on. RFC 6761 reserves
// "localhost" and everything under it for the purpose, so the subdomains count too.
func IsLocalhost(hostname string) bool {
	return strings.EqualFold(hostname, Localhost) ||
		strings.HasSuffix(strings.ToLower(hostname), "."+Localhost)
}

func SplitAddress(address string) (string, int, error) {
	ip, portString, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, &altshiftErrors.Error{
			Message: "An error occurred when splitting an address into host and port.",
			Cause:   err,
			Input:   address,
		}
	}

	port, err := strconv.Atoi(portString)
	if err != nil {
		return ip, 0, &altshiftErrors.Error{
			Message: "An error occurred when parsing an address port string as an integer.",
			Cause:   err,
			Input:   portString,
		}
	}

	return ip, port, nil
}

func GetIpVersion(ip *net.IP) int {
	if ip.To4() != nil {
		return 4
	} else if ip.To16() != nil {
		return 6
	} else {
		return 0
	}
}

// Calculate the last address in the network.
func lastAddress(network net.IPNet) net.IP {
	ip := network.IP.To16()
	if ip == nil {
		return nil
	}

	mask := network.Mask
	last := make(net.IP, len(ip))
	for i := range last {
		last[i] = ip[i] | ^mask[i]
	}
	return last
}

func ParseAddressNet(addressNet string) (*net.IPNet, error) {
	if addressNet == "" {
		return nil, nil
	}

	networkString := addressNet

	if ip := net.ParseIP(addressNet); ip != nil {
		var mask int
		switch ipVersion := GetIpVersion(&ip); ipVersion {
		case 4:
			mask = 32
		case 6:
			mask = 128
		default:
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %d", altshiftNetErrors.ErrUnexpectedIpVersion, ipVersion),
				ipVersion,
			)
		}

		networkString += fmt.Sprintf("/%d", mask)
	}

	_, network, err := net.ParseCIDR(networkString)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("net parse cidr: %w", err),
			networkString,
		)
	}

	return network, nil
}

func GetStartEndCidr(startIpAddress *net.IP, endIpAddress *net.IP, checkBoundary bool) (string, error) {
	if startIpAddress == nil || endIpAddress == nil {
		return "", nil
	}

	if (startIpAddress.To4() == nil) != (endIpAddress.To4() == nil) {
		return "", altshiftNetErrors.ErrIpVersionMismatch
	}

	startBytes := startIpAddress.To16()
	endBytes := endIpAddress.To16()
	if startBytes == nil || endBytes == nil {
		return "", altshiftErrors.NewWithTrace(nil_error.New("ip 16-byte representation"))
	}

	byteComparison := bytes.Compare(startBytes, endBytes)

	if byteComparison > 0 {
		return "", altshiftNetErrors.ErrStartAfterEnd
	}

	// Find the first byte where the two IP addresses differ
	maskLength := 0
	found := false
	for i := range startBytes {
		if startBytes[i] != endBytes[i] {
			// Calculate the mask length up to this point
			maskLength = i * 8
			diff := startBytes[i] ^ endBytes[i]
			// Count the number of leading zeros in the differing byte
			for j := 7; j >= 0; j-- {
				if diff&(1<<j) != 0 {
					maskLength += 8 - j - 1
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}

	if !found {
		// start and end are equal: emit a host route. The mask is built over the
		// 16-byte representation, so this is /128 here; IPNet.String renders an
		// IPv4 host route as /32.
		maskLength = len(startBytes) * 8
	}

	mask := net.CIDRMask(maskLength, len(startBytes)*8)
	network := net.IPNet{IP: startIpAddress.Mask(mask), Mask: mask}

	if checkBoundary && byteComparison != 0 {
		// Ensure start IP is network's base address and end IP is the last address in the network
		networkBase := network.IP
		networkLast := lastAddress(network)

		if !networkBase.Equal(*startIpAddress) || !networkLast.Equal(*endIpAddress) {
			return "", altshiftNetErrors.ErrNotOnSubnetBoundaries
		}
	}

	return network.String(), nil
}

// IntToIpv4 converts IPv4 number to net.IP.
func IntToIpv4(ipNum uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, ipNum)
	return ip
}

func NetworkFromTarget(target string) (*net.IPNet, error) {
	if target == "" {
		return nil, nil
	}

	if _, network, _ := net.ParseCIDR(target); network != nil {
		return network, nil
	}

	if ip := net.ParseIP(target); ip != nil {
		useIpv4 := ip.To4() != nil
		useIpv6 := ip.To16() != nil && ip.To4() == nil

		var targetCidrString string

		if useIpv6 {
			targetCidrString = target + "/128"
		} else if useIpv4 {
			targetCidrString = target + "/32"
		} else {
			return nil, altshiftErrors.NewWithTrace(altshiftNetErrors.ErrUndeterminableIpVersion, ip)
		}

		_, network, _ := net.ParseCIDR(targetCidrString)
		if network == nil {
			return nil, altshiftErrors.NewWithTrace(
				nil_error.NewWithInstance("ip net", "single target"), targetCidrString,
			)
		}

		return network, nil
	}

	return nil, altshiftErrors.NewWithTrace(altshiftNetErrors.ErrUndeterminableTargetFormat)
}
