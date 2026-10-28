package outbound_guard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"syscall"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

var (
	ErrDNSLookupFailed = errors.New("DNS lookup failed")
	ErrNoIPAddresses   = errors.New("DNS lookup returned no IP addresses")
	// ErrNotPublic is a target, or a connection, to an address outside the public internet.
	ErrNotPublic = errors.New("not a public address")
	// ErrInvalidNetwork is an IP network that is no IPv4 or IPv6 prefix.
	ErrInvalidNetwork = errors.New("invalid IP network")
)

var deniedPrefixStrings = []string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.88.99.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"::/128",
	"::1/128",
	"64:ff9b::/96",
	"64:ff9b:1::/48",
	"100::/64",
	"100:0:0:1::/64",
	"2001::/32",
	"2001:10::/28",
	"2001:20::/28",
	"2001:2::/48",
	"2001:db8::/32",
	"2002::/16",
	"3fff::/20",
	"5f00::/16",
	"fc00::/7",
	"fe80::/10",
	"ff00::/8",
}

var deniedPrefixes = mustParsePrefixes(deniedPrefixStrings)

// LookupIPFunc resolves host into IP addresses.
type LookupIPFunc func(ctx context.Context, network string, host string) ([]net.IP, error)

// Target describes the normalized target data used by the checks.
type Target struct {
	Target   string
	Host     string
	IPs      []net.IP
	DirectIP bool
}

// DeniedPrefixStrings returns the CIDR deny list as strings.
func DeniedPrefixStrings() []string {
	return append([]string(nil), deniedPrefixStrings...)
}

// DeniedPrefixes returns the parsed CIDR deny list.
func DeniedPrefixes() []netip.Prefix {
	return append([]netip.Prefix(nil), deniedPrefixes...)
}

// AppendDeniedPrefixStrings appends the shared CIDR deny list to values,
// preserving existing values and avoiding duplicates.
func AppendDeniedPrefixStrings(values []string) []string {
	return appendUniqueStrings(values, deniedPrefixStrings...)
}

// Check parses a target and checks that it is public, resolving it with the default resolver.
func Check(ctx context.Context, target string) (*Target, error) {
	return CheckWithLookup(ctx, target, net.DefaultResolver.LookupIP)
}

// CheckWithLookup parses a target and checks that it is public, resolving it with lookupIP.
//
// target may be a raw IP, hostname, host:port, or URL. Hostnames are resolved
// and every resulting IP must be outside the shared deny list. DNS errors and
// empty DNS answers fail closed.
func CheckWithLookup(ctx context.Context, target string, lookupIP LookupIPFunc) (*Target, error) {
	checked, err := ResolveTarget(ctx, target, lookupIP)
	if err != nil {
		return checked, err
	}

	for _, checkIP := range checked.IPs {
		if !IsPublicIP(checkIP) {
			return checked, fmt.Errorf("%w: target=%q host=%q ip=%s", ErrNotPublic, target, checked.Host, checkIP.String())
		}
	}

	return checked, nil
}

// IsPublicTarget reports whether target resolves to public addresses alone.
func IsPublicTarget(ctx context.Context, target string) bool {
	_, err := Check(ctx, target)
	return err == nil
}

// IsPublicTargetWithLookup reports whether target resolves, with lookupIP, to public addresses
// alone.
func IsPublicTargetWithLookup(ctx context.Context, target string, lookupIP LookupIPFunc) bool {
	_, err := CheckWithLookup(ctx, target, lookupIP)
	return err == nil
}

// IsPublicIP reports whether ip is outside the deny list.
func IsPublicIP(ip net.IP) bool {
	addr, ok := addrFromIP(ip)
	if !ok {
		return false
	}
	return IsPublic(addr)
}

// IsPublic reports whether addr is outside the deny list: not loopback, private, link-local, cloud
// metadata, reserved, documentation, multicast or any other range the public internet does not route.
func IsPublic(addr netip.Addr) bool {
	addr = normalizeAddr(addr)
	if !addr.IsValid() {
		return false
	}

	for _, denied := range deniedPrefixes {
		if sameFamily(addr, denied.Addr()) && denied.Contains(addr) {
			return false
		}
	}

	return true
}

// PrefixOverlapsDenied reports whether ipNet overlaps any denied prefix.
func PrefixOverlapsDenied(ipNet *net.IPNet) (bool, error) {
	prefix, err := PrefixFromIPNet(ipNet)
	if err != nil {
		return false, err
	}

	for _, denied := range deniedPrefixes {
		if sameFamily(prefix.Addr(), denied.Addr()) && prefix.Overlaps(denied) {
			return true, nil
		}
	}

	return false, nil
}

// PrefixFromIPNet converts ipNet into a normalized netip.Prefix.
func PrefixFromIPNet(ipNet *net.IPNet) (netip.Prefix, error) {
	if ipNet == nil {
		return netip.Prefix{}, nil_error.New("IP network")
	}

	ones, bits := ipNet.Mask.Size()
	if ones < 0 {
		return netip.Prefix{}, fmt.Errorf("%w: the mask is not canonical", ErrInvalidNetwork)
	}

	var (
		addr netip.Addr
		ok   bool
	)

	switch bits {
	case 32:
		ip4 := ipNet.IP.To4()
		if ip4 == nil {
			return netip.Prefix{}, fmt.Errorf("%w: %s is no IPv4 address", ErrInvalidNetwork, ipNet.IP.String())
		}
		addr, ok = netip.AddrFromSlice(ip4)
	case 128:
		ip16 := ipNet.IP.To16()
		if ip16 == nil || ipNet.IP.To4() != nil {
			return netip.Prefix{}, fmt.Errorf("%w: %s is no IPv6 address", ErrInvalidNetwork, ipNet.IP.String())
		}
		addr, ok = netip.AddrFromSlice(ip16)
	default:
		return netip.Prefix{}, fmt.Errorf("%w: a mask of %d bits", ErrInvalidNetwork, bits)
	}

	if !ok {
		return netip.Prefix{}, fmt.Errorf("%w: %s", ErrInvalidNetwork, ipNet.IP.String())
	}

	return netip.PrefixFrom(normalizeAddr(addr), ones).Masked(), nil
}

// ResolveTarget parses target and returns the IPs the checks are made of.
func ResolveTarget(ctx context.Context, target string, lookupIP LookupIPFunc) (*Target, error) {
	if strings.TrimSpace(target) == "" {
		return &Target{Target: target}, empty_error.New("target")
	}
	if lookupIP == nil {
		return &Target{Target: target}, nil_error.New("lookup IP function")
	}

	host := HostForTarget(target)
	checked := &Target{
		Target: target,
		Host:   host,
	}
	if strings.TrimSpace(host) == "" {
		return checked, empty_error.New("host")
	}

	if ip := net.ParseIP(host); ip != nil {
		checked.IPs = []net.IP{ip}
		checked.DirectIP = true
		return checked, nil
	}

	resolvedIPs, err := lookupIP(ctx, "ip", host)
	if err != nil {
		return checked, fmt.Errorf("%w: target=%q host=%q: %w", ErrDNSLookupFailed, target, host, err)
	}
	if len(resolvedIPs) == 0 {
		return checked, fmt.Errorf("%w: target=%q host=%q", ErrNoIPAddresses, target, host)
	}

	checked.IPs = append([]net.IP(nil), resolvedIPs...)
	return checked, nil
}

// HostForTarget extracts the hostname or IP portion from a raw target.
func HostForTarget(target string) string {
	target = strings.TrimSpace(target)

	if parsedURL, err := url.Parse(target); err == nil && parsedURL.Scheme != "" && parsedURL.Host != "" {
		return parsedURL.Hostname()
	}

	if host, _, err := net.SplitHostPort(target); err == nil {
		return strings.Trim(host, "[]")
	}

	return strings.Trim(target, "[]")
}

// DialControl is a net.Dialer's Control that refuses a connection to an address that is not
// public. It is checked on the address the connection is made to, after any resolution, so that a
// redirect, or a name resolving to an internal address only when dialled, is caught where a check of
// the name beforehand would not be; a dialer using it must not go through a proxy, or the address
// checked is the proxy's.
func DialControl(_ string, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: %q is no host and port: %w", ErrNotPublic, address, err)
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || !IsPublic(addr) {
		return fmt.Errorf("%w: %s", ErrNotPublic, host)
	}
	return nil
}

func appendUniqueStrings(values []string, additional ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(additional))
	for _, value := range values {
		seen[value] = struct{}{}
	}

	for _, value := range additional {
		if _, ok := seen[value]; ok {
			continue
		}
		values = append(values, value)
		seen[value] = struct{}{}
	}

	return values
}

func addrFromIP(ip net.IP) (netip.Addr, bool) {
	if ip == nil {
		return netip.Addr{}, false
	}

	if ip4 := ip.To4(); ip4 != nil {
		addr, ok := netip.AddrFromSlice(ip4)
		return addr, ok
	}

	ip16 := ip.To16()
	if ip16 == nil {
		return netip.Addr{}, false
	}

	addr, ok := netip.AddrFromSlice(ip16)
	if !ok {
		return netip.Addr{}, false
	}

	return normalizeAddr(addr), true
}

func normalizeAddr(addr netip.Addr) netip.Addr {
	if addr.Is4In6() {
		return addr.Unmap()
	}
	return addr
}

func sameFamily(a netip.Addr, b netip.Addr) bool {
	return a.Is4() == b.Is4()
}

func mustParsePrefixes(values []string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			panic(err)
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes
}
