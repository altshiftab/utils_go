// Package jwk_jwt_parser assembles a request parser that takes a JWT from a
// header and verifies it against a published JWK set.
//
// It is the shared half of iap_request_parser and oidc_request_parser, which
// differ in where the token comes from, whose keys sign it, and what its claims
// have to say -- and are identical in everything between. What is here is that
// middle: fetch the keys, build an authenticator around them, and wrap it in
// the extractor the mux takes.
//
// The claims validator is required rather than optional, and that is the point
// of the package existing. A JWT validator skips a nil payload validator
// silently, so an assembly that forgets to attach one accepts every token the
// key set signs -- for Google's OAuth2 keys, that is a token minted for anyone,
// by anyone who asked. Requiring it here makes the omission impossible rather
// than merely unlikely, and does it once instead of in each parser.
package jwk_jwt_parser

import (
	"fmt"
	"net/url"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	requestParserAdapter "github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/adapter"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/jwt_extractor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/token_header_extractor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/token_header_extractor/token_header_extractor_config"
	interfacesAuthenticator "github.com/altshiftab/utils_go/pkg/interfaces/authenticator"
	"github.com/altshiftab/utils_go/pkg/interfaces/validator"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key_handler"
	jwtAuthenticator "github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/authenticator"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/authenticator/authenticator_with_key_handler_config"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token/authenticated_token"
	"github.com/altshiftab/utils_go/pkg/utils"
)

// New makes a request parser for JWTs signed with the keys at jwkUrl.
//
// headerValuePrefix is what precedes the token in the header and may be empty:
// a bearer credential is "Bearer " while an assertion in a header of its own is
// the whole value.
//
// claimsValidator decides everything the signature does not -- who the token
// is for, who issued it, whether it has expired, whose identity it carries.
// The signature says only that the key set signed it, and a key set signs for
// everyone it serves.
func New(
	jwkUrl *url.URL,
	headerName string,
	headerValuePrefix string,
	claimsValidator validator.Validator[map[string]any],
) (request_parser.RequestParser[any], error) {
	if jwkUrl == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("jwk url"))
	}

	if headerName == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("header name"))
	}

	// Refused rather than defaulted to "accept anything". A validator left off
	// is not a parser that checks less, it is a parser that checks nothing
	// beyond the signature, and it looks like working code.
	if utils.IsNil(claimsValidator) {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("claims validator"))
	}

	keyHandler, err := key_handler.New(jwkUrl)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("key handler new: %w", err), jwkUrl)
	}

	authenticator, err := jwtAuthenticator.NewWithKeyHandler(
		keyHandler,
		authenticator_with_key_handler_config.WithClaimsValidator(claimsValidator),
	)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("authenticator new with key handler: %w", err))
	}

	return requestParserAdapter.New(
		&jwt_extractor.Parser[*token_header_extractor.Parser]{
			TokenExtractor: token_header_extractor.New(
				token_header_extractor_config.WithHeaderName(headerName),
				token_header_extractor_config.WithHeaderValuePrefix(headerValuePrefix),
			),
			Authenticators: []interfacesAuthenticator.Authenticator[*authenticated_token.Token, string]{
				authenticator,
			},
		},
	), nil
}
