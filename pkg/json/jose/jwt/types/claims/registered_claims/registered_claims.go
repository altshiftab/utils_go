package registered_claims

import (
	"fmt"
	"maps"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/claim_strings"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/numeric_date"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type Claims struct {
	// the `iss` (Issuer) claim. See https://datatracker.ietf.org/doc/html/rfc7519#section-4.1.1
	Issuer string `json:"iss,omitempty"`

	// the `sub` (Subject) claim. See https://datatracker.ietf.org/doc/html/rfc7519#section-4.1.2
	Subject string `json:"sub,omitempty"`

	// the `aud` (Audience) claim. See https://datatracker.ietf.org/doc/html/rfc7519#section-4.1.3
	Audience claim_strings.ClaimStrings `json:"aud,omitempty"`

	// the `exp` (Expiration Time) claim. See https://datatracker.ietf.org/doc/html/rfc7519#section-4.1.4
	ExpiresAt *numeric_date.Date `json:"exp,omitempty"`

	// the `nbf` (Not Before) claim. See https://datatracker.ietf.org/doc/html/rfc7519#section-4.1.5
	NotBefore *numeric_date.Date `json:"nbf,omitempty"`

	// the `iat` (Issued At) claim. See https://datatracker.ietf.org/doc/html/rfc7519#section-4.1.6
	IssuedAt *numeric_date.Date `json:"iat,omitempty"`

	// the `jti` (JWT ID) claim. See https://datatracker.ietf.org/doc/html/rfc7519#section-4.1.7
	Id string `json:"jti,omitempty"`
}

func New(m map[string]any) (*Claims, error) {
	if len(m) == 0 {
		return nil, nil
	}

	registeredClaims := &Claims{}

	if v, ok := m["iss"]; ok && v != nil {
		vs, err := utils.Convert[string](v)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("convert (iss): %w", err), v)
		}
		registeredClaims.Issuer = vs
	}

	if v, ok := m["sub"]; ok && v != nil {
		vs, err := utils.Convert[string](v)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("convert (sub): %w", err), v)
		}
		registeredClaims.Subject = vs
	}

	if v, ok := m["aud"]; ok && v != nil {
		aud, err := claim_strings.Convert(v)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("claims strings convert: %w", err), v)
		}
		registeredClaims.Audience = aud
	}

	if v, ok := m["exp"]; ok {
		if v == nil {
			registeredClaims.ExpiresAt = nil
		} else {
			numericDate, err := numeric_date.Convert(v)
			if err != nil {
				return nil, altshiftErrors.New(fmt.Errorf("get numeric date (exp): %w", err), v)
			}
			registeredClaims.ExpiresAt = numericDate
		}
	}

	if v, ok := m["nbf"]; ok {
		if v == nil {
			registeredClaims.NotBefore = nil
		} else {
			numericDate, err := numeric_date.Convert(v)
			if err != nil {
				return nil, altshiftErrors.New(fmt.Errorf("get numeric date (nbf): %w", err), v)
			}
			registeredClaims.NotBefore = numericDate
		}
	}

	if v, ok := m["iat"]; ok {
		if v == nil {
			registeredClaims.IssuedAt = nil
		} else {
			numericDate, err := numeric_date.Convert(v)
			if err != nil {
				return nil, altshiftErrors.New(fmt.Errorf("get numeric date (iat): %w", err), v)
			}
			registeredClaims.IssuedAt = numericDate
		}
	}

	if v, ok := m["jti"]; ok {
		vs, err := utils.Convert[string](v)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("convert (jti): %w", err), v)
		}
		registeredClaims.Id = vs
	}

	return registeredClaims, nil
}

type ParsedClaims map[string]any

func NewParsedClaims(claimsMap map[string]any) (ParsedClaims, error) {
	if claimsMap == nil {
		return nil, nil
	}

	clone := maps.Clone(claimsMap)
	if clone == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.NewWithInstance("map", "claims map clone"))
	}

	for key, value := range claimsMap {
		switch key {
		case "exp", "nbf", "iat":
			numericDate, err := numeric_date.Convert(value)
			if err != nil {
				return nil, altshiftErrors.New(fmt.Errorf("parse numeric date (%s): %w", key, err), value)
			}
			if numericDate == nil {
				return nil, altshiftErrors.NewWithTrace(nil_error.New("numeric date"))
			}
			clone[key] = *numericDate
		case "aud":
			claimsString, err := claim_strings.Convert(value)
			if err != nil {
				return nil, altshiftErrors.New(fmt.Errorf("parse claim string (%s): %w", key, err), value)
			}
			clone[key] = claimsString
		}
	}

	return clone, nil
}
