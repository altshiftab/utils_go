package authenticator

import (
	"context"
	"fmt"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftJwkErrors "github.com/altshiftab/utils_go/pkg/json/jose/jwk/errors"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key_handler"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/authenticator/authenticator_config"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/authenticator/authenticator_with_key_handler_config"
	altshiftJwkToken "github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token/authenticated_token"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token/authenticated_token/authenticated_token_config"
	altshiftJwtValidator "github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type Authenticator struct {
	config *authenticator_config.Config
}

func (a *Authenticator) Authenticate(ctx context.Context, tokenString string) (*authenticated_token.Token, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	signatureVerifier := a.config.SignatureVerifier
	validator := &altshiftJwtValidator.Validator{
		HeaderValidator:  a.config.HeaderValidator,
		PayloadValidator: a.config.ClaimsValidator,
	}
	token, err := authenticated_token.New(
		tokenString,
		authenticated_token_config.WithSignatureVerifier(signatureVerifier),
		authenticated_token_config.WithTokenValidator(validator),
	)
	if err != nil {
		return nil, altshiftErrors.New(
			fmt.Errorf("authenticated token new: %w", err),
			tokenString, signatureVerifier, validator,
		)
	}
	if token == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("authenticated jwt token"))
	}

	return token, nil
}

func New(options ...authenticator_config.Option) *Authenticator {
	return &Authenticator{
		config: authenticator_config.New(options...),
	}
}

type AuthenticatorWithKeyHandler struct {
	Handler *key_handler.Handler
	config  *authenticator_with_key_handler_config.Config
}

func (a *AuthenticatorWithKeyHandler) Authenticate(ctx context.Context, tokenString string) (*authenticated_token.Token, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	token, err := altshiftJwkToken.New(tokenString)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: token new: %w", altshiftErrors.ErrParseError, err))
	}
	if token == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("jwt token"))
	}

	tokenHeader := token.Header
	if tokenHeader == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("token header"))
	}

	kid, err := utils.MapGetConvert[string](tokenHeader, "kid")
	if err != nil {
		return nil, altshiftErrors.New(
			fmt.Errorf("%w: map get convert (kid): %w", altshiftErrors.ErrValidationError, err),
			tokenHeader,
		)
	}

	keyHandler := a.Handler
	if keyHandler == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("handler"))
	}
	signatureVerifier, err := keyHandler.GetNamedVerifier(ctx, kid)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("handler get named verifier: %w", err), kid)
	}
	if utils.IsNil(signatureVerifier) {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrVerificationError, altshiftJwkErrors.ErrUnknownKeyId),
			kid,
		)
	}

	validator := &altshiftJwtValidator.Validator{
		HeaderValidator:  a.config.HeaderValidator,
		PayloadValidator: a.config.ClaimsValidator,
	}

	authenticatedToken, err := authenticated_token.New(
		tokenString,
		authenticated_token_config.WithSignatureVerifier(signatureVerifier),
		authenticated_token_config.WithTokenValidator(validator),
	)
	if err != nil {
		return nil, altshiftErrors.New(
			fmt.Errorf("authenticated token new: %w", err),
			tokenString, signatureVerifier, validator,
		)
	}
	if authenticatedToken == nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w (parsed)", nil_error.New("authenticated jwt token")))
	}

	return authenticatedToken, nil
}

func NewWithKeyHandler(handler *key_handler.Handler, options ...authenticator_with_key_handler_config.Option) (*AuthenticatorWithKeyHandler, error) {
	if handler == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("handler"))
	}
	return &AuthenticatorWithKeyHandler{
		Handler: handler,
		config:  authenticator_with_key_handler_config.New(options...),
	}, nil
}
