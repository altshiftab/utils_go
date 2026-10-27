package oauth2

// TokenRequestCredentialFields are the fields of a token request that carry a credential: the
// client's secret or assertion (RFC 6749 section 2.3, RFC 7523), the grant's code, verifier,
// refresh token, assertion or password (RFC 6749 section 4, RFC 7636, RFC 7523). They are what a log
// masks of one, keeping what says what was asked for: the grant type, the client id, the redirect
// URI and the scope.
func TokenRequestCredentialFields() []string {
	return []string{
		"client_secret",
		"client_assertion",
		"code",
		"code_verifier",
		"refresh_token",
		"assertion",
		"password",
	}
}

// TokenResponseCredentialFields are the fields of a token response that carry a credential: the
// tokens issued (RFC 6749 section 5.1, OpenID Connect Core section 3.1.3.3). They are what a log
// masks of one, keeping the token type, the lifetime and scope granted, and an error response
// (RFC 6749 section 5.2) whole -- which is what says why an exchange failed.
func TokenResponseCredentialFields() []string {
	return []string{
		"access_token",
		"id_token",
		"refresh_token",
	}
}
