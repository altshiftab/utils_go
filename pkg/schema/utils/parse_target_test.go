package utils

import "testing"

func TestParseTarget(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		rawAddress   string
		rawIpAddress string
		rawPort      int
		expectedIp   string
		expectedPort int
		wantErr      bool
	}{
		{name: "an address with a port", rawIpAddress: "192.0.2.1:4321", expectedIp: "192.0.2.1", expectedPort: 4321},
		{name: "an address without a port keeps the given one", rawIpAddress: "192.0.2.1", rawPort: 43, expectedIp: "192.0.2.1", expectedPort: 43},
		{name: "a bracketed IPv6 address with a port", rawIpAddress: "[2001:db8::1]:443", expectedIp: "2001:db8::1", expectedPort: 443},
		{name: "a bracketed IPv6 address without a port", rawIpAddress: "[2001:db8::1]", rawPort: 43, expectedIp: "2001:db8::1", expectedPort: 43},
		{name: "a bare IPv6 address", rawIpAddress: "2001:db8::1", rawPort: 43, expectedIp: "2001:db8::1", expectedPort: 43},
		{name: "a non-numeric port", rawIpAddress: "192.0.2.1:whois", wantErr: true},
		{name: "a domain only", rawAddress: "whois.example.com", rawPort: 43, expectedPort: 43},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			target, err := parseTarget(testCase.rawAddress, testCase.rawIpAddress, testCase.rawPort)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("%s: expected an error", testCase.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", testCase.name, err)
			}
			if target == nil {
				t.Fatalf("%s: no target", testCase.name)
			}
			if target.Ip != testCase.expectedIp || target.Port != testCase.expectedPort {
				t.Errorf("%s: got ip %q port %d, want ip %q port %d", testCase.name, target.Ip, target.Port, testCase.expectedIp, testCase.expectedPort)
			}
		})
	}
}
