package eddsa

import (
	"encoding/base64"
	"fmt"
)

// GoString writes the method out without its private key, and String and MarshalText write the same.
//
// A method is attached to errors and so to log entries -- as the verifier of a token that was
// refused, the signer of one that failed to encode -- and every way a value is written out would
// otherwise write the key along with it: %#v and %v print the key's bytes, a JSON encoder its base64.
// The public key is kept, being what tells one method from another in a log.
func (method *Method) GoString() string {
	if method == nil {
		return "(*eddsa.Method)(nil)"
	}

	return fmt.Sprintf(
		"&eddsa.Method{PublicKey:%q, PrivateKey:%s}",
		base64.RawURLEncoding.EncodeToString(method.PublicKey),
		redacted(len(method.PrivateKey) != 0),
	)
}

func (method *Method) String() string {
	return method.GoString()
}

func (method *Method) MarshalText() ([]byte, error) {
	return []byte(method.GoString()), nil
}

func redacted(present bool) string {
	if present {
		return "<redacted>"
	}
	return "<nil>"
}
