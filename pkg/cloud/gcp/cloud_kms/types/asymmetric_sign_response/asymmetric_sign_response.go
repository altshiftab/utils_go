package asymmetric_sign_response

type Response struct {
	// Signature is base64-encoded.
	Signature string `json:"signature"`
	// Name is the key version that signed, which is the one asked for.
	Name            string `json:"name,omitzero"`
	ProtectionLevel string `json:"protectionLevel,omitzero"`
}
