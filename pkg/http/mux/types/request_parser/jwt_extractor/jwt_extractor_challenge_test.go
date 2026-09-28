package jwt_extractor

import (
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	altshiftCryptoEddsa "github.com/altshiftab/utils_go/pkg/crypto/eddsa"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/token_cookie_extractor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/token_header_extractor"
	jwtAuthenticator "github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/authenticator"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/authenticator/authenticator_config"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator/registered_claims_validator"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator/setting"
)

// TestParse_Challenge runs a real authenticator over real tokens, so that an expired one is
// recognised the way it reaches the extractor in a service: wrapped in the validator's error.
func TestParse_Challenge(t *testing.T) {
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

	testCases := []struct {
		name           string
		tokenExtractor request_parser.RequestParser[string]
		header         [2]string
		wantStatus     int
		wantChallenge  string
	}{
		{
			name:           "expired bearer token",
			tokenExtractor: token_header_extractor.New(),
			header:         [2]string{"Authorization", "Bearer " + mint(time.Now().Add(-time.Minute))},
			wantStatus:     http.StatusUnauthorized,
			wantChallenge:  `Bearer error="invalid_token", error_description="The token has expired."`,
		},
		{
			name:           "bearer token that does not verify",
			tokenExtractor: token_header_extractor.New(),
			header:         [2]string{"Authorization", "Bearer " + mint(time.Now().Add(time.Hour)) + "x"},
			wantStatus:     http.StatusUnauthorized,
			wantChallenge:  `Bearer error="invalid_token", error_description="Invalid token."`,
		},
		{
			name:           "empty bearer token",
			tokenExtractor: token_header_extractor.New(),
			header:         [2]string{"Authorization", "Bearer "},
			wantStatus:     http.StatusUnauthorized,
			wantChallenge:  `Bearer error="invalid_request", error_description="Empty token."`,
		},
		{
			name:           "missing bearer token",
			tokenExtractor: token_header_extractor.New(),
			wantStatus:     http.StatusUnauthorized,
			wantChallenge:  "Bearer",
		},
		{
			name:           "expired cookie token has no challenge",
			tokenExtractor: token_cookie_extractor.New(),
			header:         [2]string{"Cookie", "session=" + mint(time.Now().Add(-time.Minute))},
			wantStatus:     http.StatusUnauthorized,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			parser, err := New(testCase.tokenExtractor, authenticator)
			if err != nil {
				t.Fatalf("new: %v", err)
			}

			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			if testCase.header[0] != "" {
				request.Header.Set(testCase.header[0], testCase.header[1])
			}

			_, responseError := parser.Parse(request)
			if responseError == nil || responseError.ProblemDetail == nil {
				t.Fatalf("expected a refusal, got %#v", responseError)
			}
			if status := responseError.ProblemDetail.Status; status != testCase.wantStatus {
				t.Errorf("status = %d, want %d", status, testCase.wantStatus)
			}

			var got string
			for _, header := range responseError.Headers {
				if header != nil && header.Name == "WWW-Authenticate" {
					got = header.Value
				}
			}
			if got != testCase.wantChallenge {
				t.Errorf("challenge = %q, want %q", got, testCase.wantChallenge)
			}
		})
	}
}
