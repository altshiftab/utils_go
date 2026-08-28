// Package iap_request_parser admits a caller that Identity-Aware Proxy has already admitted, by
// verifying the assertion IAP signs rather than trusting that IAP was passed at all.
//
// Why a service behind IAP checks anyway: IAP enforces at the load balancer, so it protects the
// path through the load balancer and nothing else. A backend reachable by any other route -- its
// own run.app address, its instance address, a second load balancer pointed at the same backend
// service, anything already inside the VPC -- is reachable without IAP having seen the request.
//
// The plain headers IAP adds are the trap. X-Goog-Authenticated-User-Email and its siblings are
// unsigned, so anything that can reach the backend directly can set them to whatever it likes, and
// a service reading them is trusting its own attacker. The assertion header this reads is signed by
// Google, which is what makes it evidence rather than assertion.
//
// So a service that restricts its ingress to the load balancer is not relying on this; it is
// defended twice, which is what leaves an ingress setting changed by accident an inconvenience
// rather than an exposure.
package iap_request_parser

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/iap_request_parser/iap_request_parser_config"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	requestParserAdapter "github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/adapter"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/jwt_extractor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/token_header_extractor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/token_header_extractor/token_header_extractor_config"
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

// JwkUrlString is where the keys IAP signs its assertions with are published. They are elliptic
// curve keys, not the RSA ones Google signs ID tokens with, and they are published somewhere else.
const JwkUrlString = "https://www.gstatic.com/iap/verify/public_key-jwk"

// HeaderName is the header IAP puts its assertion in. It is a header of its own rather than
// Authorization, so that a request may carry both this and whatever the service's own callers use.
const HeaderName = "x-goog-iap-jwt-assertion"

// Issuer is what IAP calls itself in the iss claim.
const Issuer = "https://cloud.google.com/iap"

// BackendServiceAudience is the audience of a service behind a load balancer.
//
// The project number is the numeric one rather than the project id: IAP mints the assertion with
// what the resource is called internally, and an audience built from the id never matches.
func BackendServiceAudience(projectNumber string, backendServiceId string) string {
	return fmt.Sprintf("/projects/%s/global/backendServices/%s", projectNumber, backendServiceId)
}

// AppEngineAudience is the audience of an App Engine application behind IAP.
func AppEngineAudience(projectNumber string, projectId string) string {
	return fmt.Sprintf("/projects/%s/apps/%s", projectNumber, projectId)
}

// MakeClaimsValidator validates the claims of an IAP assertion: not expired, IAP as the issuer,
// this resource as the audience, and -- where a deployment named any -- an account it admits.
func MakeClaimsValidator(
	audience string,
	allowedEmails []string,
	allowedHostedDomains []string,
) validator.Validator[map[string]any] {
	return validator.New(func(claims map[string]any) error {
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
		if issuer != Issuer {
			return altshiftErrors.NewWithTrace(mismatch_error.New("iss", issuer, Issuer))
		}

		// Every deployment behind IAP is issued assertions by the same IAP, signed with the same
		// keys. The audience is the only thing that says this one was minted for this service.
		tokenAudience, err := claim_strings.Convert(claims["aud"])
		if err != nil {
			return altshiftErrors.New(fmt.Errorf("claim strings convert (aud): %w", err))
		}
		if !slices.Contains(tokenAudience, audience) {
			return altshiftErrors.NewWithTrace(mismatch_error.New("aud", tokenAudience, audience))
		}

		// The subject is what identifies the caller across everything IAP issues; an assertion
		// without one identifies nobody.
		subject, err := utils.MapGetConvert[string](claims, "sub")
		if err != nil {
			return altshiftErrors.New(fmt.Errorf("map get convert (sub): %w", err))
		}
		if subject == "" {
			return altshiftErrors.NewWithTrace(empty_error.New("sub"))
		}

		if len(allowedEmails) != 0 || len(allowedHostedDomains) != 0 {
			email, err := utils.MapGetConvert[string](claims, "email")
			if err != nil {
				return altshiftErrors.New(fmt.Errorf("map get convert (email): %w", err))
			}

			if err := checkAccount(email, claims, allowedEmails, allowedHostedDomains); err != nil {
				return err
			}
		}

		return nil
	})
}

// checkAccount reports whether the account is one the deployment named.
//
// Either list admitting it is enough: naming a domain and an address outside it is how a deployment
// says "everyone here, and this one guest".
func checkAccount(
	email string,
	claims map[string]any,
	allowedEmails []string,
	allowedHostedDomains []string,
) error {
	if slices.Contains(allowedEmails, strings.ToLower(email)) {
		return nil
	}

	if len(allowedHostedDomains) != 0 {
		// A consumer account carries no hd claim at all, which is how naming a domain also turns
		// away personal accounts rather than admitting them for want of a claim to check.
		hostedDomain, err := utils.MapGetConvert[string](claims, "hd")
		if err == nil && slices.Contains(allowedHostedDomains, strings.ToLower(hostedDomain)) {
			return nil
		}
	}

	return altshiftErrors.NewWithTrace(
		mismatch_error.New("email", email, append(slices.Clone(allowedEmails), allowedHostedDomains...)),
	)
}

// New makes the request parser.
//
// An audience is required rather than defaulted: without one this admits an assertion minted for
// any service IAP fronts, which is every service behind IAP anywhere.
func New(options ...iap_request_parser_config.Option) (request_parser.RequestParser[any], error) {
	config := iap_request_parser_config.New(options...)

	if config.Audience == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("audience"))
	}

	jwkUrl := config.JwkUrl
	if jwkUrl == nil {
		iapJwkUrl, err := JwkUrl()
		if err != nil {
			return nil, fmt.Errorf("jwk url: %w", err)
		}

		jwkUrl = iapJwkUrl
	}

	keyHandler, err := key_handler.New(jwkUrl)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("key handler new: %w", err), jwkUrl)
	}

	authenticator, err := jwtAuthenticator.NewWithKeyHandler(
		keyHandler,
		authenticator_with_key_handler_config.WithClaimsValidator(
			MakeClaimsValidator(config.Audience, lowered(config.AllowedEmails), lowered(config.AllowedHostedDomains)),
		),
	)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("authenticator new with key handler: %w", err))
	}

	return requestParserAdapter.New(
		&jwt_extractor.Parser[*token_header_extractor.Parser]{
			// IAP's own header, and no prefix: the assertion is the whole of the value rather than
			// a bearer token.
			TokenExtractor: token_header_extractor.New(
				token_header_extractor_config.WithHeaderName(HeaderName),
				token_header_extractor_config.WithHeaderValuePrefix(""),
			),
			Authenticators: []interfacesAuthenticator.Authenticator[*authenticated_token.Token, string]{
				authenticator,
			},
		},
	), nil
}

// lowered returns the values folded, so that an address written in another case still matches.
func lowered(values []string) []string {
	folded := make([]string, 0, len(values))
	for _, value := range values {
		folded = append(folded, strings.ToLower(value))
	}

	return folded
}

// JwkUrl is JwkUrlString parsed, which is what a parser uses when given no other.
func JwkUrl() (*url.URL, error) {
	jwkUrl, err := url.Parse(JwkUrlString)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("url parse (jwk url): %w", err), JwkUrlString)
	}

	return jwkUrl, nil
}
