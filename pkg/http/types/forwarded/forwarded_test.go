package forwarded

import (
	"reflect"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types"
)

// Corpus of valid Forwarded header values from RFC 7239 examples and common use cases.
func TestParseForwardedCorpus(t *testing.T) {
	t.Parallel()

	cases := []string{
		"for=192.0.2.43",
		"for=192.0.2.60;proto=http;by=203.0.113.43",
		"for=192.0.2.43, for=198.51.100.178",
		"for=\"_gazonk\"",
		"For=\"[2001:db8:cafe::17]:4711\"",
		"for=192.0.2.43;proto=https",
		"for=192.0.2.43;host=example.com",
		"for=192.0.2.43;by=203.0.113.43;host=example.com;proto=https",
		"for=unknown",
		"for=_hidden",
		"for=192.0.2.43, for=\"[2001:db8:cafe::17]\"",
		"for=192.0.2.43;proto=http, for=198.51.100.178;proto=https",
	}

	for _, c := range cases {
		c := c
		t.Run(c, func(t *testing.T) {
			t.Parallel()

			p, err := Parse([]byte(c))
			if err != nil {
				t.Fatalf("Parse error for %q: %v", c, err)
			}
			if p == nil {
				t.Fatalf("nil result for %q", c)
			}
			if len(p.Elements) == 0 {
				t.Fatalf("no elements parsed for %q", c)
			}
		})
	}
}

// Verify parsed values for specific Forwarded header examples.
func TestParseForwardedCorrectness(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		header string
		want   *types.Forwarded
	}{
		{
			name:   "simple for",
			header: "for=192.0.2.43",
			want: &types.Forwarded{
				Elements: []*types.ForwardedElement{
					{For: "192.0.2.43"},
				},
			},
		},
		{
			name:   "for with proto and by",
			header: "for=192.0.2.60;proto=http;by=203.0.113.43",
			want: &types.Forwarded{
				Elements: []*types.ForwardedElement{
					{For: "192.0.2.60", Proto: "http", By: "203.0.113.43"},
				},
			},
		},
		{
			name:   "multiple elements",
			header: "for=192.0.2.43, for=198.51.100.178",
			want: &types.Forwarded{
				Elements: []*types.ForwardedElement{
					{For: "192.0.2.43"},
					{For: "198.51.100.178"},
				},
			},
		},
		{
			name:   "quoted obfuscated identifier",
			header: "for=\"_gazonk\"",
			want: &types.Forwarded{
				Elements: []*types.ForwardedElement{
					{For: "_gazonk"},
				},
			},
		},
		{
			name:   "quoted IPv6 with port",
			header: "For=\"[2001:db8:cafe::17]:4711\"",
			want: &types.Forwarded{
				Elements: []*types.ForwardedElement{
					{For: "[2001:db8:cafe::17]:4711"},
				},
			},
		},
		{
			name:   "all standard parameters",
			header: "for=192.0.2.43;by=203.0.113.43;host=example.com;proto=https",
			want: &types.Forwarded{
				Elements: []*types.ForwardedElement{
					{For: "192.0.2.43", By: "203.0.113.43", Host: "example.com", Proto: "https"},
				},
			},
		},
		{
			name:   "unknown for value",
			header: "for=unknown",
			want: &types.Forwarded{
				Elements: []*types.ForwardedElement{
					{For: "unknown"},
				},
			},
		},
		{
			name:   "extension parameter",
			header: "for=192.0.2.43;secret=mytoken",
			want: &types.Forwarded{
				Elements: []*types.ForwardedElement{
					{For: "192.0.2.43", Extensions: map[string]string{"secret": "mytoken"}},
				},
			},
		},
		{
			name:   "case insensitive parameter names",
			header: "FOR=192.0.2.43;PROTO=https",
			want: &types.Forwarded{
				Elements: []*types.ForwardedElement{
					{For: "192.0.2.43", Proto: "https"},
				},
			},
		},
		{
			name:   "multiple elements with different parameters",
			header: "for=192.0.2.43;proto=http, for=198.51.100.178;proto=https",
			want: &types.Forwarded{
				Elements: []*types.ForwardedElement{
					{For: "192.0.2.43", Proto: "http"},
					{For: "198.51.100.178", Proto: "https"},
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := Parse([]byte(tc.header))
			if err != nil {
				t.Fatalf("Parse error: %v (header=%q)", err, tc.header)
			}
			if got == nil {
				t.Fatalf("nil result (header=%q)", tc.header)
			}
			if len(got.Elements) != len(tc.want.Elements) {
				t.Fatalf("got %d elements, want %d (header=%q)", len(got.Elements), len(tc.want.Elements), tc.header)
			}
			for i, wantElem := range tc.want.Elements {
				gotElem := got.Elements[i]
				if gotElem.For != wantElem.For {
					t.Errorf("element[%d].For=%q, want %q (header=%q)", i, gotElem.For, wantElem.For, tc.header)
				}
				if gotElem.By != wantElem.By {
					t.Errorf("element[%d].By=%q, want %q (header=%q)", i, gotElem.By, wantElem.By, tc.header)
				}
				if gotElem.Host != wantElem.Host {
					t.Errorf("element[%d].Host=%q, want %q (header=%q)", i, gotElem.Host, wantElem.Host, tc.header)
				}
				if gotElem.Proto != wantElem.Proto {
					t.Errorf("element[%d].Proto=%q, want %q (header=%q)", i, gotElem.Proto, wantElem.Proto, tc.header)
				}
				if !reflect.DeepEqual(gotElem.Extensions, wantElem.Extensions) {
					t.Errorf("element[%d].Extensions=%v, want %v (header=%q)", i, gotElem.Extensions, wantElem.Extensions, tc.header)
				}
			}
		})
	}
}

// Test invalid Forwarded header values.
func TestParseForwardedInvalid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		header string
	}{
		{
			name:   "empty string",
			header: "",
		},
		{
			name:   "missing equals",
			header: "for192.0.2.43",
		},
		{
			name:   "unclosed quote",
			header: "for=\"192.0.2.43",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := Parse([]byte(tc.header))
			if err == nil {
				t.Fatalf("expected error for invalid header %q, got nil", tc.header)
			}
		})
	}
}
