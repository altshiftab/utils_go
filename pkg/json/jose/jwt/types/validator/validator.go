package validator

import (
	"fmt"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/interfaces/validator"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/claims/registered_claims"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/claims/session_claims"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type Validator struct {
	HeaderValidator  validator.Validator[map[string]any]
	PayloadValidator validator.Validator[map[string]any]
}

func (v *Validator) Validate(token *token.Token) error {
	if token == nil {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, nil_error.New("jwt token"))
	}

	tokenHeader := token.Header
	if headerValidator := v.HeaderValidator; !utils.IsNil(headerValidator) {
		if err := headerValidator.Validate(tokenHeader); err != nil {
			return altshiftErrors.New(fmt.Errorf("header validator validate: %w", err), tokenHeader)
		}
	}

	if payloadValidator := v.PayloadValidator; !utils.IsNil(payloadValidator) {
		tokenPayload := token.Payload

		// Make sure the payload is parsed (both registered and session claims)

		parsedRegisteredClaims, err := registered_claims.NewParsedClaims(tokenPayload)
		if err != nil {
			return altshiftErrors.New(
				fmt.Errorf("%w: registered claims new parsed claims: %w", altshiftErrors.ErrParseError, err),
				tokenPayload,
			)
		}

		parsedSessionClaims, err := session_claims.NewParsedClaims(parsedRegisteredClaims)
		if err != nil {
			return altshiftErrors.New(fmt.Errorf("sessions claims new parsed claims: %w", err), parsedRegisteredClaims)
		}

		if err := payloadValidator.Validate(parsedSessionClaims); err != nil {
			return altshiftErrors.New(fmt.Errorf("payload validator validate: %w", err), parsedSessionClaims)
		}
	}

	return nil
}
