// Package pii_hasher names a person in a log without writing their personal data there: by a keyed
// hash of it, which is the same for the same value wherever the key is the same -- so that entries,
// and services sharing the key, can be matched on it, and an address someone gives can be looked up
// by hashing it -- and which no one without the key can reverse, or test a guessed value against.
//
// A plain hash would do neither: anyone can hash a list of likely addresses and compare.
package pii_hasher

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

// MinimumKeyLength is the shortest key New takes: the length of the hash's output, which RFC 2104
// gives as the length below which a key weakens the MAC.
const MinimumKeyLength = sha256.Size

// ErrShortKey is a key shorter than MinimumKeyLength.
var ErrShortKey = errors.New("key shorter than the minimum")

type Hasher struct {
	key []byte
}

// New makes a hasher with its own copy of the key.
func New(key []byte) (*Hasher, error) {
	if len(key) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("key"))
	}
	if len(key) < MinimumKeyLength {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %d bytes, of %d", ErrShortKey, len(key), MinimumKeyLength),
		)
	}

	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)

	return &Hasher{key: keyCopy}, nil
}

// HashEmailAddress is the HMAC-SHA256 of the address in lower case, written as hex, and the empty
// string for an empty address. Other services compute the same over the same key, and their values
// are matched against these, so the form is fixed.
//
// A nil hasher hashes nothing and returns the empty string, so that a caller given no hasher writes
// no address at all, rather than one in the clear.
func (h *Hasher) HashEmailAddress(emailAddress string) string {
	if h == nil || emailAddress == "" {
		return ""
	}

	mac := hmac.New(sha256.New, h.key)
	mac.Write([]byte(strings.ToLower(emailAddress)))

	return hex.EncodeToString(mac.Sum(nil))
}
