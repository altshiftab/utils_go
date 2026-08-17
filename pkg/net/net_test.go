package net

import (
	"errors"
	"net"
	"testing"

	motmedelNetErrors "github.com/altshiftab/utils_go/pkg/net/errors"
)

func TestSplitAddress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		address  string
		wantHost string
		wantPort int
		wantErr  bool
	}{
		{name: "ipv4 with port", address: "1.2.3.4:80", wantHost: "1.2.3.4", wantPort: 80},
		{name: "ipv6 with port", address: "[::1]:443", wantHost: "::1", wantPort: 443},
		{name: "hostname with port", address: "example.com:8080", wantHost: "example.com", wantPort: 8080},
		{name: "missing port", address: "1.2.3.4", wantErr: true},
		{name: "empty", address: "", wantErr: true},
		{name: "non-numeric port keeps host", address: "1.2.3.4:abc", wantHost: "1.2.3.4", wantPort: 0, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			host, port, err := SplitAddress(testCase.address)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if host != testCase.wantHost {
				t.Fatalf("host: expected %q, got %q", testCase.wantHost, host)
			}
			if port != testCase.wantPort {
				t.Fatalf("port: expected %d, got %d", testCase.wantPort, port)
			}
		})
	}
}

func TestGetIpVersion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		ip   net.IP
		want int
	}{
		{name: "ipv4", ip: net.ParseIP("1.2.3.4"), want: 4},
		{name: "ipv4 in dotted form", ip: net.IPv4(10, 0, 0, 1), want: 4},
		{name: "ipv6", ip: net.ParseIP("2001:db8::1"), want: 6},
		{name: "ipv6 loopback", ip: net.ParseIP("::1"), want: 6},
		{name: "invalid length", ip: net.IP{1, 2, 3}, want: 0},
		{name: "nil ip", ip: nil, want: 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ip := testCase.ip
			if got := GetIpVersion(&ip); got != testCase.want {
				t.Fatalf("expected %d, got %d", testCase.want, got)
			}
		})
	}
}

func TestParseAddressNet(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		input   string
		wantNil bool
		want    string
		wantErr bool
	}{
		{name: "empty returns nil", input: "", wantNil: true},
		{name: "bare ipv4 gets /32", input: "1.2.3.4", want: "1.2.3.4/32"},
		{name: "bare ipv6 gets /128", input: "::1", want: "::1/128"},
		{name: "ipv4 cidr", input: "10.0.0.0/8", want: "10.0.0.0/8"},
		{name: "ipv6 cidr", input: "2001:db8::/32", want: "2001:db8::/32"},
		{name: "cidr with host bits normalized", input: "10.1.2.3/8", want: "10.0.0.0/8"},
		{name: "garbage", input: "not-an-ip", wantErr: true},
		{name: "bad mask", input: "10.0.0.0/99", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseAddressNet(testCase.input)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if testCase.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil network, got nil")
			}
			if got.String() != testCase.want {
				t.Fatalf("expected %q, got %q", testCase.want, got.String())
			}
		})
	}
}

func TestGetStartEndCidr(t *testing.T) {
	t.Parallel()

	mk := func(s string) *net.IP {
		ip := net.ParseIP(s)
		return &ip
	}

	t.Run("nil start returns empty", func(t *testing.T) {
		t.Parallel()

		got, err := GetStartEndCidr(nil, mk("1.2.3.4"), true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})

	t.Run("nil end returns empty", func(t *testing.T) {
		t.Parallel()

		got, err := GetStartEndCidr(mk("1.2.3.4"), nil, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})

	t.Run("version mismatch", func(t *testing.T) {
		t.Parallel()

		_, err := GetStartEndCidr(mk("1.2.3.4"), mk("::1"), true)
		if !errors.Is(err, motmedelNetErrors.ErrIpVersionMismatch) {
			t.Fatalf("expected ErrIpVersionMismatch, got %v", err)
		}
	})

	t.Run("start after end", func(t *testing.T) {
		t.Parallel()

		_, err := GetStartEndCidr(mk("192.168.1.10"), mk("192.168.1.1"), true)
		if !errors.Is(err, motmedelNetErrors.ErrStartAfterEnd) {
			t.Fatalf("expected ErrStartAfterEnd, got %v", err)
		}
	})

	t.Run("ipv4 exact subnet boundary", func(t *testing.T) {
		t.Parallel()

		got, err := GetStartEndCidr(mk("192.168.1.0"), mk("192.168.1.255"), true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "192.168.1.0/24" {
			t.Fatalf("expected 192.168.1.0/24, got %q", got)
		}
	})

	t.Run("ipv4 not on boundary with check", func(t *testing.T) {
		t.Parallel()

		_, err := GetStartEndCidr(mk("192.168.1.1"), mk("192.168.1.254"), true)
		if !errors.Is(err, motmedelNetErrors.ErrNotOnSubnetBoundaries) {
			t.Fatalf("expected ErrNotOnSubnetBoundaries, got %v", err)
		}
	})

	t.Run("ipv4 not on boundary without check", func(t *testing.T) {
		t.Parallel()

		got, err := GetStartEndCidr(mk("192.168.1.1"), mk("192.168.1.254"), false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "192.168.1.0/24" {
			t.Fatalf("expected 192.168.1.0/24, got %q", got)
		}
	})

	t.Run("ipv6 exact boundary", func(t *testing.T) {
		t.Parallel()

		got, err := GetStartEndCidr(mk("2001:db8::"), mk("2001:db8::ffff"), true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "2001:db8::/112" {
			t.Fatalf("expected 2001:db8::/112, got %q", got)
		}
	})

	t.Run("ipv6 equal start and end", func(t *testing.T) {
		t.Parallel()

		got, err := GetStartEndCidr(mk("2001:db8::"), mk("2001:db8::"), true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "2001:db8::/128" {
			t.Fatalf("expected 2001:db8::/128, got %q", got)
		}
	})

	t.Run("ipv4 equal start and end", func(t *testing.T) {
		t.Parallel()

		got, err := GetStartEndCidr(mk("192.168.1.5"), mk("192.168.1.5"), true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "192.168.1.5/32" {
			t.Fatalf("expected 192.168.1.5/32, got %q", got)
		}
	})
}

func TestIntToIpv4(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input uint32
		want  string
	}{
		{name: "zero", input: 0, want: "0.0.0.0"},
		{name: "sequential octets", input: 0x01020304, want: "1.2.3.4"},
		{name: "all ones", input: 0xFFFFFFFF, want: "255.255.255.255"},
		{name: "typical address", input: 0xC0A80101, want: "192.168.1.1"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := IntToIpv4(testCase.input)
			if got.String() != testCase.want {
				t.Fatalf("expected %q, got %q", testCase.want, got.String())
			}
		})
	}
}

func TestNetworkFromTarget(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		target  string
		wantNil bool
		want    string
		wantErr bool
	}{
		{name: "empty returns nil", target: "", wantNil: true},
		{name: "ipv4 cidr", target: "10.0.0.0/8", want: "10.0.0.0/8"},
		{name: "ipv6 cidr", target: "2001:db8::/32", want: "2001:db8::/32"},
		{name: "single ipv4", target: "1.2.3.4", want: "1.2.3.4/32"},
		{name: "single ipv6", target: "::1", want: "::1/128"},
		{name: "garbage", target: "not-a-target", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := NetworkFromTarget(testCase.target)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, motmedelNetErrors.ErrUndeterminableTargetFormat) {
					t.Fatalf("expected ErrUndeterminableTargetFormat, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if testCase.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil network, got nil")
			}
			if got.String() != testCase.want {
				t.Fatalf("expected %q, got %q", testCase.want, got.String())
			}
		})
	}
}

func TestIsLocalhost(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		hostname string
		expected bool
	}{
		{name: "localhost", hostname: "localhost", expected: true},
		{name: "in another case", hostname: "LocalHost", expected: true},
		// RFC 6761 reserves everything under localhost for the same purpose.
		{name: "a subdomain", hostname: "service.localhost", expected: true},
		{name: "a subdomain in another case", hostname: "Service.LOCALHOST", expected: true},
		{name: "a name ending in it", hostname: "notlocalhost", expected: false},
		{name: "a domain that merely starts with it", hostname: "localhost.example.com", expected: false},
		{name: "another host", hostname: "example.com", expected: false},
		{name: "the loopback address", hostname: "127.0.0.1", expected: false},
		{name: "empty", hostname: "", expected: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := IsLocalhost(testCase.hostname); got != testCase.expected {
				t.Errorf("is localhost: got %t, want %t", got, testCase.expected)
			}
		})
	}
}
