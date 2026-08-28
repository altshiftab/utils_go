package oauth_token

// Token asks Cloud Scheduler to mint an OAuth 2.0 access token for the target. It is for calling
// Google APIs; a service of one's own wants an OIDC token instead.
type Token struct {
	ServiceAccountEmail string `json:"serviceAccountEmail,omitzero"`
	// Scope is the OAuth scope requested. Defaults to the cloud-platform scope.
	Scope string `json:"scope,omitzero"`
}
