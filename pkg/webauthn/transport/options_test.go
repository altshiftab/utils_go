package transport

import (
	"encoding/json/v2"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/webauthn"
)

func TestUnmarshalPublicKeyCredentialCreationOptions(t *testing.T) {
	t.Parallel()

	const input = `
		{
		  "challenge": "Z7IN7MdOR5o0dkDHomcmxUHh3dr7vO3SL1X_NUnD_a94-CV-TxVXjQ4LmpBa6pvImqiWYD5eKaG48MksNaOVpQ",
		  "rp": {
			"name": "Alt-Shift Login",
			"id": "alt-shift.se"
		  },
		  "user": {
			"id": "YjdiYmFhMTQtMmQzZS00ZTQyLWI1NjUtZmJhYTFkOWM1MmQ1",
			"name": "v@example.com",
			"displayName": "Please"
		  },
		  "pubKeyCredParams": [
			{
			  "type": "public-key",
			  "alg": -7
			}
		  ],
		  "authenticatorSelection": {
			"authenticatorAttachment": "platform",
			"residentKey": "required",
			"requireResidentKey": true
		  },
		  "attestation": "none"
		}
	`

	var options PublicKeyCredentialCreationOptions
	if err := json.Unmarshal([]byte(input), &options); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	relyingParty := options.RelyingParty
	if relyingParty == nil || relyingParty.Id != "alt-shift.se" {
		t.Errorf("relying party mismatch: %+v", relyingParty)
	}
}

func TestMarshalPublicKeyCredentialCreationOptions(t *testing.T) {
	t.Parallel()

	challenge := Base64URL("test-challenge")
	userId := Base64URL("test-user-id")

	options := PublicKeyCredentialCreationOptions{
		RelyingParty: &webauthn.RelyingParty{Name: "Test", Id: "example.com"},
		User:         &PublicKeyCredentialUserEntity{Id: &userId},
		Challenge:    &challenge,
		PubKeyCredParams: []*webauthn.PublicKeyCredentialParam{
			{Type: "public-key", Alg: -7},
		},
		AuthenticatorSelection: &webauthn.AuthenticatorSelection{
			AuthenticatorAttachment: "platform",
			ResidentKey:             "required",
		},
		Attestation: "none",
	}

	data, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	output := string(data)

	// The residentKey preference must force requireResidentKey for older clients.
	if !strings.Contains(output, `"requireResidentKey":true`) {
		t.Errorf("missing forced requireResidentKey: %s", output)
	}

	if !strings.Contains(output, `"challenge":"dGVzdC1jaGFsbGVuZ2U"`) {
		t.Errorf("missing base64url challenge: %s", output)
	}

	if !strings.Contains(output, `"rp":{"name":"Test","id":"example.com"}`) {
		t.Errorf("missing relying party: %s", output)
	}
}

func TestMarshalPublicKeyCredentialRequestOptions(t *testing.T) {
	t.Parallel()

	challenge := Base64URL("test-challenge")

	options := PublicKeyCredentialRequestOptions{
		Challenge: &challenge,
		RpId:      "example.com",
	}

	data, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	output := string(data)

	if !strings.Contains(output, `"challenge":"dGVzdC1jaGFsbGVuZ2U"`) {
		t.Errorf("missing base64url challenge: %s", output)
	}

	if !strings.Contains(output, `"rpId":"example.com"`) {
		t.Errorf("missing rp id: %s", output)
	}
}
