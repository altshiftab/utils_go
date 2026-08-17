package jwt

import (
	"errors"
	"testing"
	"time"

	jwtErrors "github.com/altshiftab/utils_go/pkg/json/jose/jwt/errors"
)

func TestValidateExpiresAt(t *testing.T) {
	t.Parallel()

	now := time.Now()

	testCases := []struct {
		name        string
		expiresAt   time.Time
		cmp         time.Time
		expectError error
	}{
		{
			name:        "not expired",
			expiresAt:   now.Add(time.Hour),
			cmp:         now,
			expectError: nil,
		},
		{
			name:        "expired",
			expiresAt:   now.Add(-time.Hour),
			cmp:         now,
			expectError: jwtErrors.ErrExpExpired,
		},
		{
			name:        "exactly at expiration",
			expiresAt:   now,
			cmp:         now,
			expectError: nil,
		},
		{
			name:        "one nanosecond after expiration",
			expiresAt:   now,
			cmp:         now.Add(time.Nanosecond),
			expectError: jwtErrors.ErrExpExpired,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateExpiresAt(tc.expiresAt, tc.cmp)

			if tc.expectError == nil {
				if err != nil {
					t.Fatalf("expected no error but got: %v", err)
				}
			} else {
				if !errors.Is(err, tc.expectError) {
					t.Fatalf("expected error %v, got: %v", tc.expectError, err)
				}
			}
		})
	}
}

// runNotBeforeStyleTest exercises a validator that rejects claim times lying
// after the comparison time, as ValidateNotBefore and ValidateIssuedAt both do.
func runNotBeforeStyleTest(t *testing.T, validate func(claimTime time.Time, cmp time.Time) error, rejectionError error) {
	t.Helper()

	now := time.Now()

	testCases := []struct {
		name        string
		claimTime   time.Time
		cmp         time.Time
		expectError error
	}{
		{
			name:        "after claim time",
			claimTime:   now.Add(-time.Hour),
			cmp:         now,
			expectError: nil,
		},
		{
			name:        "before claim time",
			claimTime:   now.Add(time.Hour),
			cmp:         now,
			expectError: rejectionError,
		},
		{
			name:        "exactly at claim time",
			claimTime:   now,
			cmp:         now,
			expectError: nil,
		},
		{
			name:        "one nanosecond before claim time",
			claimTime:   now,
			cmp:         now.Add(-time.Nanosecond),
			expectError: rejectionError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validate(tc.claimTime, tc.cmp)

			if tc.expectError == nil {
				if err != nil {
					t.Fatalf("expected no error but got: %v", err)
				}
			} else {
				if !errors.Is(err, tc.expectError) {
					t.Fatalf("expected error %v, got: %v", tc.expectError, err)
				}
			}
		})
	}
}

func TestValidateNotBefore(t *testing.T) {
	t.Parallel()

	runNotBeforeStyleTest(t, ValidateNotBefore, jwtErrors.ErrNbfBefore)
}

func TestValidateIssuedAt(t *testing.T) {
	t.Parallel()

	runNotBeforeStyleTest(t, ValidateIssuedAt, jwtErrors.ErrIatBefore)
}
