package jwt

import (
	"time"

	altshiftJwtErrors "github.com/altshiftab/utils_go/pkg/json/jose/jwt/errors"
)

func ValidateExpiresAt(expiresAt time.Time, cmp time.Time) error {
	if cmp.After(expiresAt) {
		return altshiftJwtErrors.ErrExpExpired
	}
	return nil
}

func ValidateNotBefore(notBefore time.Time, cmp time.Time) error {
	if cmp.Before(notBefore) {
		return altshiftJwtErrors.ErrNbfBefore
	}
	return nil
}

func ValidateIssuedAt(issuedAt time.Time, cmp time.Time) error {
	if cmp.Before(issuedAt) {
		return altshiftJwtErrors.ErrIatBefore
	}
	return nil
}
