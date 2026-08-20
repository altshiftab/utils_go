package jwt_extractor

import (
	"errors"
	"net/http"
	"sync"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	altshiftCryptoErrors "github.com/altshiftab/utils_go/pkg/crypto/errors"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	muxResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
	authenticatorPkg "github.com/altshiftab/utils_go/pkg/interfaces/authenticator"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token/authenticated_token"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type Parser[T request_parser.RequestParser[string]] struct {
	TokenExtractor T
	Authenticators []authenticatorPkg.Authenticator[*authenticated_token.Token, string]
}

func (p *Parser[T]) Parse(request *http.Request) (*authenticated_token.Token, *muxResponseError.ResponseError) {
	if request == nil {
		return nil, &muxResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request")),
		}
	}

	tokenExtractor := p.TokenExtractor
	if utils.IsNil(tokenExtractor) {
		return nil, &muxResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(
				nil_error.NewWithInstance("request parser", "token extractor"),
			),
		}
	}

	tokenString, responseError := tokenExtractor.Parse(request)
	if responseError != nil {
		return nil, responseError
	}
	if tokenString == "" {
		return nil, &muxResponseError.ResponseError{
			ProblemDetail: problem_detail.New(
				http.StatusUnauthorized,
				problem_detail_config.WithDetail("Empty token."),
			),
		}
	}

	var authenticatedToken *authenticated_token.Token
	var waitGroup sync.WaitGroup

	authenticatorErrs := make([]error, len(p.Authenticators))
	for i, authenticator := range p.Authenticators {
		if utils.IsNil(authenticator) {
			continue
		}

		waitGroup.Go(
			func() {
				token, err := authenticator.Authenticate(request.Context(), tokenString)
				if err != nil {
					authenticatorErrs[i] = err
					return
				}

				authenticatedToken = token
			},
		)
	}

	waitGroup.Wait()

	if authenticatedToken != nil {
		return authenticatedToken, nil
	}

	for _, err := range authenticatorErrs {
		if err == nil {
			continue
		}

		if e, ok := errors.AsType[*mismatch_error.Error](err); ok && e.Field == "sub" {
			return nil, &muxResponseError.ResponseError{
				ClientError: err,
				ProblemDetail: problem_detail.New(
					http.StatusForbidden,
					problem_detail_config.WithDetail("The subject is not allowed to access this resource."),
				),
			}
		} else if altshiftErrors.IsAny(
			err,
			altshiftCryptoErrors.ErrSignatureMismatch,
			altshiftErrors.ErrValidationError,
			// A token that will not parse is a token the client sent wrong, the
			// same as one that will not verify. Left out, it falls through to
			// the server error below and is answered 500 -- so anything at all
			// in an Authorization header, `Bearer x` included, is a way to make
			// the service look broken.
			altshiftErrors.ErrParseError,
		) {
			return nil, &muxResponseError.ResponseError{
				ClientError: err,
				ProblemDetail: problem_detail.New(
					http.StatusUnauthorized,
					problem_detail_config.WithDetail("Invalid token."),
				),
			}
		}
	}

	return nil, &muxResponseError.ResponseError{ServerError: errors.Join(authenticatorErrs...)}
}

func New[T request_parser.RequestParser[string]](
	tokenExtractor T,
	authenticators ...authenticatorPkg.Authenticator[*authenticated_token.Token, string],
) (*Parser[T], error) {
	if utils.IsNil(tokenExtractor) {
		return nil, altshiftErrors.NewWithTrace(nil_error.NewWithInstance("request parser", "token extractor"))
	}

	return &Parser[T]{TokenExtractor: tokenExtractor, Authenticators: authenticators}, nil
}
