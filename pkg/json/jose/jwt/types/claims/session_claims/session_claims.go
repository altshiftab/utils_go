package session_claims

import (
	"fmt"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/claims/registered_claims"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/numeric_date"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type Claims struct {
	registered_claims.Claims

	AuthenticationMethods []string           `json:"amr,omitempty"`
	AuthenticatedAt       *numeric_date.Date `json:"auth_time,omitempty"`
	AuthorizedParty       string             `json:"azp,omitempty"`
	Roles                 []string           `json:"roles,omitempty"`
}

func New(m map[string]any) (*Claims, error) {
	registeredClaims, err := registered_claims.New(m)
	if err != nil {
		return nil, fmt.Errorf("registered claims new: %w", err)
	}
	if registeredClaims == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("registered claims"))
	}

	sessionClaims := &Claims{Claims: *registeredClaims}

	if v, ok := m["amr"]; ok && v != nil {
		vs, err := utils.ConvertSlice[string](v)
		if err != nil {
			return nil, motmedelErrors.NewWithTrace(fmt.Errorf("convert slice (amr): %w", err), v)
		}
		sessionClaims.AuthenticationMethods = vs
	}

	if v, ok := m["auth_time"]; ok && v != nil {
		numericDate, err := numeric_date.Convert(v)
		if err != nil {
			return nil, motmedelErrors.New(fmt.Errorf("numeric date convert (auth_time): %w", err), v)
		}
		sessionClaims.AuthenticatedAt = numericDate
	}

	if v, ok := m["azp"]; ok && v != nil {
		vs, err := utils.Convert[string](v)
		if err != nil {
			return nil, motmedelErrors.NewWithTrace(fmt.Errorf("convert (azp): %w", err), v)
		}
		sessionClaims.AuthorizedParty = vs
	}

	if v, ok := m["roles"]; ok && v != nil {
		vs, err := utils.ConvertSlice[string](v)
		if err != nil {
			return nil, motmedelErrors.NewWithTrace(fmt.Errorf("convert slice (roles): %w", err), v)
		}
		sessionClaims.Roles = vs
	}

	return sessionClaims, nil
}

type ParsedClaims map[string]any

func NewParsedClaims(claimsMap map[string]any) (ParsedClaims, error) {
	if claimsMap == nil {
		return nil, nil
	}

	parsedRegisteredClaims, err := registered_claims.NewParsedClaims(claimsMap)
	if err != nil {
		return nil, fmt.Errorf("registered claims new parsed claims: %w", err)
	}
	if parsedRegisteredClaims == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("parsed registered claims"))
	}

	if v, ok := parsedRegisteredClaims["auth_time"]; ok && v != nil {
		numericDate, err := numeric_date.Convert(v)
		if err != nil {
			return nil, motmedelErrors.New(fmt.Errorf("numeric date convert (auth_time): %w", err), v)
		}
		if numericDate == nil {
			return nil, motmedelErrors.NewWithTrace(nil_error.New("numeric date"))
		}
		parsedRegisteredClaims["auth_time"] = *numericDate
	}

	return ParsedClaims(parsedRegisteredClaims), nil
}
