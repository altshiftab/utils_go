package generate_ephemeral_cert_request

type Request struct {
	PublicKey   string `json:"public_key"`
	AccessToken string `json:"access_token,omitzero"`
}
