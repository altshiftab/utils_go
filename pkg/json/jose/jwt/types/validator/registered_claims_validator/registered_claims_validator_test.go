package registered_claims_validator

import (
	"errors"
	"testing"
	"time"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/missing_error"
	"github.com/altshiftab/utils_go/pkg/interfaces/comparer"
	jwtErrors "github.com/altshiftab/utils_go/pkg/json/jose/jwt/errors"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/claim_strings"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/numeric_date"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator/setting"
)

var errStub = errors.New("stub comparer error")

type mockComparer[T comparable] struct {
	result bool
	err    error
}

func (m mockComparer[T]) Compare(_ T) (bool, error) {
	return m.result, m.err
}

func TestValidate(t *testing.T) {
	t.Parallel()

	now := time.Now()
	future := *numeric_date.New(now.Add(time.Hour))
	past := *numeric_date.New(now.Add(-time.Hour))

	testCases := []struct {
		name       string
		validator  *Validator
		claims     map[string]any
		expectErr  bool
		errorCheck func(t *testing.T, err error)
	}{
		{
			name:      "nil claims returns validation error",
			validator: &Validator{},
			claims:    nil,
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, motmedelErrors.ErrValidationError) {
					t.Errorf("error = %v, want ErrValidationError", err)
				}
			},
		},
		{
			name:      "empty claims passes",
			validator: &Validator{},
			claims:    map[string]any{},
			expectErr: false,
		},
		{
			name:      "future exp passes",
			validator: &Validator{},
			claims:    map[string]any{"exp": future},
			expectErr: false,
		},
		{
			name:      "expired exp fails",
			validator: &Validator{},
			claims:    map[string]any{"exp": past},
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, jwtErrors.ErrExpExpired) {
					t.Errorf("error = %v, want ErrExpExpired", err)
				}
				if !errors.Is(err, motmedelErrors.ErrValidationError) {
					t.Errorf("error = %v, want wrapped in ErrValidationError", err)
				}
			},
		},
		{
			name:      "past nbf passes",
			validator: &Validator{},
			claims:    map[string]any{"nbf": past},
			expectErr: false,
		},
		{
			name:      "future nbf fails",
			validator: &Validator{},
			claims:    map[string]any{"nbf": future},
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, jwtErrors.ErrNbfBefore) {
					t.Errorf("error = %v, want ErrNbfBefore", err)
				}
				if !errors.Is(err, motmedelErrors.ErrValidationError) {
					t.Errorf("error = %v, want wrapped in ErrValidationError", err)
				}
			},
		},
		{
			name:      "past iat passes",
			validator: &Validator{},
			claims:    map[string]any{"iat": past},
			expectErr: false,
		},
		{
			name:      "future iat fails",
			validator: &Validator{},
			claims:    map[string]any{"iat": future},
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, jwtErrors.ErrIatBefore) {
					t.Errorf("error = %v, want ErrIatBefore", err)
				}
				if !errors.Is(err, motmedelErrors.ErrValidationError) {
					t.Errorf("error = %v, want wrapped in ErrValidationError", err)
				}
			},
		},
		{
			name: "matching audience passes",
			validator: &Validator{
				Expected: &ExpectedClaims{AudienceComparer: comparer.NewEqualComparer("aud1")},
			},
			claims:    map[string]any{"aud": claim_strings.ClaimStrings{"aud0", "aud1"}},
			expectErr: false,
		},
		{
			name: "non-matching audience fails",
			validator: &Validator{
				Expected: &ExpectedClaims{AudienceComparer: comparer.NewEqualComparer("aud1")},
			},
			claims:    map[string]any{"aud": claim_strings.ClaimStrings{"other"}},
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, motmedelErrors.ErrValidationError) {
					t.Errorf("error = %v, want ErrValidationError", err)
				}
				if _, ok := errors.AsType[*mismatch_error.Error](err); !ok {
					t.Errorf("error = %v, want *mismatch_error.Error", err)
				}
			},
		},
		{
			name: "matching issuer passes",
			validator: &Validator{
				Expected: &ExpectedClaims{IssuerComparer: comparer.NewEqualComparer("issuer")},
			},
			claims:    map[string]any{"iss": "issuer"},
			expectErr: false,
		},
		{
			name: "non-matching issuer fails",
			validator: &Validator{
				Expected: &ExpectedClaims{IssuerComparer: comparer.NewEqualComparer("issuer")},
			},
			claims:    map[string]any{"iss": "wrong"},
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, motmedelErrors.ErrValidationError) {
					t.Errorf("error = %v, want ErrValidationError", err)
				}
			},
		},
		{
			name: "matching subject passes",
			validator: &Validator{
				Expected: &ExpectedClaims{SubjectComparer: comparer.NewEqualComparer("subject")},
			},
			claims:    map[string]any{"sub": "subject"},
			expectErr: false,
		},
		{
			name: "non-matching subject fails",
			validator: &Validator{
				Expected: &ExpectedClaims{SubjectComparer: comparer.NewEqualComparer("subject")},
			},
			claims:    map[string]any{"sub": "wrong"},
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, motmedelErrors.ErrValidationError) {
					t.Errorf("error = %v, want ErrValidationError", err)
				}
			},
		},
		{
			name: "matching id passes",
			validator: &Validator{
				Expected: &ExpectedClaims{IdComparer: comparer.NewEqualComparer("token-id")},
			},
			claims:    map[string]any{"jti": "token-id"},
			expectErr: false,
		},
		{
			name: "non-matching id fails",
			validator: &Validator{
				Expected: &ExpectedClaims{IdComparer: comparer.NewEqualComparer("token-id")},
			},
			claims:    map[string]any{"jti": "wrong"},
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, motmedelErrors.ErrValidationError) {
					t.Errorf("error = %v, want ErrValidationError", err)
				}
			},
		},
		{
			name: "required claim missing fails",
			validator: &Validator{
				Settings: map[string]setting.Setting{"iss": setting.Required},
			},
			claims:    map[string]any{"sub": "subject"},
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, motmedelErrors.ErrValidationError) {
					t.Errorf("error = %v, want ErrValidationError", err)
				}
				if _, ok := errors.AsType[*missing_error.Error](err); !ok {
					t.Errorf("error = %v, want *missing_error.Error", err)
				}
			},
		},
		{
			name: "skip setting bypasses expired exp",
			validator: &Validator{
				Settings: map[string]setting.Setting{"exp": setting.Skip},
			},
			claims:    map[string]any{"exp": past},
			expectErr: false,
		},
		{
			name:      "wrong exp type fails conversion",
			validator: &Validator{},
			claims:    map[string]any{"exp": "not-a-date"},
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, motmedelErrors.ErrValidationError) {
					t.Errorf("error = %v, want ErrValidationError", err)
				}
				if !errors.Is(err, motmedelErrors.ErrConversionNotOk) {
					t.Errorf("error = %v, want ErrConversionNotOk", err)
				}
			},
		},
		{
			name: "other comparer matches",
			validator: &Validator{
				Expected: &ExpectedClaims{
					OtherComparers: map[string]comparer.Comparer[any]{
						"custom": mockComparer[any]{result: true},
					},
				},
			},
			claims:    map[string]any{"custom": "value"},
			expectErr: false,
		},
		{
			name: "other comparer rejects",
			validator: &Validator{
				Expected: &ExpectedClaims{
					OtherComparers: map[string]comparer.Comparer[any]{
						"custom": mockComparer[any]{result: false},
					},
				},
			},
			claims:    map[string]any{"custom": "value"},
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, motmedelErrors.ErrValidationError) {
					t.Errorf("error = %v, want ErrValidationError", err)
				}
			},
		},
		{
			name: "comparer error is returned",
			validator: &Validator{
				Expected: &ExpectedClaims{IssuerComparer: mockComparer[string]{err: errStub}},
			},
			claims:    map[string]any{"iss": "issuer"},
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, errStub) {
					t.Errorf("error = %v, want errStub", err)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.validator.Validate(testCase.claims)

			if testCase.expectErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if testCase.errorCheck != nil {
					testCase.errorCheck(t, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
