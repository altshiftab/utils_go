package header_validator

import (
	"errors"
	"testing"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/missing_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/interfaces/comparer"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator/setting"
)

var errStub = errors.New("stub comparer error")

type errComparer struct{}

func (errComparer) Compare(_ string) (bool, error) { return false, errStub }

func TestValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		validator  *Validator
		fields     map[string]any
		expectErr  bool
		errorCheck func(t *testing.T, err error)
	}{
		{
			name:      "nil fields returns validation error",
			validator: &Validator{},
			fields:    nil,
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, motmedelErrors.ErrValidationError) {
					t.Errorf("error = %v, want ErrValidationError", err)
				}
				if _, ok := errors.AsType[*nil_error.Error](err); !ok {
					t.Errorf("error = %v, want *nil_error.Error", err)
				}
			},
		},
		{
			name:      "empty fields with no expectations passes",
			validator: &Validator{},
			fields:    map[string]any{},
			expectErr: false,
		},
		{
			name: "matching alg passes",
			validator: &Validator{
				Expected: &ExpectedFields{Alg: comparer.NewEqualComparer("HS256")},
			},
			fields:    map[string]any{"alg": "HS256"},
			expectErr: false,
		},
		{
			name: "alg mismatch fails",
			validator: &Validator{
				Expected: &ExpectedFields{Alg: comparer.NewEqualComparer("HS256")},
			},
			fields:    map[string]any{"alg": "none"},
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
			name: "matching typ passes",
			validator: &Validator{
				Expected: &ExpectedFields{Typ: comparer.NewEqualComparer("JWT")},
			},
			fields:    map[string]any{"typ": "JWT"},
			expectErr: false,
		},
		{
			name: "typ mismatch fails",
			validator: &Validator{
				Expected: &ExpectedFields{Typ: comparer.NewEqualComparer("JWT")},
			},
			fields:    map[string]any{"typ": "none"},
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, motmedelErrors.ErrValidationError) {
					t.Errorf("error = %v, want ErrValidationError", err)
				}
			},
		},
		{
			name: "required field missing fails",
			validator: &Validator{
				Settings: map[string]setting.Setting{"alg": setting.Required},
			},
			fields:    map[string]any{},
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
			name: "skip setting bypasses mismatch",
			validator: &Validator{
				Settings: map[string]setting.Setting{"alg": setting.Skip},
				Expected: &ExpectedFields{Alg: comparer.NewEqualComparer("HS256")},
			},
			fields:    map[string]any{"alg": "none"},
			expectErr: false,
		},
		{
			name:      "non-string alg fails conversion",
			validator: &Validator{Expected: &ExpectedFields{Alg: comparer.NewEqualComparer("HS256")}},
			fields:    map[string]any{"alg": 123},
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, motmedelErrors.ErrConversionNotOk) {
					t.Errorf("error = %v, want ErrConversionNotOk", err)
				}
				if errors.Is(err, motmedelErrors.ErrValidationError) {
					t.Errorf("error = %v, unexpectedly wrapped as ErrValidationError", err)
				}
			},
		},
		{
			name:      "comparer error is returned",
			validator: &Validator{Expected: &ExpectedFields{Alg: errComparer{}}},
			fields:    map[string]any{"alg": "HS256"},
			expectErr: true,
			errorCheck: func(t *testing.T, err error) {
				if !errors.Is(err, errStub) {
					t.Errorf("error = %v, want errStub", err)
				}
			},
		},
		{
			name: "no comparer configured skips comparison",
			validator: &Validator{
				Settings: map[string]setting.Setting{"alg": setting.Required},
			},
			fields:    map[string]any{"alg": "anything"},
			expectErr: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.validator.Validate(testCase.fields)

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
