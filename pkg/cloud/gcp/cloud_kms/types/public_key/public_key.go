package public_key

type PublicKey struct {
	Pem             string `json:"pem"`
	Algorithm       string `json:"algorithm,omitzero"`
	Name            string `json:"name,omitzero"`
	ProtectionLevel string `json:"protectionLevel,omitzero"`
}
