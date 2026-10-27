package rsa

import "fmt"

// GoString writes the method out without its private key, and String and MarshalText write the same.
//
// A method is attached to errors and so to log entries. %#v prints the private key's address only,
// but %+v of the key and a JSON encoder write its exponent and primes out; the method is written out
// the same way whichever is used, so that none of them can.
func (m *Method) GoString() string {
	if m == nil {
		return "(*rsa.Method)(nil)"
	}

	publicKey := m.PublicKey
	if publicKey == nil && m.PrivateKey != nil {
		publicKey = &m.PrivateKey.PublicKey
	}

	publicKeyText := "<nil>"
	if publicKey != nil && publicKey.N != nil {
		publicKeyText = fmt.Sprintf("<%d-bit key>", publicKey.N.BitLen())
	}

	privateKeyText := "<nil>"
	if m.PrivateKey != nil {
		privateKeyText = "<redacted>"
	}

	return fmt.Sprintf("&rsa.Method{Name:%q, PublicKey:%s, PrivateKey:%s}", m.Name, publicKeyText, privateKeyText)
}

func (m *Method) String() string {
	return m.GoString()
}

func (m *Method) MarshalText() ([]byte, error) {
	return []byte(m.GoString()), nil
}
