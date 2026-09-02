package oidc_token

// Token asks Cloud Tasks to mint an OIDC token for the target and send it in the Authorization
// header. It is how a task authenticates to a Cloud Run service or to a service that verifies the
// token itself.
type Token struct {
	// ServiceAccountEmail is the identity the token is minted as. Cloud Tasks' own service agent
	// needs the token creator role on it.
	ServiceAccountEmail string `json:"serviceAccountEmail,omitzero"`
	// Audience is what the token is minted for. Left out, the target URL is used, which is why a
	// service checking a fixed audience has to set this.
	Audience string `json:"audience,omitzero"`
}
