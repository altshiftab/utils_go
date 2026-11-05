// Package capability mints and verifies capability tokens: a token that is, for a few minutes, the
// right to one thing a service names in a grant of its own type, and nothing else.
//
// A service hands one out as a link, so that a file reaches the machine that wants it without
// passing through whatever asked for it -- a model would have to repeat it byte for byte. The token
// is all the link's holder needs, which is why it says exactly what it grants and runs out soon.
//
// A token is the grant and its expiry as JSON, base64url-encoded, then a dot and an HMAC-SHA256 of
// that, keyed with the service's key and prefixed with its domain, so that two services' tokens are
// never mistaken for each other's even should they ever share a key.
package capability

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

// DefaultTtl is how long a token lasts: long enough to be used at once, short enough that one found
// later in a log or a transcript grants nothing.
const DefaultTtl = 10 * time.Minute

// MinKeyBytes is the shortest key a signer takes.
const MinKeyBytes = 32

var (
	// ErrMalformed is a token that is not one a signer could have made.
	ErrMalformed = errors.New("malformed token")
	// ErrBadSignature is a token whose signature is not the key's, in the signer's domain.
	ErrBadSignature = errors.New("bad signature")
	// ErrExpired is a token whose time has run out.
	ErrExpired = errors.New("expired token")
)

// envelope is what a token carries: the grant, and when it runs out in Unix seconds.
type envelope[T any] struct {
	Grant     *T    `json:"g"`
	ExpiresAt int64 `json:"e"`
}

// Signer mints and verifies tokens of grants of type T, with one key in one domain.
type Signer[T any] struct {
	key    []byte
	domain string
	ttl    time.Duration
	now    func() time.Time
}

// NewSigner is a signer of the key, which is to be random and at least MinKeyBytes long, in the
// domain, which names the service and the use ("vvvp-google/download/v1"). A ttl of zero is
// DefaultTtl; a nil now is time.Now.
func NewSigner[T any](key []byte, domain string, ttl time.Duration, now func() time.Time) (*Signer[T], error) {
	if len(key) < MinKeyBytes {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: the key is %d bytes, and at least %d are needed", altshiftErrors.ErrValidationError, len(key), MinKeyBytes),
		)
	}
	if domain == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("domain"))
	}
	// The domain is ended with a NUL before the payload, so a domain holding one could end early.
	if strings.ContainsRune(domain, 0) {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: the domain holds a NUL", altshiftErrors.ErrValidationError))
	}
	if ttl <= 0 {
		ttl = DefaultTtl
	}
	if now == nil {
		now = time.Now
	}
	return &Signer[T]{key: key, domain: domain, ttl: ttl, now: now}, nil
}

func (signer *Signer[T]) mac(payload string) []byte {
	mac := hmac.New(sha256.New, signer.key)
	mac.Write([]byte(signer.domain))
	mac.Write([]byte{0})
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}

// Mint is a token of the grant, which runs out the signer's TTL from now, and when that is. The
// grant is the caller's to have checked: the signer signs what it is given.
func (signer *Signer[T]) Mint(grant *T) (string, time.Time, error) {
	if grant == nil {
		return "", time.Time{}, altshiftErrors.NewWithTrace(nil_error.New("grant"))
	}

	expiresAt := signer.now().Add(signer.ttl).Truncate(time.Second)
	data, err := json.Marshal(&envelope[T]{Grant: grant, ExpiresAt: expiresAt.Unix()}, json.Deterministic(true))
	if err != nil {
		return "", time.Time{}, altshiftErrors.NewWithTrace(fmt.Errorf("json marshal (envelope): %w", err))
	}
	payload := base64.RawURLEncoding.EncodeToString(data)
	return payload + "." + base64.RawURLEncoding.EncodeToString(signer.mac(payload)), expiresAt, nil
}

// Verify is the grant of a token this signer's key minted in its domain and that has not run out.
// What the grant must hold is the caller's to check.
func (signer *Signer[T]) Verify(token string) (*T, error) {
	malformed := func() error {
		return altshiftErrors.NewWithTrace(fmt.Errorf("%w: %w", altshiftErrors.ErrVerificationError, ErrMalformed))
	}

	payload, signature, ok := strings.Cut(token, ".")
	if !ok || payload == "" || signature == "" {
		return nil, malformed()
	}

	gotMac, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return nil, malformed()
	}
	if !hmac.Equal(gotMac, signer.mac(payload)) {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %w", altshiftErrors.ErrVerificationError, ErrBadSignature))
	}

	data, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, malformed()
	}
	var parsed envelope[T]
	if err := json.Unmarshal(data, &parsed, json.RejectUnknownMembers(true)); err != nil {
		return nil, malformed()
	}
	if parsed.Grant == nil || parsed.ExpiresAt == 0 {
		return nil, malformed()
	}
	if !signer.now().Before(time.Unix(parsed.ExpiresAt, 0)) {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %w", altshiftErrors.ErrVerificationError, ErrExpired))
	}

	return parsed.Grant, nil
}
