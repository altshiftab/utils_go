package oauth_token

// Token asks Cloud Tasks to mint an OAuth token for the target. It is for calling a Google API; a
// service of one's own takes an OIDC token instead.
type Token struct {
	ServiceAccountEmail string `json:"serviceAccountEmail,omitzero"`
	// Scope defaults to the cloud-platform scope where it is left out.
	Scope string `json:"scope,omitzero"`
}
