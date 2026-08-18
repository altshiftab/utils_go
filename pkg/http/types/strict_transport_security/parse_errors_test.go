package strict_transport_security

import (
	"errors"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// Parse used to answer a malformed header with a nil policy and no error, which
// left every caller one missing nil check away from a panic. These cases pin the
// replacement contract: a nil error means a policy was returned.
func TestParseReportsMalformedHeaders(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		input          string
		wantSentinel   error
		wantClassifier error
	}{
		{
			name:           "duplicate directive",
			input:          "max-age=31536000; max-age=1",
			wantSentinel:   ErrDuplicateDirective,
			wantClassifier: altshiftErrors.ErrSemanticError,
		},
		{
			name:           "max-age is not a number",
			input:          "max-age=abc",
			wantSentinel:   ErrBadMaxAgeFormat,
			wantClassifier: altshiftErrors.ErrSemanticError,
		},
		{
			name:           "includeSubDomains carries a value",
			input:          "max-age=31536000; includeSubDomains=1",
			wantSentinel:   ErrNonValuelessDirective,
			wantClassifier: altshiftErrors.ErrSemanticError,
		},
		{
			name:           "preload carries a value",
			input:          "max-age=31536000; preload=yes",
			wantSentinel:   ErrNonValuelessDirective,
			wantClassifier: altshiftErrors.ErrSemanticError,
		},
		{
			name:           "the required max-age directive is absent",
			input:          "includeSubDomains",
			wantSentinel:   ErrMissingMaxAge,
			wantClassifier: altshiftErrors.ErrSemanticError,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			policy, err := Parse([]byte(testCase.input))

			if err == nil {
				t.Fatalf("Parse(%q) returned no error; policy = %#v", testCase.input, policy)
			}
			if policy != nil {
				t.Errorf("Parse(%q) returned a policy alongside an error: %#v", testCase.input, policy)
			}
			if !errors.Is(err, testCase.wantSentinel) {
				t.Errorf("error does not wrap %v: %v", testCase.wantSentinel, err)
			}
			// Callers classify a malformed header by the syntax/semantic
			// sentinels rather than by the specific cause.
			if !errors.Is(err, testCase.wantClassifier) {
				t.Errorf("error does not classify as %v: %v", testCase.wantClassifier, err)
			}
		})
	}
}

// Whatever the input, Parse must never answer with a nil policy and a nil error.
func TestParseNeverReturnsNilPolicyWithoutError(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"",
		";",
		"!!!",
		"max-age",
		"max-age=",
		"max-age=31536000;",
		"max-age=31536000; ; includeSubDomains",
		"not-a-directive",
		"max-age=31536000; unknown=1",
		"MAX-AGE=31536000",
		"max-age=0",
		"max-age=31536000; includeSubDomains; preload",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			policy, err := Parse([]byte(input))
			if err == nil && policy == nil {
				t.Errorf("Parse(%q) returned (nil, nil)", input)
			}
		})
	}
}

func TestParseAcceptsAWellFormedPolicy(t *testing.T) {
	t.Parallel()

	policy, err := Parse([]byte("max-age=31536000; includeSubDomains; preload"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if policy == nil {
		t.Fatal("expected a policy, got nil")
	}
	if policy.MaxAge != 31536000 {
		t.Errorf("MaxAge = %d, want 31536000", policy.MaxAge)
	}
	if !policy.IncludeSubdomains {
		t.Error("IncludeSubdomains = false, want true")
	}
	if !policy.Preload {
		t.Error("Preload = false, want true")
	}
}
