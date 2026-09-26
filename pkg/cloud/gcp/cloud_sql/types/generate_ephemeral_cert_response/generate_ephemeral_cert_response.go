package generate_ephemeral_cert_response

type SslCert struct {
	Cert           string `json:"cert,omitzero"`
	ExpirationTime string `json:"expirationTime,omitzero"`
}

type Response struct {
	EphemeralCert *SslCert `json:"ephemeralCert,omitzero"`
}
