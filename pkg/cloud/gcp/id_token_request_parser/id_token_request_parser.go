// Package id_token_request_parser admits a caller that presents a Google-issued ID token minted for
// this service by a service account it names.
//
// It is what gates an endpoint a Google product calls: Cloud Scheduler, Pub/Sub push, Cloud Tasks,
// Eventarc. All of them authenticate the same way -- an OIDC token in the Authorization header --
// so all of them are admitted by the same check, and a service that had written its own would have
// written this.
//
// Three things are checked, and each is load-bearing. That Google signed it, which is what the
// published keys establish. That it was minted for this service, because a token minted for another
// would otherwise be accepted here. And that the caller is one this service admits, because any
// Google account can mint a token for a public endpoint -- the audience says a token was meant for
// this service, not that whoever sent it is entitled to call it.
//
// It lives beside the rest of the Google client rather than beside the other request parsers
// because what it encodes is Google's: the two spellings of the issuer, where the keys are
// published, and which claims carry the caller. It is a package of its own so that importing the
// Google client does not drag in the HTTP machinery.
package id_token_request_parser

import (
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/id_token_request_parser/id_token_request_parser_config"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	requestParserAdapter "github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/adapter"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/jwt_extractor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/token_header_extractor"
	interfacesAuthenticator "github.com/altshiftab/utils_go/pkg/interfaces/authenticator"
	"github.com/altshiftab/utils_go/pkg/interfaces/validator"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key_handler"
	altshiftJwt "github.com/altshiftab/utils_go/pkg/json/jose/jwt"
	jwtAuthenticator "github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/authenticator"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/authenticator/authenticator_with_key_handler_config"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/claim_strings"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/numeric_date"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token/authenticated_token"
	"github.com/altshiftab/utils_go/pkg/utils"
)

// JwkUrlString is where the keys Google signs its ID tokens with are published.
const JwkUrlString = "https://www.googleapis.com/oauth2/v3/certs"

// issuers are the two spellings Google uses for itself in the iss claim. Both are current, and a
// check against one of them turns away half of what Google issues.
var issuers = []string{"accounts.google.com", "https://accounts.google.com"}

// MakeClaimsValidator validates the claims of a Google ID token: not expired, Google as the issuer,
// this service as the audience, and one of the named service accounts as the verified caller.
func MakeClaimsValidator(audience string, serviceAccountEmails []string) validator.Validator[map[string]any] {
	return validator.New(func(claims map[string]any) error {
		// The claims arrive parsed: exp as a numeric date, aud as claim strings.
		expiresAt, err := utils.MapGetConvert[numeric_date.Date](claims, "exp")
		if err != nil {
			return altshiftErrors.New(fmt.Errorf("map get convert (exp): %w", err))
		}
		if err := altshiftJwt.ValidateExpiresAt(expiresAt.Time, time.Now()); err != nil {
			return fmt.Errorf("validate expires at: %w", err)
		}

		issuer, err := utils.MapGetConvert[string](claims, "iss")
		if err != nil {
			return altshiftErrors.New(fmt.Errorf("map get convert (iss): %w", err))
		}
		if !slices.Contains(issuers, issuer) {
			return altshiftErrors.NewWithTrace(mismatch_error.New("iss", issuer, issuers))
		}

		tokenAudience, err := claim_strings.Convert(claims["aud"])
		if err != nil {
			return altshiftErrors.New(fmt.Errorf("claim strings convert (aud): %w", err))
		}
		if !slices.Contains(tokenAudience, audience) {
			return altshiftErrors.NewWithTrace(mismatch_error.New("aud", tokenAudience, audience))
		}

		email, err := utils.MapGetConvert[string](claims, "email")
		if err != nil {
			return altshiftErrors.New(fmt.Errorf("map get convert (email): %w", err))
		}
		if !slices.Contains(serviceAccountEmails, email) {
			return altshiftErrors.NewWithTrace(mismatch_error.New("email", email, serviceAccountEmails))
		}

		// An unverified address is one Google has not established belongs to the account, so a
		// token carrying one says nothing about who the caller is.
		emailVerified, err := utils.MapGetConvert[bool](claims, "email_verified")
		if err != nil {
			return altshiftErrors.New(fmt.Errorf("map get convert (email_verified): %w", err))
		}
		if !emailVerified {
			return altshiftErrors.NewWithTrace(mismatch_error.New("email_verified", emailVerified, true))
		}

		return nil
	})
}

// New makes the request parser.
//
// An audience and at least one service account are required rather than defaulted: a parser missing
// either would admit far more than the caller meant, and there is no safe guess for either.
func New(options ...id_token_request_parser_config.Option) (request_parser.RequestParser[any], error) {
	config := id_token_request_parser_config.New(options...)

	if config.Audience == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("audience"))
	}

	if len(config.ServiceAccountEmails) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("service account emails"))
	}

	for _, email := range config.ServiceAccountEmails {
		if email == "" {
			return nil, altshiftErrors.NewWithTrace(empty_error.New("service account email"))
		}
	}

	jwkUrl := config.JwkUrl
	if jwkUrl == nil {
		googleJwkUrl, err := JwkUrl()
		if err != nil {
			return nil, fmt.Errorf("jwk url: %w", err)
		}

		jwkUrl = googleJwkUrl
	}

	keyHandler, err := key_handler.New(jwkUrl)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("key handler new: %w", err), jwkUrl)
	}

	authenticator, err := jwtAuthenticator.NewWithKeyHandler(
		keyHandler,
		authenticator_with_key_handler_config.WithClaimsValidator(
			MakeClaimsValidator(config.Audience, config.ServiceAccountEmails),
		),
	)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("authenticator new with key handler: %w", err))
	}

	return requestParserAdapter.New(
		&jwt_extractor.Parser[*token_header_extractor.Parser]{
			TokenExtractor: token_header_extractor.New(),
			Authenticators: []interfacesAuthenticator.Authenticator[*authenticated_token.Token, string]{
				authenticator,
			},
		},
	), nil
}

// JwkUrl is JwkUrlString parsed, which is what a parser uses when given no other.
func JwkUrl() (*url.URL, error) {
	jwkUrl, err := url.Parse(JwkUrlString)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("url parse (jwk url): %w", err), JwkUrlString)
	}

	return jwkUrl, nil
}
