package etag

import (
	"testing"

	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

func TestParseValid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		header string
		want   altshiftHttpTypes.ETag
	}{
		{
			name:   "simple",
			header: `"xyzzy"`,
			want:   altshiftHttpTypes.ETag{Weak: false, Tag: "xyzzy"},
		},
		{
			name:   "empty opaque-tag",
			header: `""`,
			want:   altshiftHttpTypes.ETag{Weak: false, Tag: ""},
		},
		{
			name:   "weak",
			header: `W/"xyzzy"`,
			want:   altshiftHttpTypes.ETag{Weak: true, Tag: "xyzzy"},
		},
		{
			name:   "weak empty",
			header: `W/""`,
			want:   altshiftHttpTypes.ETag{Weak: true, Tag: ""},
		},
		{
			name:   "hex-like",
			header: `"33a64df551425fcc55e4d42a148795d9f25f89d4"`,
			want:   altshiftHttpTypes.ETag{Weak: false, Tag: "33a64df551425fcc55e4d42a148795d9f25f89d4"},
		},
		{
			name:   "with dash",
			header: `"686897696a7c876b7e"`,
			want:   altshiftHttpTypes.ETag{Weak: false, Tag: "686897696a7c876b7e"},
		},
		{
			name:   "weak hex",
			header: `W/"0815"`,
			want:   altshiftHttpTypes.ETag{Weak: true, Tag: "0815"},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got, err := Parse([]byte(c.header))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", c.header, err)
			}
			if got == nil {
				t.Fatalf("Parse(%q) returned nil", c.header)
			}
			if got.Weak != c.want.Weak {
				t.Errorf("Weak = %v, want %v", got.Weak, c.want.Weak)
			}
			if got.Tag != c.want.Tag {
				t.Errorf("Tag = %q, want %q", got.Tag, c.want.Tag)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		header string
	}{
		{name: "empty", header: ``},
		{name: "unquoted", header: `xyzzy`},
		{name: "missing trailing quote", header: `"xyzzy`},
		{name: "missing leading quote", header: `xyzzy"`},
		{name: "lowercase weak", header: `w/"xyzzy"`},
		{name: "weak without slash", header: `W"xyzzy"`},
		{name: "weak without opaque", header: `W/`},
		{name: "embedded dquote", header: `"ab"cd"`},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Parse([]byte(c.header)); err == nil {
				t.Fatalf("Parse(%q) expected error, got nil", c.header)
			}
		})
	}
}

func TestStringRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []string{
		`"xyzzy"`,
		`W/"xyzzy"`,
		`""`,
		`W/""`,
		`"33a64df551425fcc55e4d42a148795d9f25f89d4"`,
	}

	for _, c := range cases {
		c := c
		t.Run(c, func(t *testing.T) {
			t.Parallel()

			parsed, err := Parse([]byte(c))
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", c, err)
			}
			if got := parsed.String(); got != c {
				t.Errorf("String() = %q, want %q", got, c)
			}
		})
	}
}
