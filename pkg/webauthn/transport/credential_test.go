package transport

import (
	"bytes"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"strings"
	"testing"

	webauthnErrors "github.com/altshiftab/utils_go/pkg/webauthn/errors"
)

// A real authentication ceremony response for rp id "alt-shift.se".
//
//nolint:gosec // G101: public ceremony output used as a test fixture, not a credential.
const assertionCredentialJson = `
	{
	  "authenticatorAttachment": "platform",
	  "clientExtensionResults": {},
	  "id": "AsDY91_hSwTVT8owaP_hfw",
	  "rawId": "AsDY91_hSwTVT8owaP_hfw",
	  "response": {
		"authenticatorData": "1a2ljx0QHe9thc1Bo3Gm2O8_GyFQPoxAhTSh0lRpifMdAAAAAA",
		"clientDataJSON": "eyJ0eXBlIjoid2ViYXV0aG4uZ2V0IiwiY2hhbGxlbmdlIjoiQVJQTU9oTTczYnVIUmxuNXFQb2lkTjV2SFdzSGwtbDVEdTJxNWlwZmlrRm5UYWRLZGJXYTZFdkNMUlBYaUR1dnotWXNibnV3Y1NpbGhSTG44NERFelEiLCJvcmlnaW4iOiJodHRwczovL2xvZ2luLmFsdC1zaGlmdC5zZSIsImNyb3NzT3JpZ2luIjpmYWxzZX0",
		"signature": "MEQCIBCCQkxvytFK7GGjITF2san-K8nHPy3f2uTX3p9zqqtWAiAaEUiZTi0FmEfLvy6Su0k6rneI-mwXEK041d9qDsCTyA",
		"userHandle": "YjdiYmFhMTQtMmQzZS00ZTQyLWI1NjUtZmJhYTFkOWM1MmQ1"
	  },
	  "type": "public-key"
	}
`

// A real registration ceremony response for the same rp id. The publicKey and
// publicKeyAlgorithm fields match the COSE key within the attestation object.
//
//nolint:gosec // G101: public ceremony output used as a test fixture, not a credential.
const attestationCredentialJson = `
	{
	  "authenticatorAttachment": "platform",
	  "clientExtensionResults": {},
	  "id": "AsDY91_hSwTVT8owaP_hfw",
	  "rawId": "AsDY91_hSwTVT8owaP_hfw",
	  "response": {
		"attestationObject": "o2NmbXRkbm9uZWdhdHRTdG10oGhhdXRoRGF0YViU1a2ljx0QHe9thc1Bo3Gm2O8_GyFQPoxAhTSh0lRpifNdAAAAAOqbjWZNAR0hPOS2tIy1ddQAEALA2Pdf4UsE1U_KMGj_4X-lAQIDJiABIVggAZdqCklTaOiYUPmAfwoiOiCzV71PdToO0G7LS-JKWJMiWCC8RDpZjuMxm4dwDtBf1Ybd1jMrqzK4LSg-8P7tVB4R4Q",
		"authenticatorData": "1a2ljx0QHe9thc1Bo3Gm2O8_GyFQPoxAhTSh0lRpifNdAAAAAOqbjWZNAR0hPOS2tIy1ddQAEALA2Pdf4UsE1U_KMGj_4X-lAQIDJiABIVggAZdqCklTaOiYUPmAfwoiOiCzV71PdToO0G7LS-JKWJMiWCC8RDpZjuMxm4dwDtBf1Ybd1jMrqzK4LSg-8P7tVB4R4Q",
		"clientDataJSON": "eyJ0eXBlIjoid2ViYXV0aG4uY3JlYXRlIiwiY2hhbGxlbmdlIjoiWjdJTjdNZE9SNW8wZGtESG9tY214VUhoM2RyN3ZPM1NMMVhfTlVuRF9hOTQtQ1YtVHhWWGpRNExtcEJhNnB2SW1xaVdZRDVlS2FHNDhNa3NOYU9WcFEiLCJvcmlnaW4iOiJodHRwczovL2xvZ2luLmFsdC1zaGlmdC5zZSIsImNyb3NzT3JpZ2luIjpmYWxzZX0",
		"publicKey": "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEAZdqCklTaOiYUPmAfwoiOiCzV71PdToO0G7LS-JKWJO8RDpZjuMxm4dwDtBf1Ybd1jMrqzK4LSg-8P7tVB4R4Q",
		"publicKeyAlgorithm": -7,
		"transports": [
		  "hybrid",
		  "internal"
		]
	  },
	  "type": "public-key"
	}
`

func TestMakeAssertionPublicKeyCredential(t *testing.T) {
	t.Parallel()

	var transportCredential AssertionPublicKeyCredential
	if err := json.Unmarshal([]byte(assertionCredentialJson), &transportCredential); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	credential, err := MakeAssertionPublicKeyCredential(&transportCredential)
	if err != nil {
		t.Fatalf("make assertion public key credential: %v", err)
	}
	if credential == nil {
		t.Fatalf("nil credential")
	}

	if credential.Type != "public-key" {
		t.Errorf("type: got %q", credential.Type)
	}

	response := credential.Response

	clientData := response.ClientDataJson
	if clientData.Type != "webauthn.get" {
		t.Errorf("client data type: got %q", clientData.Type)
	}
	if clientData.Origin != "https://login.alt-shift.se" {
		t.Errorf("client data origin: got %q", clientData.Origin)
	}

	expectedChallenge, err := base64.RawURLEncoding.DecodeString(
		"ARPMOhM73buHRln5qPoidN5vHWsHl-l5Du2q5ipfikFnTadKdbWa6EvCLRPXiDuvz-YsbnuwcSilhRLn84DEzQ",
	)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if !bytes.Equal(clientData.Challenge, expectedChallenge) {
		t.Errorf("challenge mismatch")
	}

	if string(response.UserHandle) != "b7bbaa14-2d3e-4e42-b565-fbaa1d9c52d5" {
		t.Errorf("user handle: got %q", response.UserHandle)
	}

	if len(response.Signature) == 0 {
		t.Errorf("empty signature")
	}
}

func TestMakeAttestationPublicKeyCredential(t *testing.T) {
	t.Parallel()

	var transportCredential AttestationPublicKeyCredential
	if err := json.Unmarshal([]byte(attestationCredentialJson), &transportCredential); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	credential, err := MakeAttestationPublicKeyCredential(&transportCredential)
	if err != nil {
		t.Fatalf("make attestation public key credential: %v", err)
	}
	if credential == nil {
		t.Fatalf("nil credential")
	}

	response := credential.Response

	if response.ClientDataJson.Type != "webauthn.create" {
		t.Errorf("client data type: got %q", response.ClientDataJson.Type)
	}

	attestationObject := response.AttestationObject
	if attestationObject == nil {
		t.Fatalf("nil attestation object")
	}
	if attestationObject.Format != "none" {
		t.Errorf("format: got %q", attestationObject.Format)
	}

	authenticatorData := attestationObject.AuthenticatorData
	if authenticatorData == nil {
		t.Fatalf("nil authenticator data")
	}
	if authenticatorData.AttestedCredential == nil {
		t.Fatalf("nil attested credential")
	}

	if response.Transports[0] != "hybrid" || response.Transports[1] != "internal" {
		t.Errorf("transports mismatch: %v", response.Transports)
	}
}

func TestMakeAttestationPublicKeyCredentialCrossChecks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		mutate      func(s string) string
		expectedErr error
	}{
		{
			name: "tampered public key",
			mutate: func(s string) string {
				return strings.Replace(s, `"publicKey": "MFkwEwYHK`, `"publicKey": "MFkwEwYHL`, 1)
			},
			expectedErr: webauthnErrors.ErrPublicKeyMismatch,
		},
		{
			name: "mismatching public key algorithm",
			mutate: func(s string) string {
				return strings.Replace(s, `"publicKeyAlgorithm": -7`, `"publicKeyAlgorithm": -257`, 1)
			},
			expectedErr: webauthnErrors.ErrPublicKeyAlgorithmMismatch,
		},
		{
			name: "mismatching authenticator data",
			mutate: func(s string) string {
				return strings.Replace(
					s,
					`"authenticatorData": "1a2ljx0QHe9`,
					`"authenticatorData": "2a2ljx0QHe9`,
					1,
				)
			},
			expectedErr: webauthnErrors.ErrAuthenticatorDataMismatch,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var transportCredential AttestationPublicKeyCredential
			err := json.Unmarshal([]byte(testCase.mutate(attestationCredentialJson)), &transportCredential)
			if err != nil {
				t.Fatalf("json unmarshal: %v", err)
			}

			if _, err := MakeAttestationPublicKeyCredential(&transportCredential); !errors.Is(err, testCase.expectedErr) {
				t.Errorf("expected %v, got %v", testCase.expectedErr, err)
			}
		})
	}
}

func TestMakeAssertionPublicKeyCredentialMissingFields(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		mutate func(credential *AssertionPublicKeyCredential)
	}{
		{name: "missing id", mutate: func(c *AssertionPublicKeyCredential) { c.Id = nil }},
		{name: "missing raw id", mutate: func(c *AssertionPublicKeyCredential) { c.RawId = nil }},
		{
			name:   "missing signature",
			mutate: func(c *AssertionPublicKeyCredential) { c.Response.Signature = nil },
		},
		{
			name:   "missing user handle",
			mutate: func(c *AssertionPublicKeyCredential) { c.Response.UserHandle = nil },
		},
		{
			name:   "missing client data json",
			mutate: func(c *AssertionPublicKeyCredential) { c.Response.ClientDataJson = nil },
		},
		{
			name:   "missing authenticator data",
			mutate: func(c *AssertionPublicKeyCredential) { c.Response.AuthenticatorData = nil },
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var transportCredential AssertionPublicKeyCredential
			if err := json.Unmarshal([]byte(assertionCredentialJson), &transportCredential); err != nil {
				t.Fatalf("json unmarshal: %v", err)
			}

			testCase.mutate(&transportCredential)

			if _, err := MakeAssertionPublicKeyCredential(&transportCredential); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}
