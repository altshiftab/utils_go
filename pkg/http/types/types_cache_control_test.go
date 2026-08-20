package types

import (
	"errors"
	"testing"
)

func cacheControl(directives ...*CacheControlDirective) *CacheControl {
	return &CacheControl{Directives: directives}
}

func TestCacheControlBoolDirectives(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		directive string
		method    func(*CacheControl) bool
	}{
		{name: "no-cache", directive: "no-cache", method: (*CacheControl).NoCache},
		{name: "no-store", directive: "no-store", method: (*CacheControl).NoStore},
		{name: "no-transform", directive: "no-transform", method: (*CacheControl).NoTransform},
		{name: "only-if-cached", directive: "only-if-cached", method: (*CacheControl).OnlyIfCached},
		{name: "must-revalidate", directive: "must-revalidate", method: (*CacheControl).MustRevalidate},
		{name: "must-understand", directive: "must-understand", method: (*CacheControl).MustUnderstand},
		{name: "private", directive: "private", method: (*CacheControl).Private},
		{name: "proxy-revalidate", directive: "proxy-revalidate", method: (*CacheControl).ProxyRevalidate},
		{name: "public", directive: "public", method: (*CacheControl).Public},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			present := cacheControl(&CacheControlDirective{Name: testCase.directive})
			if !testCase.method(present) {
				t.Errorf("%s: method returned false when directive present", testCase.directive)
			}

			absent := cacheControl(&CacheControlDirective{Name: "something-else"})
			if testCase.method(absent) {
				t.Errorf("%s: method returned true when directive absent", testCase.directive)
			}
		})
	}
}

func TestCacheControlDeltaSeconds(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		directive string
		method    func(*CacheControl) (int, error)
	}{
		{name: "max-age", directive: "max-age", method: (*CacheControl).MaxAge},
		{name: "min-fresh", directive: "min-fresh", method: (*CacheControl).MinFresh},
		{name: "s-maxage", directive: "s-maxage", method: (*CacheControl).SMaxAge},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			t.Run("valid", func(t *testing.T) {
				t.Parallel()
				cc := cacheControl(&CacheControlDirective{Name: testCase.directive, Value: "3600"})
				value, err := testCase.method(cc)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if value != 3600 {
					t.Errorf("value = %d, want 3600", value)
				}
			})

			t.Run("missing", func(t *testing.T) {
				t.Parallel()
				cc := cacheControl()
				_, err := testCase.method(cc)
				if !errors.Is(err, ErrDirectiveNotPresent) {
					t.Errorf("err = %v, want ErrDirectiveNotPresent", err)
				}
			})

			t.Run("invalid value", func(t *testing.T) {
				t.Parallel()
				cc := cacheControl(&CacheControlDirective{Name: testCase.directive, Value: "notanumber"})
				_, err := testCase.method(cc)
				if err == nil {
					t.Error("expected error for non-numeric value, got nil")
				}
			})
		})
	}
}

func TestCacheControlMaxStale(t *testing.T) {
	t.Parallel()

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		value, present, err := cacheControl().MaxStale()
		if value != 0 || present {
			t.Errorf("got (%d, %v), want (0, false)", value, present)
		}
		if !errors.Is(err, ErrDirectiveNotPresent) {
			t.Errorf("err = %v, want ErrDirectiveNotPresent", err)
		}
	})

	t.Run("present without value", func(t *testing.T) {
		t.Parallel()
		value, present, err := cacheControl(&CacheControlDirective{Name: "max-stale"}).MaxStale()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if value != 0 || present {
			t.Errorf("got (%d, %v), want (0, false)", value, present)
		}
	})

	t.Run("present with value", func(t *testing.T) {
		t.Parallel()
		value, present, err := cacheControl(&CacheControlDirective{Name: "max-stale", Value: "60"}).MaxStale()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if value != 60 || !present {
			t.Errorf("got (%d, %v), want (60, true)", value, present)
		}
	})

	t.Run("present with invalid value", func(t *testing.T) {
		t.Parallel()
		value, present, err := cacheControl(&CacheControlDirective{Name: "max-stale", Value: "bad"}).MaxStale()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if value != 0 || !present {
			t.Errorf("got (%d, %v), want (0, true)", value, present)
		}
	})
}

func TestCacheControlFieldNames(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		directive string
		method    func(*CacheControl) []string
	}{
		{name: "no-cache", directive: "no-cache", method: (*CacheControl).NoCacheFieldNames},
		{name: "private", directive: "private", method: (*CacheControl).PrivateFieldNames},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			t.Run("with field names", func(t *testing.T) {
				t.Parallel()
				cc := cacheControl(&CacheControlDirective{Name: testCase.directive, Value: "Set-Cookie, Authorization"})
				got := testCase.method(cc)
				if len(got) != 2 || got[0] != "Set-Cookie" || got[1] != "Authorization" {
					t.Errorf("got %v, want [Set-Cookie Authorization]", got)
				}
			})

			t.Run("empty value", func(t *testing.T) {
				t.Parallel()
				cc := cacheControl(&CacheControlDirective{Name: testCase.directive})
				if got := testCase.method(cc); got != nil {
					t.Errorf("got %v, want nil", got)
				}
			})

			t.Run("missing directive", func(t *testing.T) {
				t.Parallel()
				cc := cacheControl()
				if got := testCase.method(cc); got != nil {
					t.Errorf("got %v, want nil", got)
				}
			})
		})
	}
}

func directive(name string, value string) *CacheControlDirective {
	return &CacheControlDirective{Name: name, Value: value}
}

func TestCacheControlString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    *CacheControl
		expected string
	}{
		{name: "nil", input: nil, expected: ""},
		{name: "empty", input: cacheControl(), expected: ""},
		{name: "valueless", input: cacheControl(directive("no-store", "")), expected: "no-store"},
		{name: "token value", input: cacheControl(directive("max-age", "3600")), expected: "max-age=3600"},
		{
			name:     "order is kept",
			input:    cacheControl(directive("public", ""), directive("max-age", "31356000"), directive("immutable", "")),
			expected: "public, max-age=31356000, immutable",
		},
		{
			name:     "a value that is not a token is quoted",
			input:    cacheControl(directive("private", "set-cookie, authorization")),
			expected: `private="set-cookie, authorization"`,
		},
		{
			name:     "a quote inside a value is escaped",
			input:    cacheControl(directive("private", `a"b`)),
			expected: `private="a\"b"`,
		},
		{
			name:     "a nameless directive is dropped rather than written",
			input:    cacheControl(directive("", "x"), directive("no-store", "")),
			expected: "no-store",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := testCase.input.String(); got != testCase.expected {
				t.Fatalf("got %q, want %q", got, testCase.expected)
			}
		})
	}
}

func TestCacheControlSetVisibility(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    *CacheControl
		public   bool
		expected string
	}{
		{
			name:     "public becomes private, the rest untouched",
			input:    cacheControl(directive("public", ""), directive("max-age", "31356000"), directive("immutable", "")),
			public:   false,
			expected: "private, max-age=31356000, immutable",
		},
		{
			name:     "private becomes public, dropping its field names",
			input:    cacheControl(directive("private", "set-cookie"), directive("max-age", "60")),
			public:   true,
			expected: "public, max-age=60",
		},
		{
			name:     "a header stating neither is left stating neither",
			input:    cacheControl(directive("no-store", "")),
			public:   false,
			expected: "no-store",
		},
		{
			name:     "no-cache keeps its meaning",
			input:    cacheControl(directive("public", ""), directive("no-cache", "")),
			public:   false,
			expected: "private, no-cache",
		},
		{
			name:     "already private, asked for private",
			input:    cacheControl(directive("private", ""), directive("max-age", "60")),
			public:   false,
			expected: "private, max-age=60",
		},
		{name: "nil does not panic", input: nil, public: false, expected: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			testCase.input.SetVisibility(testCase.public)
			if got := testCase.input.String(); got != testCase.expected {
				t.Fatalf("got %q, want %q", got, testCase.expected)
			}
		})
	}
}
