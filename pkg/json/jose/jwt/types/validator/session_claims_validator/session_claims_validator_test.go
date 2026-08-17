package session_claims_validator

import (
	"errors"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/interfaces/comparer"
)

// mockComparer is a test comparer that returns a predefined result.
type mockComparer[T any] struct {
	result bool
	err    error
}

func (m *mockComparer[T]) Compare(value T) (bool, error) {
	return m.result, m.err
}

func TestValidator_Validate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		validator   *Validator
		claims      map[string]any
		expectError bool
		errorCheck  func(error) bool
	}{
		{
			name:        "nil claims returns error",
			validator:   &Validator{},
			claims:      nil,
			expectError: true,
			errorCheck: func(err error) bool {
				return errors.Is(err, altshiftErrors.ErrValidationError)
			},
		},
		{
			name:        "empty claims passes with no validators",
			validator:   &Validator{},
			claims:      map[string]any{},
			expectError: false,
		},
		{
			name: "valid amr claim with matching comparer",
			validator: &Validator{
				Expected: &ExpectedClaims{
					AuthenticationMethodsComparer: &mockComparer[string]{result: true},
				},
			},
			claims: map[string]any{
				"amr": []any{"pwd", "mfa"},
			},
			expectError: false,
		},
		{
			name: "invalid amr type triggers conversion error and continues (bug fix test)",
			validator: &Validator{
				Expected: &ExpectedClaims{
					AuthenticationMethodsComparer: &mockComparer[string]{result: true},
				},
			},
			claims: map[string]any{
				"amr": 12345, // invalid type - should be []string or []any
			},
			expectError: true,
			errorCheck: func(err error) bool {
				// Should get a validation error containing the conversion error
				// but NOT a mismatch error (the bug was that it would also add mismatch)
				return errors.Is(err, altshiftErrors.ErrValidationError)
			},
		},
		{
			name: "amr claim with no match returns mismatch error",
			validator: &Validator{
				Expected: &ExpectedClaims{
					AuthenticationMethodsComparer: &mockComparer[string]{result: false},
				},
			},
			claims: map[string]any{
				"amr": []any{"pwd"},
			},
			expectError: true,
			errorCheck: func(err error) bool {
				return errors.Is(err, altshiftErrors.ErrValidationError)
			},
		},
		{
			name: "valid roles claim with matching comparer",
			validator: &Validator{
				Expected: &ExpectedClaims{
					RolesComparer: &mockComparer[string]{result: true},
				},
			},
			claims: map[string]any{
				"roles": []any{"admin", "user"},
			},
			expectError: false,
		},
		{
			name: "invalid roles type triggers conversion error and continues (bug fix test)",
			validator: &Validator{
				Expected: &ExpectedClaims{
					RolesComparer: &mockComparer[string]{result: true},
				},
			},
			claims: map[string]any{
				"roles": 12345, // invalid type - should be []string or []any
			},
			expectError: true,
			errorCheck: func(err error) bool {
				// Should get a validation error containing the conversion error
				// but NOT a mismatch error (the bug was that it would also add mismatch)
				return errors.Is(err, altshiftErrors.ErrValidationError)
			},
		},
		{
			name: "roles claim with no match returns mismatch error",
			validator: &Validator{
				Expected: &ExpectedClaims{
					RolesComparer: &mockComparer[string]{result: false},
				},
			},
			claims: map[string]any{
				"roles": []any{"user"},
			},
			expectError: true,
			errorCheck: func(err error) bool {
				return errors.Is(err, altshiftErrors.ErrValidationError)
			},
		},
		{
			name: "valid azp claim with matching comparer",
			validator: &Validator{
				Expected: &ExpectedClaims{
					AuthorizedPartyComparer: &mockComparer[string]{result: true},
				},
			},
			claims: map[string]any{
				"azp": "my-client-id",
			},
			expectError: false,
		},
		{
			name: "azp claim with no match returns mismatch error",
			validator: &Validator{
				Expected: &ExpectedClaims{
					AuthorizedPartyComparer: &mockComparer[string]{result: false},
				},
			},
			claims: map[string]any{
				"azp": "wrong-client-id",
			},
			expectError: true,
			errorCheck: func(err error) bool {
				return errors.Is(err, altshiftErrors.ErrValidationError)
			},
		},
		{
			name: "other comparer validates custom claim",
			validator: &Validator{
				Expected: &ExpectedClaims{
					OtherComparers: map[string]comparer.Comparer[any]{
						"custom_claim": &mockComparer[any]{result: true},
					},
				},
			},
			claims: map[string]any{
				"custom_claim": "some_value",
			},
			expectError: false,
		},
		{
			name: "other comparer rejects non-matching custom claim",
			validator: &Validator{
				Expected: &ExpectedClaims{
					OtherComparers: map[string]comparer.Comparer[any]{
						"custom_claim": &mockComparer[any]{result: false},
					},
				},
			},
			claims: map[string]any{
				"custom_claim": "wrong_value",
			},
			expectError: true,
			errorCheck: func(err error) bool {
				return errors.Is(err, altshiftErrors.ErrValidationError)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.validator.Validate(tc.claims)

			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tc.errorCheck != nil && !tc.errorCheck(err) {
					t.Fatalf("error check failed for error: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error but got: %v", err)
				}
			}
		})
	}
}

// TestAmrConversionErrorContinues specifically tests the bug fix where
// a conversion error for 'amr' should NOT also trigger a mismatch error.
func TestAmrConversionErrorContinues(t *testing.T) {
	t.Parallel()

	// This comparer should never be called if conversion fails
	callCount := 0
	countingComparer := &countingMockComparer[string]{
		callCount: &callCount,
		result:    false, // Would cause mismatch if called
	}

	validator := &Validator{
		Expected: &ExpectedClaims{
			AuthenticationMethodsComparer: countingComparer,
		},
	}

	claims := map[string]any{
		"amr": "not-an-array", // Invalid type
	}

	err := validator.Validate(claims)
	if err == nil {
		t.Fatal("expected error for invalid amr type")
	}

	// The comparer should NOT have been called because conversion failed
	// and we should have continued to the next claim
	if callCount > 0 {
		t.Fatalf("comparer should not have been called after conversion error, but was called %d times", callCount)
	}
}

// TestRolesConversionErrorContinues specifically tests the bug fix where
// a conversion error for 'roles' should NOT also trigger a mismatch error.
func TestRolesConversionErrorContinues(t *testing.T) {
	t.Parallel()

	// This comparer should never be called if conversion fails
	callCount := 0
	countingComparer := &countingMockComparer[string]{
		callCount: &callCount,
		result:    false, // Would cause mismatch if called
	}

	validator := &Validator{
		Expected: &ExpectedClaims{
			RolesComparer: countingComparer,
		},
	}

	claims := map[string]any{
		"roles": "not-an-array", // Invalid type
	}

	err := validator.Validate(claims)
	if err == nil {
		t.Fatal("expected error for invalid roles type")
	}

	// The comparer should NOT have been called because conversion failed
	// and we should have continued to the next claim
	if callCount > 0 {
		t.Fatalf("comparer should not have been called after conversion error, but was called %d times", callCount)
	}
}

type countingMockComparer[T any] struct {
	callCount *int
	result    bool
	err       error
}

func (m *countingMockComparer[T]) Compare(value T) (bool, error) {
	*m.callCount++
	return m.result, m.err
}
