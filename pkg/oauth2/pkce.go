package oauth2

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"

	"github.com/altshiftab/utils_go/pkg/oauth2/types/auth_code_option"
)

const (
	codeChallengeKey       = "code_challenge"
	codeChallengeMethodKey = "code_challenge_method"
	codeVerifierKey        = "code_verifier"
)

// GenerateVerifier generates a PKCE code verifier with 32 octets of randomness.
// This follows recommendations in RFC 7636.
//
// A fresh verifier should be generated for each authorization.
// S256ChallengeOption(verifier) should then be passed to Config.AuthCodeURL
// and VerifierOption(verifier) to Config.Exchange.
func GenerateVerifier() string {
	// "RECOMMENDED that the output of a suitable random number generator be
	// used to create a 32-octet sequence.  The octet sequence is then
	// base64url-encoded to produce a 43-octet URL-safe string to use as the
	// code verifier."
	// https://datatracker.ietf.org/doc/html/rfc7636#section-4.1
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

// S256ChallengeFromVerifier returns a PKCE code challenge derived from verifier with method S256.
//
// Prefer to use S256ChallengeOption where possible.
func S256ChallengeFromVerifier(verifier string) string {
	sha := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sha[:])
}

// S256ChallengeOption returns AuthCodeOption values that set the PKCE code challenge
// and challenge method derived from verifier with method S256.
// The returned options should be passed to Config.AuthCodeURL only.
func S256ChallengeOption(verifier string) []auth_code_option.AuthCodeOption {
	return []auth_code_option.AuthCodeOption{
		auth_code_option.New(codeChallengeMethodKey, "S256"),
		auth_code_option.New(codeChallengeKey, S256ChallengeFromVerifier(verifier)),
	}
}

// VerifierOption returns a PKCE code verifier AuthCodeOption.
// It should be passed to Config.Exchange only.
func VerifierOption(verifier string) auth_code_option.AuthCodeOption {
	return auth_code_option.New(codeVerifierKey, verifier)
}
