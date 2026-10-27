package oauth2

import (
	"slices"
	"testing"
)

func TestCredentialFields(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		fields    []string
		wantIn    []string
		wantNotIn []string
	}{
		{
			name:      "request",
			fields:    TokenRequestCredentialFields(),
			wantIn:    []string{"client_secret", "client_assertion", "code", "code_verifier", "refresh_token", "assertion", "username", "password"},
			wantNotIn: []string{"grant_type", "client_id", "redirect_uri", "scope"},
		},
		{
			name:      "response",
			fields:    TokenResponseCredentialFields(),
			wantIn:    []string{"access_token", "id_token", "refresh_token"},
			wantNotIn: []string{"error", "error_description", "token_type", "expires_in", "scope"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for _, field := range testCase.wantIn {
				if !slices.Contains(testCase.fields, field) {
					t.Errorf("%s: %q is not masked", testCase.name, field)
				}
			}
			for _, field := range testCase.wantNotIn {
				if slices.Contains(testCase.fields, field) {
					t.Errorf("%s: %q is masked, and it says what the exchange was", testCase.name, field)
				}
			}
		})
	}

	// Each call is the caller's own to change.
	fields := TokenRequestCredentialFields()
	fields[0] = "changed"
	if TokenRequestCredentialFields()[0] == "changed" {
		t.Error("a caller's change reached the list")
	}
}
