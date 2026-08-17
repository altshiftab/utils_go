package webauthn

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cbor"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// Authenticator data from a real registration ceremony for rp id "alt-shift.se" (extracted from
// the attestation object in attestation_object_test.go).
const attestationAuthenticatorDataBase64 = "1a2ljx0QHe9thc1Bo3Gm2O8_GyFQPoxAhTSh0lRpifNdAAAAAOqbjWZNAR0hPOS2tIy1ddQAEALA2Pdf4UsE1U_KMGj_4X-lAQIDJiABIVggAZdqCklTaOiYUPmAfwoiOiCzV71PdToO0G7LS-JKWJMiWCC8RDpZjuMxm4dwDtBf1Ybd1jMrqzK4LSg-8P7tVB4R4Q"

// Authenticator data from a real authentication ceremony for the same rp id.
const assertionAuthenticatorDataBase64 = "1a2ljx0QHe9thc1Bo3Gm2O8_GyFQPoxAhTSh0lRpifMdAAAAAA"

func decodeBase64(t *testing.T, encoded string) []byte {
	t.Helper()

	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}

	return data
}

func TestParseAuthenticatorDataAttestation(t *testing.T) {
	t.Parallel()

	data := decodeBase64(t, attestationAuthenticatorDataBase64)

	authenticatorData, err := ParseAuthenticatorData(data)
	if err != nil {
		t.Fatalf("parse authenticator data: %v", err)
	}

	expectedRpIdHash := sha256.Sum256([]byte("alt-shift.se"))
	if !bytes.Equal(authenticatorData.RpIdHash, expectedRpIdHash[:]) {
		t.Errorf("rp id hash mismatch")
	}

	if authenticatorData.Flags != 0x5d {
		t.Errorf("flags: got %#x, want 0x5d", authenticatorData.Flags)
	}

	if !authenticatorData.UserPresent() || !authenticatorData.UserVerified() {
		t.Errorf("expected user present and verified")
	}

	if authenticatorData.SignCount != 0 {
		t.Errorf("sign count: got %d, want 0", authenticatorData.SignCount)
	}

	attestedCredential := authenticatorData.AttestedCredential
	if attestedCredential == nil {
		t.Fatalf("missing attested credential")
	}

	if len(attestedCredential.Aaguid) != aaguidLength {
		t.Errorf("aaguid length: got %d, want %d", len(attestedCredential.Aaguid), aaguidLength)
	}

	if len(attestedCredential.CredentialId) != 16 {
		t.Errorf("credential id length: got %d, want 16", len(attestedCredential.CredentialId))
	}

	if _, ok := attestedCredential.PublicKey.(*ecdsa.PublicKey); !ok {
		t.Errorf("public key type: got %T, want *ecdsa.PublicKey", attestedCredential.PublicKey)
	}

	if attestedCredential.PublicKeyAlgorithm != -7 {
		t.Errorf("public key algorithm: got %d, want -7", attestedCredential.PublicKeyAlgorithm)
	}

	// The COSE key must be delimited exactly: its raw bytes plus everything before it must
	// account for the entire input.
	expectedRawPublicKeyLength := len(data) - minAuthenticatorDataLength - aaguidLength -
		credentialIdLengthLength - len(attestedCredential.CredentialId)
	if len(attestedCredential.RawPublicKey) != expectedRawPublicKeyLength {
		t.Errorf(
			"raw public key length: got %d, want %d",
			len(attestedCredential.RawPublicKey),
			expectedRawPublicKeyLength,
		)
	}

	if authenticatorData.Extensions != nil {
		t.Errorf("unexpected extensions: %v", authenticatorData.Extensions)
	}
}

func TestParseAuthenticatorDataAssertion(t *testing.T) {
	t.Parallel()

	authenticatorData, err := ParseAuthenticatorData(decodeBase64(t, assertionAuthenticatorDataBase64))
	if err != nil {
		t.Fatalf("parse authenticator data: %v", err)
	}

	if authenticatorData.AttestedCredential != nil {
		t.Errorf("unexpected attested credential")
	}

	if !authenticatorData.UserPresent() || !authenticatorData.UserVerified() {
		t.Errorf("expected user present and verified")
	}
}

func makeTestAuthenticatorData(t *testing.T, flags byte, signCount uint32, tail []byte) []byte {
	t.Helper()

	rpIdHash := sha256.Sum256([]byte("example.com"))
	data := make([]byte, 0, minAuthenticatorDataLength+len(tail))
	data = append(data, rpIdHash[:]...)
	data = append(data, flags)
	data = binary.BigEndian.AppendUint32(data, signCount)
	return append(data, tail...)
}

func TestParseAuthenticatorDataExtensions(t *testing.T) {
	t.Parallel()

	extensions, err := cbor.Encode(map[any]any{"credProtect": int64(2)})
	if err != nil {
		t.Fatalf("cbor encode: %v", err)
	}

	authenticatorData, err := ParseAuthenticatorData(
		makeTestAuthenticatorData(t, FlagUserPresent|FlagExtensionData, 1, extensions),
	)
	if err != nil {
		t.Fatalf("parse authenticator data: %v", err)
	}

	if value, ok := authenticatorData.Extensions["credProtect"]; !ok || value != int64(2) {
		t.Errorf("extensions mismatch: %v", authenticatorData.Extensions)
	}
}

func TestParseAuthenticatorDataAttestedCredentialWithExtensions(t *testing.T) {
	t.Parallel()

	coseKey := decodeBase64(
		t,
		"pQECAyYgASFYIAGXagpJU2jomFD5gH8KIjogs1e9T3U6DtBuy0viSliTIlggvEQ6WY7jMZuHcA7QX9WG3dYzK6syuC0oPvD-7VQeEeE",
	)

	extensions, err := cbor.Encode(map[any]any{"credProtect": int64(2)})
	if err != nil {
		t.Fatalf("cbor encode: %v", err)
	}

	var tail []byte
	tail = append(tail, make([]byte, aaguidLength)...)
	tail = binary.BigEndian.AppendUint16(tail, 4)
	tail = append(tail, []byte{1, 2, 3, 4}...)
	tail = append(tail, coseKey...)
	tail = append(tail, extensions...)

	authenticatorData, err := ParseAuthenticatorData(
		makeTestAuthenticatorData(
			t,
			FlagUserPresent|FlagAttestedCredentialData|FlagExtensionData,
			1,
			tail,
		),
	)
	if err != nil {
		t.Fatalf("parse authenticator data: %v", err)
	}

	attestedCredential := authenticatorData.AttestedCredential
	if attestedCredential == nil {
		t.Fatalf("missing attested credential")
	}

	// The COSE key must be delimited from the extensions that follow it.
	if !bytes.Equal(attestedCredential.RawPublicKey, coseKey) {
		t.Errorf("raw public key mismatch")
	}

	if value, ok := authenticatorData.Extensions["credProtect"]; !ok || value != int64(2) {
		t.Errorf("extensions mismatch: %v", authenticatorData.Extensions)
	}
}

func TestParseAuthenticatorDataRejects(t *testing.T) {
	t.Parallel()

	coseKey := decodeBase64(
		t,
		"pQECAyYgASFYIAGXagpJU2jomFD5gH8KIjogs1e9T3U6DtBuy0viSliTIlggvEQ6WY7jMZuHcA7QX9WG3dYzK6syuC0oPvD-7VQeEeE",
	)

	makeAttestedTail := func(credentialIdLength uint16, credentialId []byte, key []byte) []byte {
		var tail []byte
		tail = append(tail, make([]byte, aaguidLength)...)
		tail = binary.BigEndian.AppendUint16(tail, credentialIdLength)
		tail = append(tail, credentialId...)
		return append(tail, key...)
	}

	testCases := []struct {
		name string
		data []byte
	}{
		{name: "too short", data: make([]byte, minAuthenticatorDataLength-1)},
		{
			name: "trailing data without extension flag",
			data: makeTestAuthenticatorData(t, FlagUserPresent, 1, []byte{0xff}),
		},
		{
			name: "extension flag without extension data",
			data: makeTestAuthenticatorData(t, FlagUserPresent|FlagExtensionData, 1, nil),
		},
		{
			name: "extensions not a map",
			data: makeTestAuthenticatorData(t, FlagUserPresent|FlagExtensionData, 1, []byte{0x01}),
		},
		{
			name: "attested credential flag without data",
			data: makeTestAuthenticatorData(t, FlagUserPresent|FlagAttestedCredentialData, 1, nil),
		},
		{
			name: "credential id length exceeding data",
			data: makeTestAuthenticatorData(
				t,
				FlagUserPresent|FlagAttestedCredentialData,
				1,
				makeAttestedTail(100, []byte{1, 2}, nil),
			),
		},
		{
			name: "malformed credential public key",
			data: makeTestAuthenticatorData(
				t,
				FlagUserPresent|FlagAttestedCredentialData,
				1,
				makeAttestedTail(2, []byte{1, 2}, []byte{0xff}),
			),
		},
		{
			name: "credential public key not a map",
			data: makeTestAuthenticatorData(
				t,
				FlagUserPresent|FlagAttestedCredentialData,
				1,
				makeAttestedTail(2, []byte{1, 2}, []byte{0x01}),
			),
		},
		{
			name: "trailing data after credential public key",
			data: makeTestAuthenticatorData(
				t,
				FlagUserPresent|FlagAttestedCredentialData,
				1,
				makeAttestedTail(2, []byte{1, 2}, append(append([]byte{}, coseKey...), 0xff)),
			),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := ParseAuthenticatorData(testCase.data); !errors.Is(err, altshiftErrors.ErrParseError) {
				t.Errorf("expected parse error, got %v", err)
			}
		})
	}
}
