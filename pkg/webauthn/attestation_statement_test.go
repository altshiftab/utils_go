package webauthn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/cbor"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	webauthnErrors "github.com/altshiftab/utils_go/pkg/webauthn/errors"
)

type attestationCertificateConfig struct {
	organizationalUnit string
	isCa               bool
	aaguid             []byte
	appleNonce         []byte
}

func makeAttestationCertificate(
	t *testing.T,
	publicKey *ecdsa.PublicKey,
	signerKey *ecdsa.PrivateKey,
	config *attestationCertificateConfig,
) []byte {
	t.Helper()

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "attestation"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  config.isCa,
	}

	if config.organizationalUnit != "" {
		template.Subject.OrganizationalUnit = []string{config.organizationalUnit}
	}

	if config.aaguid != nil {
		aaguidValue, err := asn1.Marshal(config.aaguid)
		if err != nil {
			t.Fatalf("asn1 marshal (aaguid): %v", err)
		}
		template.ExtraExtensions = append(
			template.ExtraExtensions,
			pkix.Extension{Id: oidFidoGenCeAaguid, Value: aaguidValue},
		)
	}

	if config.appleNonce != nil {
		nonceValue, err := asn1.Marshal(struct {
			Nonce []byte `asn1:"tag:1,explicit"`
		}{Nonce: config.appleNonce})
		if err != nil {
			t.Fatalf("asn1 marshal (apple nonce): %v", err)
		}
		template.ExtraExtensions = append(
			template.ExtraExtensions,
			pkix.Extension{Id: oidAppleAnonymousAttestation, Value: nonceValue},
		)
	}

	certificateDer, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, signerKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	return certificateDer
}

func makeAttestationObject(t *testing.T, format string, statement map[any]any, authData []byte) *AttestationObject {
	t.Helper()

	data, err := cbor.Encode(map[any]any{
		"fmt":      format,
		"attStmt":  statement,
		"authData": authData,
	})
	if err != nil {
		t.Fatalf("cbor encode: %v", err)
	}

	attestationObject, err := ParseAttestationObject(data)
	if err != nil {
		t.Fatalf("parse attestation object: %v", err)
	}

	return attestationObject
}

func signStatementMessage(t *testing.T, privateKey *ecdsa.PrivateKey, message []byte) []byte {
	t.Helper()

	messageHash := sha256.Sum256(message)
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, messageHash[:])
	if err != nil {
		t.Fatalf("ecdsa sign asn1: %v", err)
	}

	return signature
}

func TestVerifyAttestationStatementNone(t *testing.T) {
	t.Parallel()

	t.Run("real fixture", func(t *testing.T) {
		t.Parallel()

		attestationObject, err := ParseAttestationObject(decodeBase64(t, attestationObjectBase64))
		if err != nil {
			t.Fatalf("parse attestation object: %v", err)
		}

		result, err := VerifyAttestationStatement(attestationObject, []byte("{}"))
		if err != nil {
			t.Fatalf("verify attestation statement: %v", err)
		}

		if result.Type != AttestationTypeNone || result.TrustPath != nil {
			t.Errorf("result: got %+v", result)
		}
	})

	t.Run("non-empty statement", func(t *testing.T) {
		t.Parallel()

		attestationObject := makeAttestationObject(
			t,
			"none",
			map[any]any{"sig": []byte{1}},
			makeTestAuthenticatorData(t, FlagUserPresent, 0, nil),
		)

		if _, err := VerifyAttestationStatement(attestationObject, []byte("{}")); !errors.Is(err, altshiftErrors.ErrValidationError) {
			t.Errorf("expected validation error, got %v", err)
		}
	})
}

func TestVerifyAttestationStatementPacked(t *testing.T) {
	t.Parallel()

	credentialKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}

	rawClientDataJson := makeClientDataJson(t, WebauthnCreateType, testChallenge)
	clientDataHash := sha256.Sum256(rawClientDataJson)
	authData := makeCeremonyAuthenticatorData(
		t,
		FlagUserPresent|FlagAttestedCredentialData,
		0,
		makeEcdsaCoseKey(t, &credentialKey.PublicKey),
	)
	message := append(append([]byte{}, authData...), clientDataHash[:]...)

	t.Run("self attestation", func(t *testing.T) {
		t.Parallel()

		statement := map[any]any{
			"alg": int64(-7),
			"sig": signStatementMessage(t, credentialKey, message),
		}

		result, err := VerifyAttestationStatement(
			makeAttestationObject(t, "packed", statement, authData),
			rawClientDataJson,
		)
		if err != nil {
			t.Fatalf("verify attestation statement: %v", err)
		}

		if result.Type != AttestationTypeSelf || result.TrustPath != nil {
			t.Errorf("result: got %+v", result)
		}
	})

	t.Run("self attestation algorithm mismatch", func(t *testing.T) {
		t.Parallel()

		statement := map[any]any{
			"alg": int64(-257),
			"sig": signStatementMessage(t, credentialKey, message),
		}

		_, err := VerifyAttestationStatement(
			makeAttestationObject(t, "packed", statement, authData),
			rawClientDataJson,
		)
		if !errors.Is(err, webauthnErrors.ErrPublicKeyAlgorithmMismatch) {
			t.Errorf("expected public key algorithm mismatch, got %v", err)
		}
	})

	t.Run("self attestation tampered signature", func(t *testing.T) {
		t.Parallel()

		signature := signStatementMessage(t, credentialKey, message)
		signature[len(signature)-1] ^= 0xff
		statement := map[any]any{"alg": int64(-7), "sig": signature}

		_, err := VerifyAttestationStatement(
			makeAttestationObject(t, "packed", statement, authData),
			rawClientDataJson,
		)
		if !errors.Is(err, webauthnErrors.ErrSignatureVerifyFailure) {
			t.Errorf("expected signature verify failure, got %v", err)
		}
	})

	attestationKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}

	t.Run("basic attestation", func(t *testing.T) {
		t.Parallel()

		certificateDer := makeAttestationCertificate(
			t,
			&attestationKey.PublicKey,
			attestationKey,
			&attestationCertificateConfig{
				organizationalUnit: "Authenticator Attestation",
				aaguid:             make([]byte, aaguidLength),
			},
		)
		statement := map[any]any{
			"alg": int64(-7),
			"sig": signStatementMessage(t, attestationKey, message),
			"x5c": []any{certificateDer},
		}

		result, err := VerifyAttestationStatement(
			makeAttestationObject(t, "packed", statement, authData),
			rawClientDataJson,
		)
		if err != nil {
			t.Fatalf("verify attestation statement: %v", err)
		}

		if result.Type != AttestationTypeBasic || len(result.TrustPath) != 1 {
			t.Errorf("result: got %+v", result)
		}
	})

	certificateCases := []struct {
		name           string
		config         *attestationCertificateConfig
		expectMismatch bool
	}{
		{
			name:   "missing organizational unit",
			config: &attestationCertificateConfig{aaguid: make([]byte, aaguidLength)},
		},
		{
			name: "ca certificate",
			config: &attestationCertificateConfig{
				organizationalUnit: "Authenticator Attestation",
				isCa:               true,
			},
		},
		{
			name: "aaguid mismatch",
			config: &attestationCertificateConfig{
				organizationalUnit: "Authenticator Attestation",
				aaguid:             bytes_Repeat(0xff, aaguidLength),
			},
			expectMismatch: true,
		},
	}

	for _, testCase := range certificateCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			certificateDer := makeAttestationCertificate(
				t,
				&attestationKey.PublicKey,
				attestationKey,
				testCase.config,
			)
			statement := map[any]any{
				"alg": int64(-7),
				"sig": signStatementMessage(t, attestationKey, message),
				"x5c": []any{certificateDer},
			}

			_, err := VerifyAttestationStatement(
				makeAttestationObject(t, "packed", statement, authData),
				rawClientDataJson,
			)
			if !errors.Is(err, altshiftErrors.ErrValidationError) {
				t.Errorf("expected validation error, got %v", err)
			}
			if testCase.expectMismatch {
				if mismatchErr, ok := errors.AsType[*mismatch_error.Error](err); !ok || mismatchErr.Field != "aaguid" {
					t.Errorf("expected aaguid mismatch error, got %v", err)
				}
			}
		})
	}
}

func bytes_Repeat(value byte, count int) []byte {
	data := make([]byte, count)
	for i := range data {
		data[i] = value
	}
	return data
}

func TestVerifyAttestationStatementApple(t *testing.T) {
	t.Parallel()

	credentialKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}

	rawClientDataJson := makeClientDataJson(t, WebauthnCreateType, testChallenge)
	clientDataHash := sha256.Sum256(rawClientDataJson)
	authData := makeCeremonyAuthenticatorData(
		t,
		FlagUserPresent|FlagAttestedCredentialData,
		0,
		makeEcdsaCoseKey(t, &credentialKey.PublicKey),
	)

	nonce := sha256.Sum256(append(append([]byte{}, authData...), clientDataHash[:]...))

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		certificateDer := makeAttestationCertificate(
			t,
			&credentialKey.PublicKey,
			credentialKey,
			&attestationCertificateConfig{appleNonce: nonce[:]},
		)

		result, err := VerifyAttestationStatement(
			makeAttestationObject(t, "apple", map[any]any{"x5c": []any{certificateDer}}, authData),
			rawClientDataJson,
		)
		if err != nil {
			t.Fatalf("verify attestation statement: %v", err)
		}

		if result.Type != AttestationTypeAnonymizationCa || len(result.TrustPath) != 1 {
			t.Errorf("result: got %+v", result)
		}
	})

	t.Run("nonce mismatch", func(t *testing.T) {
		t.Parallel()

		certificateDer := makeAttestationCertificate(
			t,
			&credentialKey.PublicKey,
			credentialKey,
			&attestationCertificateConfig{appleNonce: bytes_Repeat(0xff, 32)},
		)

		_, err := VerifyAttestationStatement(
			makeAttestationObject(t, "apple", map[any]any{"x5c": []any{certificateDer}}, authData),
			rawClientDataJson,
		)
		if mismatchErr, ok := errors.AsType[*mismatch_error.Error](err); !ok || mismatchErr.Field != "apple nonce" {
			t.Errorf("expected apple nonce mismatch error, got %v", err)
		}
	})

	t.Run("key mismatch", func(t *testing.T) {
		t.Parallel()

		otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("ecdsa generate key: %v", err)
		}

		certificateDer := makeAttestationCertificate(
			t,
			&otherKey.PublicKey,
			otherKey,
			&attestationCertificateConfig{appleNonce: nonce[:]},
		)

		_, err = VerifyAttestationStatement(
			makeAttestationObject(t, "apple", map[any]any{"x5c": []any{certificateDer}}, authData),
			rawClientDataJson,
		)
		if !errors.Is(err, webauthnErrors.ErrPublicKeyMismatch) {
			t.Errorf("expected public key mismatch, got %v", err)
		}
	})
}

func TestVerifyAttestationStatementFidoU2f(t *testing.T) {
	t.Parallel()

	credentialKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}

	attestationKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}

	rawClientDataJson := makeClientDataJson(t, WebauthnCreateType, testChallenge)
	clientDataHash := sha256.Sum256(rawClientDataJson)
	authData := makeCeremonyAuthenticatorData(
		t,
		FlagUserPresent|FlagAttestedCredentialData,
		0,
		makeEcdsaCoseKey(t, &credentialKey.PublicKey),
	)

	attestationObject := makeAttestationObject(t, "fido-u2f", map[any]any{}, authData)
	authenticatorData := attestationObject.AuthenticatorData
	attestedCredential := authenticatorData.AttestedCredential

	ecdhPublicKey, err := credentialKey.PublicKey.ECDH()
	if err != nil {
		t.Fatalf("ecdh: %v", err)
	}

	var verificationData []byte
	verificationData = append(verificationData, 0x00)
	verificationData = append(verificationData, authenticatorData.RpIdHash...)
	verificationData = append(verificationData, clientDataHash[:]...)
	verificationData = append(verificationData, attestedCredential.CredentialId...)
	verificationData = append(verificationData, ecdhPublicKey.Bytes()...)

	certificateDer := makeAttestationCertificate(
		t,
		&attestationKey.PublicKey,
		attestationKey,
		&attestationCertificateConfig{},
	)

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		statement := map[any]any{
			"sig": signStatementMessage(t, attestationKey, verificationData),
			"x5c": []any{certificateDer},
		}

		result, err := VerifyAttestationStatement(
			makeAttestationObject(t, "fido-u2f", statement, authData),
			rawClientDataJson,
		)
		if err != nil {
			t.Fatalf("verify attestation statement: %v", err)
		}

		if result.Type != AttestationTypeBasic || len(result.TrustPath) != 1 {
			t.Errorf("result: got %+v", result)
		}
	})

	t.Run("tampered signature", func(t *testing.T) {
		t.Parallel()

		signature := signStatementMessage(t, attestationKey, verificationData)
		signature[len(signature)-1] ^= 0xff
		statement := map[any]any{"sig": signature, "x5c": []any{certificateDer}}

		_, err := VerifyAttestationStatement(
			makeAttestationObject(t, "fido-u2f", statement, authData),
			rawClientDataJson,
		)
		if !errors.Is(err, webauthnErrors.ErrSignatureVerifyFailure) {
			t.Errorf("expected signature verify failure, got %v", err)
		}
	})
}

func TestVerifyAttestationStatementRejects(t *testing.T) {
	t.Parallel()

	authData := makeTestAuthenticatorData(t, FlagUserPresent, 0, nil)

	t.Run("nil attestation object", func(t *testing.T) {
		t.Parallel()

		if _, err := VerifyAttestationStatement(nil, []byte("{}")); err == nil {
			t.Errorf("expected error")
		}
	})

	t.Run("empty raw client data json", func(t *testing.T) {
		t.Parallel()

		attestationObject := makeAttestationObject(t, "none", map[any]any{}, authData)
		if _, err := VerifyAttestationStatement(attestationObject, nil); err == nil {
			t.Errorf("expected error")
		}
	})

	t.Run("unsupported format", func(t *testing.T) {
		t.Parallel()

		attestationObject := makeAttestationObject(t, "tpm", map[any]any{"sig": []byte{1}}, authData)

		_, err := VerifyAttestationStatement(attestationObject, []byte("{}"))
		if !errors.Is(err, webauthnErrors.ErrUnsupportedAttestationFormat) {
			t.Errorf("expected unsupported attestation format, got %v", err)
		}
	})
}
