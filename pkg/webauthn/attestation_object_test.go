package webauthn

import (
	"bytes"
	"crypto/x509"
	"errors"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cbor"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// The attestation object of a real registration ceremony, and the credential public key of the
// same ceremony as PKIX DER from the client's getPublicKey().
const (
	attestationObjectBase64  = "o2NmbXRkbm9uZWdhdHRTdG10oGhhdXRoRGF0YViU1a2ljx0QHe9thc1Bo3Gm2O8_GyFQPoxAhTSh0lRpifNdAAAAAOqbjWZNAR0hPOS2tIy1ddQAEALA2Pdf4UsE1U_KMGj_4X-lAQIDJiABIVggAZdqCklTaOiYUPmAfwoiOiCzV71PdToO0G7LS-JKWJMiWCC8RDpZjuMxm4dwDtBf1Ybd1jMrqzK4LSg-8P7tVB4R4Q"
	clientPublicKeyDerBase64 = "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEAZdqCklTaOiYUPmAfwoiOiCzV71PdToO0G7LS-JKWJO8RDpZjuMxm4dwDtBf1Ybd1jMrqzK4LSg-8P7tVB4R4Q"
)

func TestParseAttestationObject(t *testing.T) {
	t.Parallel()

	attestationObject, err := ParseAttestationObject(decodeBase64(t, attestationObjectBase64))
	if err != nil {
		t.Fatalf("parse attestation object: %v", err)
	}

	if attestationObject.Format != "none" {
		t.Errorf("format: got %q, want \"none\"", attestationObject.Format)
	}

	if len(attestationObject.AttestationStatement) != 0 {
		t.Errorf("unexpected attestation statement: %v", attestationObject.AttestationStatement)
	}

	if !bytes.Equal(
		attestationObject.RawAuthenticatorData,
		decodeBase64(t, attestationAuthenticatorDataBase64),
	) {
		t.Errorf("raw authenticator data mismatch")
	}

	authenticatorData := attestationObject.AuthenticatorData
	if authenticatorData == nil {
		t.Fatalf("missing authenticator data")
	}

	attestedCredential := authenticatorData.AttestedCredential
	if attestedCredential == nil {
		t.Fatalf("missing attested credential")
	}

	// The server-side extraction of the credential public key from the attestation object must
	// agree with the key the client reported via getPublicKey().
	der, err := x509.MarshalPKIXPublicKey(attestedCredential.PublicKey)
	if err != nil {
		t.Fatalf("marshal pkix public key: %v", err)
	}

	if !bytes.Equal(der, decodeBase64(t, clientPublicKeyDerBase64)) {
		t.Errorf("public key der mismatch")
	}
}

func TestParseAttestationObjectRejects(t *testing.T) {
	t.Parallel()

	encode := func(value any) []byte {
		data, err := cbor.Encode(value)
		if err != nil {
			t.Fatalf("cbor encode: %v", err)
		}
		return data
	}

	authData := decodeBase64(t, assertionAuthenticatorDataBase64)

	testCases := []struct {
		name string
		data []byte
	}{
		{name: "not cbor", data: []byte{0xff}},
		{name: "not a map", data: encode(int64(1))},
		{
			name: "missing format",
			data: encode(map[any]any{"attStmt": map[any]any{}, "authData": authData}),
		},
		{
			name: "missing attestation statement",
			data: encode(map[any]any{"fmt": "none", "authData": authData}),
		},
		{
			name: "missing authenticator data",
			data: encode(map[any]any{"fmt": "none", "attStmt": map[any]any{}}),
		},
		{
			name: "malformed authenticator data",
			data: encode(map[any]any{
				"fmt":      "none",
				"attStmt":  map[any]any{},
				"authData": []byte{1, 2, 3},
			}),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := ParseAttestationObject(testCase.data); !errors.Is(err, altshiftErrors.ErrParseError) {
				t.Errorf("expected parse error, got %v", err)
			}
		})
	}
}
