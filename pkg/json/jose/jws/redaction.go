package jws

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
)

// Fingerprint names a serialization without carrying it: "sha256:" and the first eight bytes of its
// SHA-256, in hex. It is what goes on an error in place of a token, so that the log lines one token
// produced can be told together and apart from another's, and no line holds a token anyone could
// present again.
func Fingerprint(serialization string) string {
	sum := sha256.Sum256([]byte(serialization))
	return "sha256:" + hex.EncodeToString(sum[:8])
}

// compactSerialization matches a JWS compact serialization with a JSON header -- "eyJ" is the
// base64url of `{"` -- up to and including the dot before its signature. The header must start a
// token rather than sit inside a word, so that "keyJar.a.b" or a path segment that happens to hold
// "eyJ" is left as it is. The first group is what precedes the token, the second the token up to its
// signature.
var compactSerialization = regexp.MustCompile(`(^|[^A-Za-z0-9_-])(eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*\.)[A-Za-z0-9_-]+`)

// MaskedSignature is what MaskSignatures writes in place of a signature.
const MaskedSignature = "(MASKED)"

// MaskSignatures replaces the signature of every JWS compact serialization in text with
// MaskedSignature, keeping the header and payload: without its signature a token cannot be
// presented again, and what it says is still there to read. It is the same masking the request log
// gives the headers that carry tokens, for text that reaches a log by another way.
func MaskSignatures(text string) string {
	return compactSerialization.ReplaceAllString(text, "${1}${2}"+MaskedSignature)
}
