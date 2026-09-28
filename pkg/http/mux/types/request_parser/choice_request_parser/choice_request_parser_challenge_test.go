package choice_request_parser

import (
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	altshiftCryptoEddsa "github.com/altshiftab/utils_go/pkg/crypto/eddsa"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/choice_request_parser/choice_request_parser_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/jwt_extractor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/token_cookie_extractor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/token_header_extractor"
	jwtAuthenticator "github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/authenticator"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/authenticator/authenticator_config"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token/authenticated_token"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator/registered_claims_validator"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator/setting"
)

// TestParse_BearerBesideCookie is the arrangement a service offering both a browser session and an
// API key has: a cookie authorizer first, a bearer one second, either admitting. A refusal has to
// speak to the credential that was sent, and carry the bearer challenge whichever is answered.
func TestParse_BearerBesideCookie(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer := &altshiftCryptoEddsa.Method{PrivateKey: privateKey, PublicKey: publicKey}

	mint := func(expiresAt time.Time) string {
		t.Helper()
		tokenString, err := (&token.Token{Payload: map[string]any{"sub": "subject", "exp": expiresAt.Unix()}}).Encode(signer)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		return tokenString
	}

	authenticator := jwtAuthenticator.New(
		authenticator_config.WithSignatureVerifier(signer),
		authenticator_config.WithClaimsValidator(
			&registered_claims_validator.Validator{Settings: map[string]setting.Setting{"exp": setting.Required}},
		),
	)
	cookieParser, err := jwt_extractor.New[request_parser.RequestParser[string]](token_cookie_extractor.New(), authenticator)
	if err != nil {
		t.Fatalf("cookie parser: %v", err)
	}
	bearerParser, err := jwt_extractor.New[request_parser.RequestParser[string]](token_header_extractor.New(), authenticator)
	if err != nil {
		t.Fatalf("bearer parser: %v", err)
	}

	testCases := []struct {
		name           string
		cookie         string
		authorization  string
		wantDetail     string
		wantChallenges []string
	}{
		{
			name:           "nothing sent",
			wantDetail:     "Missing cookie with token.",
			wantChallenges: []string{"Bearer"},
		},
		{
			name:           "a bearer token that does not verify",
			authorization:  "Bearer x.y.z",
			wantDetail:     "Invalid token.",
			wantChallenges: []string{`Bearer error="invalid_token", error_description="Invalid token."`},
		},
		{
			name:           "an expired bearer token",
			authorization:  "Bearer " + mint(time.Now().Add(-time.Minute)),
			wantDetail:     "Invalid token.",
			wantChallenges: []string{`Bearer error="invalid_token", error_description="The token has expired."`},
		},
		{
			name:           "an expired cookie",
			cookie:         mint(time.Now().Add(-time.Minute)),
			wantDetail:     "Invalid token.",
			wantChallenges: []string{"Bearer"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			parser := New(
				[]request_parser.RequestParser[*authenticated_token.Token]{cookieParser, bearerParser},
				choice_request_parser_config.WithExclusive(),
			)

			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			if testCase.cookie != "" {
				request.AddCookie(&http.Cookie{Name: "session", Value: testCase.cookie, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
			}
			if testCase.authorization != "" {
				request.Header.Set("Authorization", testCase.authorization)
			}

			_, responseError := parser.Parse(request)
			if responseError == nil || responseError.ProblemDetail == nil {
				t.Fatalf("expected a refusal, got %#v", responseError)
			}
			if responseError.ProblemDetail.Status != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", responseError.ProblemDetail.Status)
			}
			if responseError.ProblemDetail.Detail != testCase.wantDetail {
				t.Errorf("detail = %q, want %q", responseError.ProblemDetail.Detail, testCase.wantDetail)
			}

			var challenges []string
			for _, header := range responseError.Headers {
				if header != nil && header.Name == "WWW-Authenticate" {
					challenges = append(challenges, header.Value)
				}
			}
			if !slices.Equal(challenges, testCase.wantChallenges) {
				t.Errorf("challenges = %q, want %q", challenges, testCase.wantChallenges)
			}
		})
	}
}
