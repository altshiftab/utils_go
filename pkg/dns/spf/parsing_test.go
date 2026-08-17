package spf

import (
	"net"
	"testing"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/testing/cmp"
)

func TestParseSpfRecord(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		input          []byte
		expected       *Record
		expectedErrors []error
	}{
		{
			name:           "empty data",
			input:          nil,
			expected:       nil,
			expectedErrors: []error{motmedelErrors.ErrSyntaxError},
		},
		{
			name:           "syntax error",
			input:          []byte("garbage"),
			expected:       nil,
			expectedErrors: []error{motmedelErrors.ErrSyntaxError},
		},
		{
			name:  "rfc example #1",
			input: []byte("v=spf1 +mx a:colo.example.com/28 -all"),
			expected: &Record{
				Terms: []any{
					&Directive{
						Index:     0,
						Qualifier: "+",
						Mechanism: &Mechanism{
							Label: "mx",
						},
					},
					&Directive{
						Index: 1,
						Mechanism: &Mechanism{
							Label: "a",
							Value: "colo.example.com/28",
						},
					},
					&Directive{
						Index:     2,
						Qualifier: "-",
						Mechanism: &Mechanism{
							Label: "all",
						},
					},
				},
			},
		},
		{
			name:  "rfc example #2",
			input: []byte("v=spf1 +mx redirect=_spf.example.com"),
			expected: &Record{
				Terms: []any{
					&Directive{
						Index:     0,
						Qualifier: "+",
						Mechanism: &Mechanism{Label: "mx"},
					},
					&Modifier{
						Index: 1,
						Label: "redirect",
						Value: "_spf.example.com",
					},
				},
			},
		},
		{
			name:  "rfc example #3",
			input: []byte("v=spf1 ?exists:_h.%{h}._l.%{l}._o.%{o}._i.%{i}._spf.%{d} ?all"),
			expected: &Record{
				Terms: []any{
					&Directive{
						Index:     0,
						Qualifier: "?",
						Mechanism: &Mechanism{
							Label: "exists",
							Value: `_h.%{h}._l.%{l}._o.%{o}._i.%{i}._spf.%{d}`,
						},
					},
					&Directive{
						Index:     1,
						Qualifier: "?",
						Mechanism: &Mechanism{
							Label: "all",
						},
					},
				},
			},
		},
		{
			name:  "rfc example #4",
			input: []byte("v=spf1 mx -all exp=explain._spf.%{d}"),
			expected: &Record{
				Terms: []any{
					&Directive{
						Index:     0,
						Mechanism: &Mechanism{Label: "mx"},
					},
					&Directive{
						Index:     1,
						Qualifier: "-",
						Mechanism: &Mechanism{Label: "all"},
					},
					&Modifier{
						Index: 2,
						Label: "exp",
						Value: `explain._spf.%{d}`,
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			spfRecord, err := ParseSpfRecord(testCase.input)
			expectedErrors := testCase.expectedErrors
			if !motmedelErrors.IsAll(err, expectedErrors...) {
				t.Fatalf("expected errors: %v, got: %v", expectedErrors, err)
			}

			expected := testCase.expected
			if expected != nil {
				expected.Raw = string(testCase.input)
			}

			if diff := cmp.Diff(expected, spfRecord); diff != "" {
				t.Fatalf("struct mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func makeRecord(terms ...any) *Record {
	return &Record{Terms: terms}
}

func TestExtractIncludeValues(t *testing.T) {
	t.Parallel()

	t.Run("nil record returns nil", func(t *testing.T) {
		t.Parallel()
		if got := ExtractIncludeValues(nil); got != nil {
			t.Fatalf("got %v want nil", got)
		}
	})

	t.Run("collects include directives regardless of qualifier", func(t *testing.T) {
		t.Parallel()

		record := makeRecord(
			&Directive{Index: 0, Qualifier: "+", Mechanism: &Mechanism{Label: "include", Value: "a.example"}},
			&Directive{Index: 1, Qualifier: "-", Mechanism: &Mechanism{Label: "include", Value: "b.example"}},
			&Directive{Index: 2, Qualifier: "?", Mechanism: &Mechanism{Label: "INCLUDE", Value: "c.example"}},
			&Directive{Index: 3, Qualifier: "+", Mechanism: &Mechanism{Label: "mx"}},
			&Modifier{Index: 4, Label: "include", Value: "ignored.example"},
		)

		got := ExtractIncludeValues(record)
		want := []string{"a.example", "b.example", "c.example"}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestExtractRedirectValues(t *testing.T) {
	t.Parallel()

	t.Run("nil record returns nil", func(t *testing.T) {
		t.Parallel()
		if got := ExtractRedirectValues(nil); got != nil {
			t.Fatalf("got %v want nil", got)
		}
	})

	t.Run("collects redirect modifiers ignoring directives and other labels", func(t *testing.T) {
		t.Parallel()

		record := makeRecord(
			&Modifier{Index: 0, Label: "redirect", Value: "_spf1.example"},
			&Modifier{Index: 1, Label: "REDIRECT", Value: "_spf2.example"},
			&Modifier{Index: 2, Label: "exp", Value: "explain"},
			&Directive{Index: 3, Qualifier: "+", Mechanism: &Mechanism{Label: "redirect", Value: "ignored"}},
		)

		got := ExtractRedirectValues(record)
		want := []string{"_spf1.example", "_spf2.example"}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestExtractNetworks(t *testing.T) {
	t.Parallel()

	t.Run("nil record returns nil", func(t *testing.T) {
		t.Parallel()
		if got := ExtractNetworks(nil, false); got != nil {
			t.Fatalf("got %v want nil", got)
		}
	})

	t.Run("returns ip4 and ip6 networks", func(t *testing.T) {
		t.Parallel()

		record := makeRecord(
			&Directive{Index: 0, Qualifier: "+", Mechanism: &Mechanism{Label: "ip4", Value: "192.0.2.0/24"}},
			&Directive{Index: 1, Qualifier: "-", Mechanism: &Mechanism{Label: "ip4", Value: "198.51.100.5"}},
			&Directive{Index: 2, Qualifier: "+", Mechanism: &Mechanism{Label: "IP6", Value: "2001:db8::/32"}},
			&Directive{Index: 3, Qualifier: "+", Mechanism: &Mechanism{Label: "mx"}},
		)

		got := ExtractNetworks(record, false)
		var gotStrings []string
		for _, n := range got {
			gotStrings = append(gotStrings, n.String())
		}
		want := []string{"192.0.2.0/24", "198.51.100.5/32", "2001:db8::/32"}
		if diff := cmp.Diff(want, gotStrings, cmp.EquateEmpty()); diff != "" {
			t.Fatalf("network strings mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("passOnly skips non-pass qualifiers", func(t *testing.T) {
		t.Parallel()

		record := makeRecord(
			&Directive{Index: 0, Qualifier: "+", Mechanism: &Mechanism{Label: "ip4", Value: "192.0.2.0/24"}},
			&Directive{Index: 1, Qualifier: "-", Mechanism: &Mechanism{Label: "ip4", Value: "198.51.100.0/24"}},
			&Directive{Index: 2, Qualifier: "", Mechanism: &Mechanism{Label: "ip4", Value: "203.0.113.0/24"}},
		)

		got := ExtractNetworks(record, true)
		var gotStrings []string
		for _, n := range got {
			gotStrings = append(gotStrings, n.String())
		}
		want := []string{"192.0.2.0/24", "203.0.113.0/24"}
		if diff := cmp.Diff(want, gotStrings); diff != "" {
			t.Fatalf("passOnly network strings mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("invalid network strings are dropped", func(t *testing.T) {
		t.Parallel()

		record := makeRecord(
			&Directive{Index: 0, Qualifier: "+", Mechanism: &Mechanism{Label: "ip4", Value: "garbage"}},
			&Directive{Index: 1, Qualifier: "+", Mechanism: &Mechanism{Label: "ip4", Value: "192.0.2.0/24"}},
		)

		got := ExtractNetworks(record, false)
		if len(got) != 1 {
			t.Fatalf("got %d networks want 1", len(got))
		}
		if _, ok := any(got[0]).(*net.IPNet); !ok {
			t.Fatalf("expected *net.IPNet, got %T", got[0])
		}
		if got[0].String() != "192.0.2.0/24" {
			t.Fatalf("got %q want %q", got[0].String(), "192.0.2.0/24")
		}
	})
}
