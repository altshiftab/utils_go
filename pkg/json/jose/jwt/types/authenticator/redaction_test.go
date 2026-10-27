package authenticator

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	altshiftCryptoEddsa "github.com/altshiftab/utils_go/pkg/crypto/eddsa"
	"github.com/altshiftab/utils_go/pkg/interfaces/comparer"
	altshiftJws "github.com/altshiftab/utils_go/pkg/json/jose/jws"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/authenticator/authenticator_config"
	altshiftJwtToken "github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator/registered_claims_validator"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator/setting"
	altshiftLog "github.com/altshiftab/utils_go/pkg/log"
)

/*
 * A refused token's error, written out as the log writes it: the token's fingerprint and the claims
 * that decided, and none of the token, its signature, what it says about the person, or the key it
 * was checked with. The verifier holds the private key, as an issuing service's did, because that is
 * how the key reached the log in the first place.
 */
func TestAuthenticator_Authenticate_RefusalWritesNoSecrets(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("ed25519 generate key: %v", err)
	}
	signer := &altshiftCryptoEddsa.Method{PrivateKey: privateKey, PublicKey: publicKey}

	otherPublicKey, otherPrivateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("ed25519 generate key: %v", err)
	}
	otherSigner := &altshiftCryptoEddsa.Method{PrivateKey: otherPrivateKey, PublicKey: otherPublicKey}

	const email = "person@example.com"

	mint := func(method *altshiftCryptoEddsa.Method, changes map[string]any) string {
		t.Helper()

		payload := map[string]any{
			"iss":   "test-issuer",
			"aud":   "test-audience",
			"sub":   "test-subject-id:" + email,
			"email": email,
			"name":  "A Person",
			"exp":   time.Now().Add(time.Hour).Unix(),
		}
		for name, value := range changes {
			payload[name] = value
		}

		tokenString, err := (&altshiftJwtToken.Token{Header: map[string]any{"typ": "JWT"}, Payload: payload}).Encode(method)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		return tokenString
	}

	authenticator := New(
		authenticator_config.WithSignatureVerifier(signer),
		authenticator_config.WithClaimsValidator(&registered_claims_validator.Validator{
			Settings: map[string]setting.Setting{"aud": setting.Required, "exp": setting.Required},
			Expected: &registered_claims_validator.ExpectedClaims{
				AudienceComparer: comparer.NewEqualComparer("test-audience"),
			},
		}),
	)

	// The private key, every way it could be written out.
	seed := privateKey.Seed()
	var byteList []string
	for _, b := range seed[:8] {
		byteList = append(byteList, fmt.Sprintf("%#x", b))
	}
	keyForms := []string{hex.EncodeToString(seed), strings.Join(byteList, ", ")}

	testCases := []struct {
		name  string
		token string
	}{
		{name: "signed by another key", token: mint(otherSigner, nil)},
		{name: "for another audience", token: mint(signer, map[string]any{"aud": "another-audience"})},
		{name: "expired", token: mint(signer, map[string]any{"exp": time.Now().Add(-time.Hour).Unix()})},
		{ //nolint:gosec // A made-up token, not a credential.
			// A signed token whose payload is not JSON, refused before it is ever verified.
			name:  "a payload that is not JSON",
			token: "eyJhbGciOiJFZERTQSIsInR5cCI6IkpXVCJ9.cGVyc29uQGV4YW1wbGUuY29t.c2lnbmF0dXJlLWJ5dGVz",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := authenticator.Authenticate(t.Context(), testCase.token)
			if err == nil {
				t.Fatalf("%s: accepted", testCase.name)
			}

			written := fmt.Sprint((&altshiftLog.ErrorContextExtractor{}).MakeErrorAttrs(err))

			signature := testCase.token[strings.LastIndexByte(testCase.token, '.')+1:]
			if strings.Contains(written, signature) {
				t.Errorf("%s: the token's signature is written out: %s", testCase.name, written)
			}
			if strings.Contains(written, email) {
				t.Errorf("%s: the person's email address is written out: %s", testCase.name, written)
			}
			for _, keyForm := range keyForms {
				if strings.Contains(written, keyForm) {
					t.Errorf("%s: the private key is written out: %s", testCase.name, written)
				}
			}
			if fingerprint := altshiftJws.Fingerprint(testCase.token); !strings.Contains(written, fingerprint) {
				t.Errorf("%s: the token's fingerprint %s is not written out: %s", testCase.name, fingerprint, written)
			}
		})
	}
}
