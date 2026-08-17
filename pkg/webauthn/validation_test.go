package webauthn

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json/v2"
	"errors"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cbor"
	"github.com/altshiftab/utils_go/pkg/cose"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	webauthnErrors "github.com/altshiftab/utils_go/pkg/webauthn/errors"
)

const (
	testRpId   = "example.com"
	testOrigin = "https://example.com"
)

var testChallenge = []byte("test-challenge-test-challenge-test-challenge-test-challenge-1234")

func makeClientDataJson(t *testing.T, ceremonyType string, challenge []byte) []byte {
	t.Helper()

	data, err := json.Marshal(map[string]any{
		"type":        ceremonyType,
		"challenge":   base64.RawURLEncoding.EncodeToString(challenge),
		"origin":      testOrigin,
		"crossOrigin": false,
	})
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}

	return data
}

func makeEcdsaCoseKey(t *testing.T, publicKey *ecdsa.PublicKey) []byte {
	t.Helper()

	ecdhKey, err := publicKey.ECDH()
	if err != nil {
		t.Fatalf("ecdh: %v", err)
	}

	// Uncompressed point: 0x04 || X || Y
	raw := ecdhKey.Bytes()
	coordinateSize := (len(raw) - 1) / 2
	data, err := cbor.Encode(map[int64]any{
		1:  int64(2),
		3:  int64(-7),
		-1: cose.CurveP256,
		-2: raw[1 : 1+coordinateSize],
		-3: raw[1+coordinateSize:],
	})
	if err != nil {
		t.Fatalf("cbor encode: %v", err)
	}

	return data
}

func makeCeremonyAuthenticatorData(t *testing.T, flags byte, signCount uint32, coseKey []byte) []byte {
	t.Helper()

	rpIdHash := sha256.Sum256([]byte(testRpId))
	data := append([]byte{}, rpIdHash[:]...)
	data = append(data, flags)
	data = binary.BigEndian.AppendUint32(data, signCount)

	if coseKey != nil {
		data = append(data, make([]byte, aaguidLength)...)
		data = binary.BigEndian.AppendUint16(data, 4)
		data = append(data, []byte{1, 2, 3, 4}...)
		data = append(data, coseKey...)
	}

	return data
}

func makeAttestationCredential(t *testing.T, publicKey *ecdsa.PublicKey) *AttestationPublicKeyCredential {
	t.Helper()

	authData := makeCeremonyAuthenticatorData(
		t,
		FlagUserPresent|FlagUserVerified|FlagAttestedCredentialData,
		0,
		makeEcdsaCoseKey(t, publicKey),
	)

	attestationObjectData, err := cbor.Encode(map[any]any{
		"fmt":      "none",
		"attStmt":  map[any]any{},
		"authData": authData,
	})
	if err != nil {
		t.Fatalf("cbor encode: %v", err)
	}

	attestationObject, err := ParseAttestationObject(attestationObjectData)
	if err != nil {
		t.Fatalf("parse attestation object: %v", err)
	}

	collectedClientData := CollectedClientData{
		Type:      WebauthnCreateType,
		Challenge: testChallenge,
		Origin:    testOrigin,
	}

	return &AttestationPublicKeyCredential{
		Id:    []byte{1, 2, 3, 4},
		Type:  ExpectedCredentialType,
		RawId: []byte{1, 2, 3, 4},
		Response: AuthenticatorAttestationResponse{
			ClientDataJson:    collectedClientData,
			AttestationObject: attestationObject,
			Transports:        []string{"internal"},
		},
	}
}

func TestValidateAttestationPublicKeyCredential(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}

	testCases := []struct {
		name              string
		challenge         []byte
		allowedAlgorithms []cose.Algorithm
		expectedErr       error
	}{
		{
			name:              "valid",
			challenge:         testChallenge,
			allowedAlgorithms: []cose.Algorithm{-7},
		},
		{
			name:              "challenge mismatch",
			challenge:         []byte("another-challenge-another-challenge-another-challenge-another-12"),
			allowedAlgorithms: []cose.Algorithm{-7},
			expectedErr:       webauthnErrors.ErrChallengeMismatch,
		},
		{
			name:              "algorithm mismatch",
			challenge:         testChallenge,
			allowedAlgorithms: []cose.Algorithm{-257},
			expectedErr:       webauthnErrors.ErrPublicKeyAlgorithmMismatch,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateAttestationPublicKeyCredential(
				makeAttestationCredential(t, &privateKey.PublicKey),
				testCase.challenge,
				testOrigin,
				testRpId,
				testCase.allowedAlgorithms,
			)
			if testCase.expectedErr == nil {
				if err != nil {
					t.Fatalf("expected ok, got: %v", err)
				}
				return
			}
			if !errors.Is(err, testCase.expectedErr) {
				t.Errorf("expected %v, got %v", testCase.expectedErr, err)
			}
			if !errors.Is(err, motmedelErrors.ErrValidationError) {
				t.Errorf("expected validation error classification, got %v", err)
			}
		})
	}
}

type assertionCeremony struct {
	credential           *AssertionPublicKeyCredential
	rawClientDataJson    []byte
	rawAuthenticatorData []byte
	signature            []byte
}

func makeAssertionCeremony(
	t *testing.T,
	signCount uint32,
	sign func(message []byte) []byte,
) *assertionCeremony {
	t.Helper()

	rawClientDataJson := makeClientDataJson(t, WebauthnGetType, testChallenge)
	rawAuthenticatorData := makeCeremonyAuthenticatorData(
		t,
		FlagUserPresent|FlagUserVerified,
		signCount,
		nil,
	)

	clientDataJsonHash := sha256.Sum256(rawClientDataJson)
	signature := sign(append(append([]byte{}, rawAuthenticatorData...), clientDataJsonHash[:]...))

	authenticatorData, err := ParseAuthenticatorData(rawAuthenticatorData)
	if err != nil {
		t.Fatalf("parse authenticator data: %v", err)
	}

	collectedClientData, err := func() (*CollectedClientData, error) {
		var transportData struct {
			Type      string `json:"type"`
			Challenge string `json:"challenge"`
			Origin    string `json:"origin"`
		}
		if err := json.Unmarshal(rawClientDataJson, &transportData); err != nil {
			return nil, err
		}
		challenge, err := base64.RawURLEncoding.DecodeString(transportData.Challenge)
		if err != nil {
			return nil, err
		}
		return &CollectedClientData{
			Type:      transportData.Type,
			Challenge: challenge,
			Origin:    transportData.Origin,
		}, nil
	}()
	if err != nil {
		t.Fatalf("parse client data json: %v", err)
	}

	return &assertionCeremony{
		credential: &AssertionPublicKeyCredential{
			Id:    []byte{1, 2, 3, 4},
			Type:  ExpectedCredentialType,
			RawId: []byte{1, 2, 3, 4},
			Response: AuthenticatorAssertionResponse{
				ClientDataJson:    *collectedClientData,
				AuthenticatorData: *authenticatorData,
				Signature:         signature,
				UserHandle:        []byte("test-user-id"),
			},
		},
		rawClientDataJson:    rawClientDataJson,
		rawAuthenticatorData: rawAuthenticatorData,
		signature:            signature,
	}
}

func TestValidateAssertionPublicKeyCredentialEcdsa(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}

	sign := func(message []byte) []byte {
		messageHash := sha256.Sum256(message)
		signature, err := ecdsa.SignASN1(rand.Reader, privateKey, messageHash[:])
		if err != nil {
			t.Fatalf("ecdsa sign asn1: %v", err)
		}
		return signature
	}

	verifier, err := NewVerifier(-7, &privateKey.PublicKey)
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}

	testCases := []struct {
		name                   string
		tamperSignature        bool
		previousSignatureCount uint32
		signCount              uint32
		expectedErr            error
	}{
		{name: "valid", signCount: 1},
		{name: "valid without sign count", signCount: 0},
		{
			name:            "tampered signature",
			signCount:       1,
			tamperSignature: true,
			expectedErr:     webauthnErrors.ErrSignatureVerifyFailure,
		},
		{
			name:                   "stale sign count",
			signCount:              1,
			previousSignatureCount: 1,
			expectedErr:            webauthnErrors.ErrUnexpectedSignatureCount,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ceremony := makeAssertionCeremony(t, testCase.signCount, sign)
			if testCase.tamperSignature {
				ceremony.credential.Response.Signature[0] ^= 0xff
			}

			err := ValidateAssertionPublicKeyCredential(
				ceremony.credential,
				ceremony.rawClientDataJson,
				ceremony.rawAuthenticatorData,
				testChallenge,
				testOrigin,
				testRpId,
				testCase.previousSignatureCount,
				verifier,
			)
			if testCase.expectedErr == nil {
				if err != nil {
					t.Fatalf("expected ok, got: %v", err)
				}
				return
			}
			if !errors.Is(err, testCase.expectedErr) {
				t.Errorf("expected %v, got %v", testCase.expectedErr, err)
			}
		})
	}
}

func TestValidateAssertionPublicKeyCredentialEd25519(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519 generate key: %v", err)
	}

	verifier, err := NewVerifier(-8, publicKey)
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}

	ceremony := makeAssertionCeremony(t, 1, func(message []byte) []byte {
		return ed25519.Sign(privateKey, message)
	})

	err = ValidateAssertionPublicKeyCredential(
		ceremony.credential,
		ceremony.rawClientDataJson,
		ceremony.rawAuthenticatorData,
		testChallenge,
		testOrigin,
		testRpId,
		0,
		verifier,
	)
	if err != nil {
		t.Fatalf("expected ok, got: %v", err)
	}
}

func TestNewVerifierRejects(t *testing.T) {
	t.Parallel()

	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}

	ed25519Key, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519 generate key: %v", err)
	}

	testCases := []struct {
		name      string
		algorithm cose.Algorithm
		publicKey any
	}{
		{name: "unknown algorithm", algorithm: 999, publicKey: &ecdsaKey.PublicKey},
		{name: "curve algorithm mismatch", algorithm: -7, publicKey: &ecdsaKey.PublicKey},
		{name: "ed25519 with es256", algorithm: -7, publicKey: ed25519Key},
		{name: "nil public key", algorithm: -7, publicKey: nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewVerifier(testCase.algorithm, testCase.publicKey); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}
