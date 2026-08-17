package flow_tuple

import (
	"net"
	"testing"

	altshiftNet "github.com/altshiftab/utils_go/pkg/net"
)

// Test vectors from the official Community ID spec:
// https://github.com/corelight/community-id-spec/blob/master/baseline/baseline_deflt.json
func TestHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sourceIp    string
		destIp      string
		sourcePort  uint16
		destPort    uint16
		protocol    uint8
		communityId string
	}{
		// TCP IPv4.
		{
			name:        "TCP IPv4 128.232.110.120:34855 -> 66.35.250.204:80",
			sourceIp:    "128.232.110.120",
			destIp:      "66.35.250.204",
			sourcePort:  34855,
			destPort:    80,
			protocol:    altshiftNet.ProtocolTcp,
			communityId: "1:LQU9qZlK+B5F3KDmev6m5PMibrg=",
		},
		{
			name:        "TCP IPv4 66.35.250.204:80 -> 128.232.110.120:34855 (reverse)",
			sourceIp:    "66.35.250.204",
			destIp:      "128.232.110.120",
			sourcePort:  80,
			destPort:    34855,
			protocol:    altshiftNet.ProtocolTcp,
			communityId: "1:LQU9qZlK+B5F3KDmev6m5PMibrg=",
		},

		// UDP IPv4.
		{
			name:        "UDP IPv4 192.168.1.52:54585 -> 8.8.8.8:53",
			sourceIp:    "192.168.1.52",
			destIp:      "8.8.8.8",
			sourcePort:  54585,
			destPort:    53,
			protocol:    altshiftNet.ProtocolUdp,
			communityId: "1:d/FP5EW3wiY1vCndhwleRRKHowQ=",
		},
		{
			name:        "UDP IPv4 8.8.8.8:53 -> 192.168.1.52:54585 (reverse)",
			sourceIp:    "8.8.8.8",
			destIp:      "192.168.1.52",
			sourcePort:  53,
			destPort:    54585,
			protocol:    altshiftNet.ProtocolUdp,
			communityId: "1:d/FP5EW3wiY1vCndhwleRRKHowQ=",
		},

		// ICMP IPv4 echo request/reply.
		{
			name:        "ICMP echo request 192.168.0.89 -> 192.168.0.1 type=8 code=0",
			sourceIp:    "192.168.0.89",
			destIp:      "192.168.0.1",
			sourcePort:  8,
			destPort:    0,
			protocol:    altshiftNet.ProtocolIcmp,
			communityId: "1:X0snYXpgwiv9TZtqg64sgzUn6Dk=",
		},
		{
			name:        "ICMP echo reply 192.168.0.1 -> 192.168.0.89 type=0 code=8",
			sourceIp:    "192.168.0.1",
			destIp:      "192.168.0.89",
			sourcePort:  0,
			destPort:    8,
			protocol:    altshiftNet.ProtocolIcmp,
			communityId: "1:X0snYXpgwiv9TZtqg64sgzUn6Dk=",
		},

		// ICMP IPv4 one-way (type 11 not in equivalents map).
		{
			name:        "ICMP one-way 10.0.0.1 -> 10.0.0.2 type=11 code=0",
			sourceIp:    "10.0.0.1",
			destIp:      "10.0.0.2",
			sourcePort:  11,
			destPort:    0,
			protocol:    altshiftNet.ProtocolIcmp,
			communityId: "1:YHxtAirCG//0OzkcVAukqKQN9xM=",
		},

		// TCP IPv6.
		{
			name:        "TCP IPv6 2001:470:e5bf:dead:4957:2174:e82c:4887:63943 -> 2607:f8b0:400c:c03::1a:25",
			sourceIp:    "2001:470:e5bf:dead:4957:2174:e82c:4887",
			destIp:      "2607:f8b0:400c:c03::1a",
			sourcePort:  63943,
			destPort:    25,
			protocol:    altshiftNet.ProtocolTcp,
			communityId: "1:/qFaeAR+gFe1KYjMzVDsMv+wgU4=",
		},
		{
			name:        "TCP IPv6 reverse",
			sourceIp:    "2607:f8b0:400c:c03::1a",
			destIp:      "2001:470:e5bf:dead:4957:2174:e82c:4887",
			sourcePort:  25,
			destPort:    63943,
			protocol:    altshiftNet.ProtocolTcp,
			communityId: "1:/qFaeAR+gFe1KYjMzVDsMv+wgU4=",
		},

		// ICMPv6 neighbor solicitation/advertisement.
		{
			name:        "ICMPv6 neighbor solicitation fe80::200:86ff:fe05:80da -> fe80::260:97ff:fe07:69ea type=135 code=136",
			sourceIp:    "fe80::200:86ff:fe05:80da",
			destIp:      "fe80::260:97ff:fe07:69ea",
			sourcePort:  135,
			destPort:    136,
			protocol:    altshiftNet.ProtocolIcmp6,
			communityId: "1:dGHyGvjMfljg6Bppwm3bg0LO8TY=",
		},
		{
			name:        "ICMPv6 neighbor advertisement (reverse)",
			sourceIp:    "fe80::260:97ff:fe07:69ea",
			destIp:      "fe80::200:86ff:fe05:80da",
			sourcePort:  136,
			destPort:    135,
			protocol:    altshiftNet.ProtocolIcmp6,
			communityId: "1:dGHyGvjMfljg6Bppwm3bg0LO8TY=",
		},

		// ICMPv6 echo request/reply.
		{
			name:        "ICMPv6 echo request",
			sourceIp:    "3ffe:507:0:1:200:86ff:fe05:80da",
			destIp:      "3ffe:501:0:1001::2",
			sourcePort:  128,
			destPort:    129,
			protocol:    altshiftNet.ProtocolIcmp6,
			communityId: "1:+TW+HtLHvV1xnGhV1lv7XoJrqQg=",
		},
		{
			name:        "ICMPv6 echo reply (reverse)",
			sourceIp:    "3ffe:501:0:1001::2",
			destIp:      "3ffe:507:0:1:200:86ff:fe05:80da",
			sourcePort:  129,
			destPort:    128,
			protocol:    altshiftNet.ProtocolIcmp6,
			communityId: "1:+TW+HtLHvV1xnGhV1lv7XoJrqQg=",
		},

		// SCTP (proto 132).
		{
			name:        "SCTP 192.168.170.8:7 -> 192.168.170.56:7",
			sourceIp:    "192.168.170.8",
			destIp:      "192.168.170.56",
			sourcePort:  7,
			destPort:    7,
			protocol:    132,
			communityId: "1:MP2EtRCAUIZvTw6MxJHLV7N7JDs=",
		},
		{
			name:        "SCTP reverse",
			sourceIp:    "192.168.170.56",
			destIp:      "192.168.170.8",
			sourcePort:  7,
			destPort:    7,
			protocol:    132,
			communityId: "1:MP2EtRCAUIZvTw6MxJHLV7N7JDs=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tuple := New(
				net.ParseIP(tt.sourceIp),
				net.ParseIP(tt.destIp),
				tt.sourcePort,
				tt.destPort,
				tt.protocol,
			)

			got := tuple.Hash()
			if got != tt.communityId {
				t.Errorf("Hash() = %q, want %q", got, tt.communityId)
			}
		})
	}
}

func TestHashDirectionIndependence(t *testing.T) {
	t.Parallel()

	forward := New(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		12345,
		80,
		altshiftNet.ProtocolTcp,
	)
	reverse := New(
		net.ParseIP("10.0.0.2"),
		net.ParseIP("10.0.0.1"),
		80,
		12345,
		altshiftNet.ProtocolTcp,
	)

	if forward.Hash() != reverse.Hash() {
		t.Errorf("forward hash %q != reverse hash %q", forward.Hash(), reverse.Hash())
	}
}

func TestNewAlwaysOrdered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		sourceIp       string
		destIp         string
		sourcePort     uint16
		destPort       uint16
		protocol       uint8
		wantSourceIp   string
		wantDestIp     string
		wantSourcePort uint16
		wantDestPort   uint16
		wantIsOneWay   bool
	}{
		{
			name:           "TCP already ordered",
			sourceIp:       "10.0.0.1",
			destIp:         "10.0.0.2",
			sourcePort:     12345,
			destPort:       80,
			protocol:       altshiftNet.ProtocolTcp,
			wantSourceIp:   "10.0.0.1",
			wantDestIp:     "10.0.0.2",
			wantSourcePort: 12345,
			wantDestPort:   80,
			wantIsOneWay:   false,
		},
		{
			name:           "TCP reversed gets reordered",
			sourceIp:       "10.0.0.2",
			destIp:         "10.0.0.1",
			sourcePort:     80,
			destPort:       12345,
			protocol:       altshiftNet.ProtocolTcp,
			wantSourceIp:   "10.0.0.1",
			wantDestIp:     "10.0.0.2",
			wantSourcePort: 12345,
			wantDestPort:   80,
			wantIsOneWay:   false,
		},
		{
			name:           "same IPs ordered by port",
			sourceIp:       "10.0.0.1",
			destIp:         "10.0.0.1",
			sourcePort:     443,
			destPort:       80,
			protocol:       altshiftNet.ProtocolTcp,
			wantSourceIp:   "10.0.0.1",
			wantDestIp:     "10.0.0.1",
			wantSourcePort: 80,
			wantDestPort:   443,
			wantIsOneWay:   false,
		},
		{
			name:           "ICMP echo request maps ports and orders",
			sourceIp:       "192.168.0.89",
			destIp:         "192.168.0.1",
			sourcePort:     8,
			destPort:       0,
			protocol:       altshiftNet.ProtocolIcmp,
			wantSourceIp:   "192.168.0.1",
			wantDestIp:     "192.168.0.89",
			wantSourcePort: 0,
			wantDestPort:   8,
			wantIsOneWay:   false,
		},
		{
			name:           "ICMP one-way preserves original order",
			sourceIp:       "10.0.0.2",
			destIp:         "10.0.0.1",
			sourcePort:     11,
			destPort:       0,
			protocol:       altshiftNet.ProtocolIcmp,
			wantSourceIp:   "10.0.0.2",
			wantDestIp:     "10.0.0.1",
			wantSourcePort: 11,
			wantDestPort:   0,
			wantIsOneWay:   true,
		},
		{
			name:           "ICMPv6 echo request maps ports",
			sourceIp:       "fe80::1",
			destIp:         "fe80::2",
			sourcePort:     128,
			destPort:       0,
			protocol:       altshiftNet.ProtocolIcmp6,
			wantSourceIp:   "fe80::1",
			wantDestIp:     "fe80::2",
			wantSourcePort: 128,
			wantDestPort:   129,
			wantIsOneWay:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tuple := New(
				net.ParseIP(tt.sourceIp),
				net.ParseIP(tt.destIp),
				tt.sourcePort,
				tt.destPort,
				tt.protocol,
			)

			if !tuple.SourceIp.Equal(net.ParseIP(tt.wantSourceIp)) {
				t.Errorf("SourceIp = %v, want %v", tuple.SourceIp, tt.wantSourceIp)
			}
			if !tuple.DestinationIp.Equal(net.ParseIP(tt.wantDestIp)) {
				t.Errorf("DestinationIp = %v, want %v", tuple.DestinationIp, tt.wantDestIp)
			}
			if tuple.SourcePort != tt.wantSourcePort {
				t.Errorf("SourcePort = %d, want %d", tuple.SourcePort, tt.wantSourcePort)
			}
			if tuple.DestinationPort != tt.wantDestPort {
				t.Errorf("DestinationPort = %d, want %d", tuple.DestinationPort, tt.wantDestPort)
			}
			if tuple.IsOneWay != tt.wantIsOneWay {
				t.Errorf("IsOneWay = %v, want %v", tuple.IsOneWay, tt.wantIsOneWay)
			}
		})
	}
}

func BenchmarkHash(b *testing.B) {
	tuple := New(
		net.ParseIP("128.232.110.120"),
		net.ParseIP("66.35.250.204"),
		34855,
		80,
		altshiftNet.ProtocolTcp,
	)

	for b.Loop() {
		tuple.Hash()
	}
}
