package outbound_guard

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"slices"
	"testing"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

// errLookupFailed is what the tests' resolver fails with.
var errLookupFailed = errors.New("lookup failed")

func TestDeniedPrefixStringsReturnsCopy(t *testing.T) {
	t.Parallel()

	prefixes := DeniedPrefixStrings()
	if len(prefixes) == 0 {
		t.Fatal("DeniedPrefixStrings returned no prefixes")
	}

	original := prefixes[0]
	prefixes[0] = "8.8.8.0/24"

	if DeniedPrefixStrings()[0] != original {
		t.Fatal("DeniedPrefixStrings returned mutable shared backing storage")
	}
}

func TestDeniedPrefixesReturnsCopy(t *testing.T) {
	t.Parallel()

	prefixes := DeniedPrefixes()
	if len(prefixes) == 0 {
		t.Fatal("DeniedPrefixes returned no prefixes")
	}

	original := prefixes[0]
	prefixes[0] = netip.MustParsePrefix("8.8.8.0/24")

	if DeniedPrefixes()[0] != original {
		t.Fatal("DeniedPrefixes returned mutable shared backing storage")
	}
}

func TestAppendDeniedPrefixStrings(t *testing.T) {
	t.Parallel()

	got := AppendDeniedPrefixStrings([]string{"example.com", "10.0.0.0/8"})

	for _, value := range []string{"example.com", "10.0.0.0/8", "127.0.0.0/8", "64:ff9b:1::/48", "fc00::/7"} {
		if !slices.Contains(got, value) {
			t.Fatalf("AppendDeniedPrefixStrings missing %q in %#v", value, got)
		}
	}

	if countValues(got, "10.0.0.0/8") != 1 {
		t.Fatalf("AppendDeniedPrefixStrings duplicated 10.0.0.0/8 in %#v", got)
	}
}

func TestIsPublicIP(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		ip       string
		expected bool
	}{
		{name: "Public IPv4", ip: "8.8.8.8", expected: true},
		{name: "Public IPv6", ip: "2606:4700:4700::1111", expected: true},
		{name: "RFC1918 IPv4", ip: "10.0.0.1", expected: false},
		{name: "Loopback IPv4", ip: "127.0.0.1", expected: false},
		{name: "Metadata IPv4", ip: "169.254.169.254", expected: false},
		{name: "IPv4 mapped loopback", ip: "::ffff:127.0.0.1", expected: false},
		{name: "IPv6 ULA", ip: "fd00::1", expected: false},
		{name: "IPv6 link-local", ip: "fe80::1", expected: false},
		{name: "IPv6 unspecified", ip: "::", expected: false},
		{name: "IPv6 local-use translation", ip: "64:ff9b:1::1", expected: false},
		{name: "IPv6 dummy prefix", ip: "100:0:0:1::1", expected: false},
		{name: "IPv6 benchmarking", ip: "2001:2::1", expected: false},
		{name: "Documentation IPv6", ip: "2001:db8::1", expected: false},
		{name: "Documentation IPv6 3fff", ip: "3fff::1", expected: false},
		{name: "IPv6 SRv6 SID", ip: "5f00::1", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if actual := IsPublicIP(net.ParseIP(tc.ip)); actual != tc.expected {
				t.Fatalf("IsPublicIP(%q) = %v; want %v", tc.ip, actual, tc.expected)
			}
		})
	}
}

func TestIsPublic(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		addr     string
		expected bool
	}{
		{name: "Public IPv4", addr: "8.8.8.8", expected: true},
		{name: "Public IPv6", addr: "2606:4700:4700::1111", expected: true},
		{name: "Loopback IPv6", addr: "::1", expected: false},
		{name: "IPv4 mapped loopback", addr: "::ffff:127.0.0.1", expected: false},
		{name: "Loopback IPv6 with a zone", addr: "::1%lo", expected: false},
		{name: "IPv6 link-local with a zone", addr: "fe80::1%eth0", expected: false},
		{name: "IPv6 ULA with a zone", addr: "fd00::1%eth0", expected: false},
		{name: "IPv4 mapped loopback with a zone", addr: "::ffff:127.0.0.1%lo", expected: false},
		{name: "Public IPv6 with a zone", addr: "2606:4700:4700::1111%eth0", expected: false},
		{name: "The zero value", addr: "", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var addr netip.Addr
			if tc.addr != "" {
				addr = netip.MustParseAddr(tc.addr)
			}
			if actual := IsPublic(addr); actual != tc.expected {
				t.Fatalf("IsPublic(%q) = %v; want %v", tc.addr, actual, tc.expected)
			}
		})
	}
}

func TestCheckWithLookup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	lookupErr := errLookupFailed
	lookup := func(ctx context.Context, network string, host string) ([]net.IP, error) {
		switch host {
		case "public.example":
			return []net.IP{net.ParseIP("8.8.8.8")}, nil
		case "private.example":
			return []net.IP{net.ParseIP("10.0.0.1")}, nil
		case "mixed.example":
			return []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("127.0.0.1")}, nil
		case "local-use-translation.example":
			return []net.IP{net.ParseIP("64:ff9b:1::1")}, nil
		case "empty.example":
			return nil, nil
		default:
			return nil, lookupErr
		}
	}

	testCases := []struct {
		name       string
		target     string
		wantHost   string
		wantDirect bool
		wantErr    error
		// wantEmpty is an empty_error, which is a type rather than a sentinel.
		wantEmpty bool
	}{
		{name: "Public IPv4", target: "8.8.8.8", wantHost: "8.8.8.8", wantDirect: true},
		{name: "Public IPv4 with whitespace", target: " 8.8.8.8 ", wantHost: "8.8.8.8", wantDirect: true},
		{name: "Public IPv6", target: "2606:4700:4700::1111", wantHost: "2606:4700:4700::1111", wantDirect: true},
		{name: "Public URL", target: "https://public.example:443/path", wantHost: "public.example"},
		{name: "Public host port", target: "public.example:443", wantHost: "public.example"},
		{name: "Public bracketed IPv6 host port", target: "[2606:4700:4700::1111]:443", wantHost: "2606:4700:4700::1111", wantDirect: true},
		{name: "Private IPv4", target: "10.0.0.1", wantErr: ErrNotPublic},
		{name: "Private IPv6 ULA", target: "fd00::1", wantErr: ErrNotPublic},
		{name: "Loopback URL", target: "http://127.0.0.1:8080/admin", wantHost: "127.0.0.1", wantErr: ErrNotPublic},
		{name: "Private hostname", target: "private.example", wantHost: "private.example", wantErr: ErrNotPublic},
		{name: "Mixed public and private hostname", target: "mixed.example", wantHost: "mixed.example", wantErr: ErrNotPublic},
		{name: "Special-purpose IPv6 hostname", target: "local-use-translation.example", wantHost: "local-use-translation.example", wantErr: ErrNotPublic},
		{name: "Unknown hostname", target: "unknown.example", wantHost: "unknown.example", wantErr: ErrDNSLookupFailed},
		{name: "Empty DNS answer", target: "empty.example", wantHost: "empty.example", wantErr: ErrNoIPAddresses},
		{name: "Empty string", target: "", wantEmpty: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			checked, err := CheckWithLookup(ctx, tc.target, lookup)
			if tc.wantEmpty {
				if _, ok := errors.AsType[*empty_error.Error](err); !ok {
					t.Fatalf("CheckWithLookup(%q) error = %v; want an empty error", tc.target, err)
				}
				return
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("CheckWithLookup(%q) error = %v; want %v", tc.target, err, tc.wantErr)
				}
				if checked != nil && tc.wantHost != "" && checked.Host != tc.wantHost {
					t.Fatalf("CheckWithLookup(%q) host = %q; want %q", tc.target, checked.Host, tc.wantHost)
				}
				return
			}
			if err != nil {
				t.Fatalf("CheckWithLookup(%q) returned error: %v", tc.target, err)
			}
			if checked == nil {
				t.Fatal("CheckWithLookup returned nil Target")
			}
			if checked.Host != tc.wantHost {
				t.Fatalf("CheckWithLookup(%q) host = %q; want %q", tc.target, checked.Host, tc.wantHost)
			}
			if checked.DirectIP != tc.wantDirect {
				t.Fatalf("CheckWithLookup(%q) DirectIP = %v; want %v", tc.target, checked.DirectIP, tc.wantDirect)
			}
			if len(checked.IPs) == 0 {
				t.Fatal("CheckWithLookup returned no IPs")
			}
			if !IsPublicTargetWithLookup(ctx, tc.target, lookup) {
				t.Fatalf("IsPublicTargetWithLookup(%q) = false; want true", tc.target)
			}
		})
	}
}

func TestIsPublicTargetWithLookupRejectsUnsafeTargets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	lookup := func(ctx context.Context, network string, host string) ([]net.IP, error) {
		switch host {
		case "public.example":
			return []net.IP{net.ParseIP("8.8.8.8")}, nil
		case "private.example":
			return []net.IP{net.ParseIP("10.0.0.1")}, nil
		case "mixed.example":
			return []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("127.0.0.1")}, nil
		case "dummy-ipv6.example":
			return []net.IP{net.ParseIP("100:0:0:1::1")}, nil
		default:
			return nil, errLookupFailed
		}
	}

	testCases := []struct {
		name   string
		target string
	}{
		{name: "Private IPv4 10/8", target: "10.0.0.1"},
		{name: "Private IPv4 172.16/12", target: "172.16.0.1"},
		{name: "Private IPv4 192.168/16", target: "192.168.1.1"},
		{name: "Private IPv6 ULA", target: "fd00::1"},
		{name: "Loopback IPv4", target: "127.0.0.1"},
		{name: "Loopback IPv6", target: "::1"},
		{name: "Link-local IPv4", target: "169.254.1.1"},
		{name: "Link-local IPv6", target: "fe80::1"},
		{name: "CGNAT IPv4", target: "100.64.0.1"},
		{name: "TEST-NET-1 IPv4", target: "192.0.2.1"},
		{name: "TEST-NET-2 IPv4", target: "198.51.100.1"},
		{name: "TEST-NET-3 IPv4", target: "203.0.113.1"},
		{name: "Documentation IPv6", target: "2001:db8::1"},
		{name: "Local-use translation IPv6", target: "64:ff9b:1::1"},
		{name: "Dummy IPv6 prefix", target: "100:0:0:1::1"},
		{name: "Benchmarking IPv6", target: "2001:2::1"},
		{name: "Documentation IPv6 3fff", target: "3fff::1"},
		{name: "SRv6 SID IPv6", target: "5f00::1"},
		{name: "Unspecified IPv4", target: "0.0.0.0"},
		{name: "Unspecified IPv6", target: "::"},
		{name: "Private URL", target: "http://127.0.0.1:8080/admin"},
		{name: "Private hostname", target: "private.example"},
		{name: "Mixed public and private hostname", target: "mixed.example"},
		{name: "Special-purpose IPv6 hostname", target: "dummy-ipv6.example"},
		{name: "Invalid hostname", target: "invalid-hostname-that-does-not-exist-blah"},
		{name: "Empty string", target: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if IsPublicTargetWithLookup(ctx, tc.target, lookup) {
				t.Fatalf("IsPublicTargetWithLookup(%q) = true; want false", tc.target)
			}
		})
	}
}

func TestHostForTarget(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		target string
		want   string
	}{
		{target: "https://public.example:443/path", want: "public.example"},
		{target: "public.example:443", want: "public.example"},
		{target: "[2606:4700:4700::1111]:443", want: "2606:4700:4700::1111"},
		{target: "2606:4700:4700::1111", want: "2606:4700:4700::1111"},
		{target: "[::1]", want: "::1"},
		{target: "https://[::1]:8443/admin", want: "::1"},
		{target: " public.example ", want: "public.example"},
	}

	for _, tc := range testCases {
		t.Run(tc.target, func(t *testing.T) {
			t.Parallel()

			if got := HostForTarget(tc.target); got != tc.want {
				t.Fatalf("HostForTarget(%q) = %q; want %q", tc.target, got, tc.want)
			}
		})
	}
}

func TestPrefixOverlapsDenied(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		cidr     string
		expected bool
	}{
		{name: "Public IPv4", cidr: "8.8.8.0/24", expected: false},
		{name: "Public IPv6", cidr: "2606:4700:4700::/48", expected: false},
		{name: "Private IPv4", cidr: "10.42.0.0/24", expected: true},
		{name: "Metadata IPv4", cidr: "169.254.169.0/24", expected: true},
		{name: "IPv6 ULA", cidr: "fd00::/120", expected: true},
		{name: "Local-use translation IPv6", cidr: "64:ff9b:1::/120", expected: true},
		{name: "Dummy IPv6 prefix", cidr: "100:0:0:1::/120", expected: true},
		{name: "Benchmarking IPv6", cidr: "2001:2::/64", expected: true},
		{name: "Documentation IPv6 3fff", cidr: "3fff::/32", expected: true},
		{name: "SRv6 SID IPv6", cidr: "5f00::/32", expected: true},
		{name: "Broad range covering denied IPv4", cidr: "8.0.0.0/6", expected: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, ipNet, err := net.ParseCIDR(tc.cidr)
			if err != nil {
				t.Fatalf("net.ParseCIDR(%q) returned error: %v", tc.cidr, err)
			}

			actual, err := PrefixOverlapsDenied(ipNet)
			if err != nil {
				t.Fatalf("PrefixOverlapsDenied(%q) returned error: %v", tc.cidr, err)
			}
			if actual != tc.expected {
				t.Fatalf("PrefixOverlapsDenied(%q) = %v; want %v", tc.cidr, actual, tc.expected)
			}
		})
	}
}

func TestPrefixFromIPNet(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		cidr string
		want string
	}{
		{cidr: "8.8.8.0/24", want: "8.8.8.0/24"},
		{cidr: "2606:4700:4700::/48", want: "2606:4700:4700::/48"},
	}

	for _, tc := range testCases {
		t.Run(tc.cidr, func(t *testing.T) {
			t.Parallel()

			_, ipNet, err := net.ParseCIDR(tc.cidr)
			if err != nil {
				t.Fatalf("net.ParseCIDR returned error: %v", err)
			}

			prefix, err := PrefixFromIPNet(ipNet)
			if err != nil {
				t.Fatalf("PrefixFromIPNet returned error: %v", err)
			}
			if prefix.String() != tc.want {
				t.Fatalf("PrefixFromIPNet = %s; want %s", prefix.String(), tc.want)
			}
		})
	}
}

func countValues(values []string, needle string) int {
	count := 0
	for _, value := range values {
		if value == needle {
			count++
		}
	}
	return count
}

func TestDialControl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		address string
		wantErr bool
	}{
		{name: "a public IPv4 address", address: "93.184.215.14:443"},
		{name: "a public IPv6 address", address: "[2606:2800:21f:cb07:6820:80da:af6b:8b2c]:443"},
		{name: "loopback", address: "127.0.0.1:443", wantErr: true},
		{name: "loopback, IPv6", address: "[::1]:443", wantErr: true},
		{name: "loopback, IPv4 in IPv6", address: "[::ffff:127.0.0.1]:80", wantErr: true},
		{name: "a private network", address: "10.1.2.3:80", wantErr: true},
		{name: "the metadata server", address: "169.254.169.254:80", wantErr: true},
		{name: "a unique local IPv6 address", address: "[fd00::1]:80", wantErr: true},
		{name: "loopback, IPv6, with a zone", address: "[::1%lo]:80", wantErr: true},
		{name: "link-local, with a zone", address: "[fe80::1%eth0]:80", wantErr: true},
		{name: "a name, unresolved", address: "localhost:80", wantErr: true},
		{name: "no port", address: "93.184.215.14", wantErr: true},
	}

	for _, testCase := range testCases {
		err := DialControl("tcp", testCase.address, nil)
		if (err != nil) != testCase.wantErr {
			t.Errorf("%s: %v", testCase.name, err)
		}
		if err != nil && !errors.Is(err, ErrNotPublic) {
			t.Errorf("%s: %v is not ErrNotPublic", testCase.name, err)
		}
	}
}
