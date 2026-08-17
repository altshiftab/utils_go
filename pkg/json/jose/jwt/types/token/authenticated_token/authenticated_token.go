package authenticated_token

import (
	"fmt"

	motmedelCryptoInterfaces "github.com/altshiftab/utils_go/pkg/crypto/interfaces"
	"github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/json/jose/jws/types/jws_object"
	motmedelJwtToken "github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token/authenticated_token/authenticated_token_config"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type Token struct {
	*motmedelJwtToken.Token
	verifier motmedelCryptoInterfaces.NamedVerifier
	raw      string
}

func (token *Token) Raw() string {
	return token.raw
}

func (token *Token) Verifier() motmedelCryptoInterfaces.NamedVerifier {
	return token.verifier
}

func New(tokenString string, options ...authenticated_token_config.Option) (*Token, error) {
	if tokenString == "" {
		return nil, nil
	}

	rawToken, err := jws_object.New(tokenString)
	if err != nil {
		return nil, fmt.Errorf("%w: raw token new: %w", errors.ErrParseError, err)
	}
	if rawToken == nil {
		return nil, errors.NewWithTrace(nil_error.New("raw jwt token"))
	}

	token, err := motmedelJwtToken.NewFromJws(rawToken)
	if err != nil {
		return nil, errors.New(
			fmt.Errorf("%w: token from raw token: %w", errors.ErrParseError, err),
			rawToken,
		)
	}
	if token == nil {
		return nil, errors.NewWithTrace(nil_error.New("jwt token"))
	}

	config := authenticated_token_config.New(options...)
	signatureVerifier := config.SignatureVerifier

	authenticatedToken := &Token{Token: token, raw: tokenString, verifier: signatureVerifier}

	if !utils.IsNil(signatureVerifier) {
		tokenHeader := token.Header
		if tokenHeader == nil {
			return authenticatedToken, errors.NewWithTrace(nil_error.New("token header"))
		}

		alg, err := utils.MapGetConvert[string](tokenHeader, "alg")
		if err != nil {
			var wrappedErr error = errors.New(fmt.Errorf("map get convert: %w", err), tokenHeader)
			if errors.IsAny(err, errors.ErrConversionNotOk, errors.ErrNotInMap) {
				wrappedErr = fmt.Errorf("%w: %w", errors.ErrValidationError, wrappedErr)
			}
			return authenticatedToken, wrappedErr
		}

		verifierMethodName := signatureVerifier.GetName()
		if alg != verifierMethodName {
			return authenticatedToken, errors.NewWithTrace(
				fmt.Errorf("%w: %w", errors.ErrVerificationError, mismatch_error.New("alg", alg, verifierMethodName)),
			)
		}

		if err := rawToken.Verify(signatureVerifier); err != nil {
			return authenticatedToken, errors.New(fmt.Errorf("raw token verify: %w", err), rawToken)
		}
	} else if !config.AllowUnauthenticated {
		return authenticatedToken, errors.NewWithTrace(nil_error.New("verifier"))
	}

	tokenValidator := config.TokenValidator
	if !utils.IsNil(tokenValidator) {
		// TODO: Should I really add `ErrValidationError` here?
		if err := tokenValidator.Validate(token); err != nil {
			return authenticatedToken, errors.New(
				fmt.Errorf("%w: token validator validate: %w", errors.ErrValidationError, err),
				token,
			)
		}
	}

	return authenticatedToken, nil
}
