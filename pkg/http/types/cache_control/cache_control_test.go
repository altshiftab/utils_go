package cache_control

import (
	"errors"
	"reflect"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

// Corpus of realistic Cache-Control header values; tests focus on parsing without errors.
func TestParseCacheControlCorpus(t *testing.T) {
	t.Parallel()

	cases := []string{
		"no-cache",
		"no-store",
		"no-transform",
		"only-if-cached",
		"max-age=0",
		"max-age=3600",
		"max-age=86400",
		"max-stale",
		"max-stale=60",
		"min-fresh=120",
		"public",
		"private",
		"must-revalidate",
		"must-understand",
		"proxy-revalidate",
		"s-maxage=3600",
		"no-cache, no-store",
		"public, max-age=31536000",
		"no-cache, no-store, must-revalidate",
		"private, max-age=0, no-cache",
		"public, max-age=604800, immutable",
		"max-age=3600, s-maxage=7200, proxy-revalidate",
		"max-age=0, must-revalidate",
		"no-store, no-cache, must-revalidate, max-age=0",
		"max-age=3600, stale-while-revalidate=60",
		"max-age=600, stale-if-error=1200",
		"private, no-cache, no-store, max-age=0, must-revalidate",
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
			if len(p.Directives) == 0 {
				t.Fatalf("no directives parsed for %q", c)
			}
		})
	}
}

// Verify parsed directive names and values for representative headers.
func TestParseCacheControlCorrectness(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		header string
		want   []*altshiftHttpTypes.CacheControlDirective
	}{
		{
			name:   "no-cache",
			header: "no-cache",
			want: []*altshiftHttpTypes.CacheControlDirective{
				{Name: "no-cache"},
			},
		},
		{
			name:   "max-age",
			header: "max-age=3600",
			want: []*altshiftHttpTypes.CacheControlDirective{
				{Name: "max-age", Value: "3600"},
			},
		},
		{
			name:   "max-stale without value",
			header: "max-stale",
			want: []*altshiftHttpTypes.CacheControlDirective{
				{Name: "max-stale"},
			},
		},
		{
			name:   "max-stale with value",
			header: "max-stale=60",
			want: []*altshiftHttpTypes.CacheControlDirective{
				{Name: "max-stale", Value: "60"},
			},
		},
		{
			name:   "public with max-age",
			header: "public, max-age=31536000",
			want: []*altshiftHttpTypes.CacheControlDirective{
				{Name: "public"},
				{Name: "max-age", Value: "31536000"},
			},
		},
		{
			name:   "private with no-cache and must-revalidate",
			header: "private, no-cache, must-revalidate",
			want: []*altshiftHttpTypes.CacheControlDirective{
				{Name: "private"},
				{Name: "no-cache"},
				{Name: "must-revalidate"},
			},
		},
		{
			name:   "s-maxage with proxy-revalidate",
			header: "max-age=3600, s-maxage=7200, proxy-revalidate",
			want: []*altshiftHttpTypes.CacheControlDirective{
				{Name: "max-age", Value: "3600"},
				{Name: "s-maxage", Value: "7200"},
				{Name: "proxy-revalidate"},
			},
		},
		{
			name:   "quoted string value",
			header: "no-cache=\"Set-Cookie\"",
			want: []*altshiftHttpTypes.CacheControlDirective{
				{Name: "no-cache", Value: "Set-Cookie"},
			},
		},
		{
			name:   "private with quoted field names",
			header: "private=\"Set-Cookie, X-Custom\"",
			want: []*altshiftHttpTypes.CacheControlDirective{
				{Name: "private", Value: "Set-Cookie, X-Custom"},
			},
		},
		{
			name:   "extension directive",
			header: "max-age=600, stale-while-revalidate=60",
			want: []*altshiftHttpTypes.CacheControlDirective{
				{Name: "max-age", Value: "600"},
				{Name: "stale-while-revalidate", Value: "60"},
			},
		},
		{
			name:   "directive names are lowercased",
			header: "No-Cache, Max-Age=300",
			want: []*altshiftHttpTypes.CacheControlDirective{
				{Name: "no-cache"},
				{Name: "max-age", Value: "300"},
			},
		},
		{
			name:   "spaces around commas",
			header: "no-store ,  no-cache ,  must-revalidate",
			want: []*altshiftHttpTypes.CacheControlDirective{
				{Name: "no-store"},
				{Name: "no-cache"},
				{Name: "must-revalidate"},
			},
		},
		{
			name:   "all request directives",
			header: "max-age=0, max-stale=60, min-fresh=120, no-cache, no-store, no-transform, only-if-cached",
			want: []*altshiftHttpTypes.CacheControlDirective{
				{Name: "max-age", Value: "0"},
				{Name: "max-stale", Value: "60"},
				{Name: "min-fresh", Value: "120"},
				{Name: "no-cache"},
				{Name: "no-store"},
				{Name: "no-transform"},
				{Name: "only-if-cached"},
			},
		},
		{
			name:   "all response directives",
			header: "max-age=3600, must-revalidate, must-understand, no-cache, no-store, no-transform, private, proxy-revalidate, public, s-maxage=7200",
			want: []*altshiftHttpTypes.CacheControlDirective{
				{Name: "max-age", Value: "3600"},
				{Name: "must-revalidate"},
				{Name: "must-understand"},
				{Name: "no-cache"},
				{Name: "no-store"},
				{Name: "no-transform"},
				{Name: "private"},
				{Name: "proxy-revalidate"},
				{Name: "public"},
				{Name: "s-maxage", Value: "7200"},
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
			if len(got.Directives) != len(tc.want) {
				t.Fatalf("got %d directives, want %d (header=%q)", len(got.Directives), len(tc.want), tc.header)
			}
			for i, wantDir := range tc.want {
				gotDir := got.Directives[i]
				if !reflect.DeepEqual(gotDir, wantDir) {
					t.Errorf("directive[%d]=%+v, want %+v (header=%q)", i, gotDir, wantDir, tc.header)
				}
			}
			if got.Raw != tc.header {
				t.Errorf("Raw=%q, want %q", got.Raw, tc.header)
			}
		})
	}
}

// Test invalid Cache-Control header values.
func TestParseCacheControlInvalid(t *testing.T) {
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
			name:   "unclosed quote",
			header: "no-cache=\"Set-Cookie",
		},
		{
			name:   "bare equals",
			header: "=value",
		},
		{
			name:   "max-age non-numeric",
			header: "max-age=abc",
		},
		{
			name:   "s-maxage non-numeric",
			header: "s-maxage=foo",
		},
		{
			name:   "min-fresh non-numeric",
			header: "min-fresh=bar",
		},
		{
			name:   "max-stale non-numeric",
			header: "max-stale=baz",
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

// Verify that non-numeric delta-seconds values produce a semantic error.
func TestParseCacheControlInvalidDeltaSeconds(t *testing.T) {
	t.Parallel()

	cases := []string{
		"max-age=abc",
		"s-maxage=foo",
		"min-fresh=bar",
		"max-stale=baz",
	}

	for _, c := range cases {
		c := c
		t.Run(c, func(t *testing.T) {
			t.Parallel()

			_, err := Parse([]byte(c))
			if err == nil {
				t.Fatalf("expected error for %q, got nil", c)
			}
			if !errors.Is(err, altshiftErrors.ErrSemanticError) {
				t.Fatalf("expected ErrSemanticError, got %v", err)
			}
		})
	}
}

// Verify accessor methods on CacheControl.
func TestCacheControlMethods(t *testing.T) {
	t.Parallel()

	t.Run("boolean directives", func(t *testing.T) {
		t.Parallel()

		cc, err := Parse([]byte("no-cache, no-store, no-transform, only-if-cached, must-revalidate, must-understand, proxy-revalidate, public, private"))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}

		if !cc.NoCache() {
			t.Error("NoCache() = false, want true")
		}
		if !cc.NoStore() {
			t.Error("NoStore() = false, want true")
		}
		if !cc.NoTransform() {
			t.Error("NoTransform() = false, want true")
		}
		if !cc.OnlyIfCached() {
			t.Error("OnlyIfCached() = false, want true")
		}
		if !cc.MustRevalidate() {
			t.Error("MustRevalidate() = false, want true")
		}
		if !cc.MustUnderstand() {
			t.Error("MustUnderstand() = false, want true")
		}
		if !cc.ProxyRevalidate() {
			t.Error("ProxyRevalidate() = false, want true")
		}
		if !cc.Public() {
			t.Error("Public() = false, want true")
		}
		if !cc.Private() {
			t.Error("Private() = false, want true")
		}
	})

	t.Run("boolean directives absent", func(t *testing.T) {
		t.Parallel()

		cc, err := Parse([]byte("max-age=60"))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}

		if cc.NoCache() {
			t.Error("NoCache() = true, want false")
		}
		if cc.NoStore() {
			t.Error("NoStore() = true, want false")
		}
		if cc.MustRevalidate() {
			t.Error("MustRevalidate() = true, want false")
		}
		if cc.Public() {
			t.Error("Public() = true, want false")
		}
		if cc.Private() {
			t.Error("Private() = true, want false")
		}
	})

	t.Run("MaxAge", func(t *testing.T) {
		t.Parallel()

		cc, err := Parse([]byte("max-age=3600"))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		val, err := cc.MaxAge()
		if err != nil {
			t.Fatalf("MaxAge() error: %v", err)
		}
		if val != 3600 {
			t.Fatalf("MaxAge() = %d, want 3600", val)
		}
	})

	t.Run("MaxAge absent", func(t *testing.T) {
		t.Parallel()

		cc, err := Parse([]byte("no-cache"))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		_, err = cc.MaxAge()
		if !errors.Is(err, altshiftHttpTypes.ErrDirectiveNotPresent) {
			t.Fatalf("MaxAge() error = %v, want ErrDirectiveNotPresent", err)
		}
	})

	t.Run("SMaxAge", func(t *testing.T) {
		t.Parallel()

		cc, err := Parse([]byte("s-maxage=7200"))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		val, err := cc.SMaxAge()
		if err != nil {
			t.Fatalf("SMaxAge() error: %v", err)
		}
		if val != 7200 {
			t.Fatalf("SMaxAge() = %d, want 7200", val)
		}
	})

	t.Run("MinFresh", func(t *testing.T) {
		t.Parallel()

		cc, err := Parse([]byte("min-fresh=120"))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		val, err := cc.MinFresh()
		if err != nil {
			t.Fatalf("MinFresh() error: %v", err)
		}
		if val != 120 {
			t.Fatalf("MinFresh() = %d, want 120", val)
		}
	})

	t.Run("MaxStale with value", func(t *testing.T) {
		t.Parallel()

		cc, err := Parse([]byte("max-stale=60"))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		val, hasValue, err := cc.MaxStale()
		if err != nil {
			t.Fatalf("MaxStale() error: %v", err)
		}
		if !hasValue {
			t.Fatal("MaxStale() hasValue=false, want true")
		}
		if val != 60 {
			t.Fatalf("MaxStale() value = %d, want 60", val)
		}
	})

	t.Run("MaxStale without value", func(t *testing.T) {
		t.Parallel()

		cc, err := Parse([]byte("max-stale"))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		_, hasValue, err := cc.MaxStale()
		if err != nil {
			t.Fatalf("MaxStale() error: %v", err)
		}
		if hasValue {
			t.Fatal("MaxStale() hasValue=true, want false (unlimited)")
		}
	})

	t.Run("MaxStale absent", func(t *testing.T) {
		t.Parallel()

		cc, err := Parse([]byte("no-cache"))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		_, _, err = cc.MaxStale()
		if !errors.Is(err, altshiftHttpTypes.ErrDirectiveNotPresent) {
			t.Fatalf("MaxStale() error = %v, want ErrDirectiveNotPresent", err)
		}
	})

	t.Run("NoCacheFieldNames", func(t *testing.T) {
		t.Parallel()

		cc, err := Parse([]byte("no-cache=\"Set-Cookie, X-Custom\""))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		fields := cc.NoCacheFieldNames()
		want := []string{"Set-Cookie", "X-Custom"}
		if !reflect.DeepEqual(fields, want) {
			t.Fatalf("NoCacheFieldNames() = %v, want %v", fields, want)
		}
	})

	t.Run("NoCacheFieldNames unqualified", func(t *testing.T) {
		t.Parallel()

		cc, err := Parse([]byte("no-cache"))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		fields := cc.NoCacheFieldNames()
		if fields != nil {
			t.Fatalf("NoCacheFieldNames() = %v, want nil", fields)
		}
	})

	t.Run("PrivateFieldNames", func(t *testing.T) {
		t.Parallel()

		cc, err := Parse([]byte("private=\"Set-Cookie, X-Custom\""))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		fields := cc.PrivateFieldNames()
		want := []string{"Set-Cookie", "X-Custom"}
		if !reflect.DeepEqual(fields, want) {
			t.Fatalf("PrivateFieldNames() = %v, want %v", fields, want)
		}
	})

	t.Run("PrivateFieldNames unqualified", func(t *testing.T) {
		t.Parallel()

		cc, err := Parse([]byte("private"))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		fields := cc.PrivateFieldNames()
		if fields != nil {
			t.Fatalf("PrivateFieldNames() = %v, want nil", fields)
		}
	})

	t.Run("first occurrence wins for duplicates", func(t *testing.T) {
		t.Parallel()

		cc, err := Parse([]byte("max-age=100, max-age=200"))
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		val, err := cc.MaxAge()
		if err != nil {
			t.Fatalf("MaxAge() error: %v", err)
		}
		if val != 100 {
			t.Fatalf("MaxAge() = %d, want 100 (first occurrence)", val)
		}
	})
}

// A header that is parsed and written back out should say the same thing, so
// that changing one directive does not mean rebuilding the header by hand.
func TestParseStringRoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		header   string
		expected string
	}{
		{name: "single", header: "no-store", expected: "no-store"},
		{name: "with a value", header: "max-age=3600", expected: "max-age=3600"},
		{
			name:     "several, in order",
			header:   "public, max-age=31356000, immutable",
			expected: "public, max-age=31356000, immutable",
		},
		{name: "spacing is normalized", header: "public,   max-age=60", expected: "public, max-age=60"},
		{name: "quoted value survives", header: `private="set-cookie"`, expected: `private="set-cookie"`},
		{name: "names fold to lower", header: "No-Store, Max-Age=5", expected: "no-store, max-age=5"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := Parse([]byte(testCase.header))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := parsed.String(); got != testCase.expected {
				t.Fatalf("got %q, want %q", got, testCase.expected)
			}
		})
	}
}

func TestParseSetVisibilityRoundTrip(t *testing.T) {
	t.Parallel()

	parsed, err := Parse([]byte("public, max-age=31356000, immutable"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	parsed.SetVisibility(false)
	if got := parsed.String(); got != "private, max-age=31356000, immutable" {
		t.Fatalf("got %q", got)
	}
	if !parsed.Private() || parsed.Public() {
		t.Fatalf("accessors disagree with the written header: private=%t public=%t", parsed.Private(), parsed.Public())
	}
}
