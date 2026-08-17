package jwt

import (
	"time"

	motmedelJwtErrors "github.com/altshiftab/utils_go/pkg/json/jose/jwt/errors"
)

func ValidateExpiresAt(expiresAt time.Time, cmp time.Time) error {
	if cmp.After(expiresAt) {
		return motmedelJwtErrors.ErrExpExpired
	}
	return nil
}

func ValidateNotBefore(notBefore time.Time, cmp time.Time) error {
	if cmp.Before(notBefore) {
		return motmedelJwtErrors.ErrNbfBefore
	}
	return nil
}

func ValidateIssuedAt(issuedAt time.Time, cmp time.Time) error {
	if cmp.Before(issuedAt) {
		return motmedelJwtErrors.ErrIatBefore
	}
	return nil
}
