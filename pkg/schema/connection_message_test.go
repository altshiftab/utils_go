package schema

import (
	"testing"
)

func TestBaseMakeConnectionMessage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		base *Base
		want string
	}{
		{
			name: "nil base",
			base: nil,
			want: "",
		},
		{
			name: "all nil",
			base: &Base{},
			want: unknownPlaceholder,
		},
		{
			name: "source non-nil but empty",
			base: &Base{Source: &Target{}},
			want: unknownPlaceholder,
		},
		{
			name: "network present but empty",
			base: &Base{Network: &Network{}},
			want: unknownPlaceholder,
		},
		{
			name: "source ip and port",
			base: &Base{Source: &Target{Ip: "10.0.0.1", Port: 1234}},
			want: "10.0.0.1:1234 -> (unknown) (unknown)",
		},
		{
			name: "source ip only",
			base: &Base{Source: &Target{Ip: "10.0.0.1"}},
			want: "10.0.0.1 -> (unknown) (unknown)",
		},
		{
			name: "source port only",
			base: &Base{Source: &Target{Port: 22}},
			want: ":22 -> (unknown) (unknown)",
		},
		{
			name: "destination ip and port",
			base: &Base{Destination: &Target{Ip: "10.0.0.2", Port: 80}},
			want: "(unknown) -> 10.0.0.2:80 (unknown)",
		},
		{
			name: "destination ip only",
			base: &Base{Destination: &Target{Ip: "1.1.1.1"}},
			want: "(unknown) -> 1.1.1.1 (unknown)",
		},
		{
			name: "destination port only",
			base: &Base{Destination: &Target{Port: 53}},
			want: "(unknown) -> :53 (unknown)",
		},
		{
			name: "network transport only",
			base: &Base{Network: &Network{Transport: "udp"}},
			want: "(unknown) -> (unknown) udp",
		},
		{
			name: "network iana number only",
			base: &Base{Network: &Network{IanaNumber: "6"}},
			want: "(unknown) -> (unknown) (6)",
		},
		{
			name: "network transport takes precedence over iana number",
			base: &Base{Network: &Network{Transport: "tcp", IanaNumber: "6"}},
			want: "(unknown) -> (unknown) tcp",
		},
		{
			name: "fully populated",
			base: &Base{
				Source:      &Target{Ip: "10.0.0.1", Port: 1234},
				Destination: &Target{Ip: "10.0.0.2", Port: 80},
				Network:     &Network{Transport: "tcp"},
			},
			want: "10.0.0.1:1234 -> 10.0.0.2:80 tcp",
		},
		{
			name: "mixed known parts",
			base: &Base{
				Source:      &Target{Ip: "10.0.0.1"},
				Destination: &Target{Port: 80},
				Network:     &Network{Transport: "tcp"},
			},
			want: "10.0.0.1 -> :80 tcp",
		},
		{
			name: "ipv6 addresses are bracketed by JoinHostPort",
			base: &Base{
				Source:      &Target{Ip: "2001:db8::1", Port: 443},
				Destination: &Target{Ip: "::1", Port: 80},
				Network:     &Network{Transport: "tcp"},
			},
			want: "[2001:db8::1]:443 -> [::1]:80 tcp",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := testCase.base.MakeConnectionMessage()
			if got != testCase.want {
				t.Errorf("MakeConnectionMessage() = %q, want %q", got, testCase.want)
			}
		})
	}
}
