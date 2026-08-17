package webauthn

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"fmt"

	"github.com/altshiftab/utils_go/pkg/cose"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	webauthnErrors "github.com/altshiftab/utils_go/pkg/webauthn/errors"
)

// AttestationType is the type of attestation conveyed by a verified attestation statement
// (WebAuthn §6.5.3).
type AttestationType int

const (
	// AttestationTypeNone conveys no attestation information.
	AttestationTypeNone AttestationType = iota
	// AttestationTypeSelf is produced with the credential private key itself.
	AttestationTypeSelf
	// AttestationTypeBasic is produced with an authenticator attestation key backed by a
	// certificate chain.
	AttestationTypeBasic
	// AttestationTypeAnonymizationCa is produced with an authenticator-specific key certified by
	// an anonymization CA (Apple).
	AttestationTypeAnonymizationCa
)

// AttestationVerificationResult is the outcome of a successful attestation statement
// verification.
type AttestationVerificationResult struct {
	Type AttestationType
	// TrustPath is the attestation certificate chain, leaf first, for statement formats that
	// carry one. Evaluating it against acceptable trust anchors (e.g. FIDO metadata) is the
	// caller's policy decision; VerifyAttestationStatement only verifies the statement itself.
	TrustPath []*x509.Certificate
}

var (
	// id-fido-gen-ce-aaguid (WebAuthn §8.2.1).
	oidFidoGenCeAaguid = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 45724, 1, 1, 4}
	// The Apple anonymous attestation nonce extension (WebAuthn §8.8).
	oidAppleAnonymousAttestation = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 8, 2}
)

func statementCertificates(statement map[any]any) ([]*x509.Certificate, error) {
	x5cValue, ok := statement["x5c"]
	if !ok {
		return nil, nil
	}

	entries, ok := x5cValue.([]any)
	if !ok || len(entries) == 0 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: malformed x5c", altshiftErrors.ErrValidationError),
		)
	}

	certificates := make([]*x509.Certificate, 0, len(entries))
	for _, entry := range entries {
		certificateDer, ok := entry.([]byte)
		if !ok {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: malformed x5c entry", altshiftErrors.ErrValidationError),
			)
		}

		certificate, err := x509.ParseCertificate(certificateDer)
		if err != nil {
			return nil, altshiftErrors.New(
				fmt.Errorf("%w: parse certificate: %w", altshiftErrors.ErrValidationError, err),
				certificateDer,
			)
		}

		certificates = append(certificates, certificate)
	}

	return certificates, nil
}

func verifySignature(
	algorithm cose.Algorithm,
	publicKey any,
	message []byte,
	signature []byte,
) error {
	verifier, err := NewVerifier(algorithm, publicKey)
	if err != nil {
		return fmt.Errorf("new verifier: %w", err)
	}

	if err := verifier.Verify(message, signature); err != nil {
		return altshiftErrors.New(
			fmt.Errorf(
				"%w: %w: %w",
				altshiftErrors.ErrVerificationError,
				webauthnErrors.ErrSignatureVerifyFailure,
				err,
			),
			message,
			signature,
		)
	}

	return nil
}

// verifyAttestationCertificate checks the attestation certificate requirements of the packed
// format (WebAuthn §8.2.1).
func verifyAttestationCertificate(certificate *x509.Certificate, aaguid []byte) error {
	if certificate.Version != 3 {
		return fmt.Errorf(
			"%w: attestation certificate version %d is not 3",
			altshiftErrors.ErrValidationError,
			certificate.Version,
		)
	}

	organizationalUnitFound := false
	for _, organizationalUnit := range certificate.Subject.OrganizationalUnit {
		if organizationalUnit == "Authenticator Attestation" {
			organizationalUnitFound = true
			break
		}
	}
	if !organizationalUnitFound {
		return fmt.Errorf(
			"%w: attestation certificate misses the Authenticator Attestation organizational unit",
			altshiftErrors.ErrValidationError,
		)
	}

	if !certificate.BasicConstraintsValid || certificate.IsCA {
		return fmt.Errorf(
			"%w: attestation certificate must not be a ca",
			altshiftErrors.ErrValidationError,
		)
	}

	for _, extension := range certificate.Extensions {
		if !extension.Id.Equal(oidFidoGenCeAaguid) {
			continue
		}

		var certificateAaguid []byte
		if _, err := asn1.Unmarshal(extension.Value, &certificateAaguid); err != nil {
			return altshiftErrors.New(
				fmt.Errorf(
					"%w: asn1 unmarshal (aaguid extension): %w",
					altshiftErrors.ErrValidationError,
					err,
				),
				extension.Value,
			)
		}

		if !bytes.Equal(certificateAaguid, aaguid) {
			return fmt.Errorf(
				"%w: %w",
				altshiftErrors.ErrValidationError,
				mismatch_error.New("aaguid", certificateAaguid, aaguid),
			)
		}
	}

	return nil
}

func verifyPackedStatement(
	attestationObject *AttestationObject,
	attestedCredential *AttestedCredentialData,
	clientDataHash []byte,
) (*AttestationVerificationResult, error) {
	statement := attestationObject.AttestationStatement

	algorithmValue, ok := statement["alg"]
	if !ok {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("attestation statement alg")),
		)
	}
	algorithm, ok := algorithmValue.(int64)
	if !ok {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: malformed attestation statement alg", altshiftErrors.ErrValidationError),
		)
	}

	signature, ok := statement["sig"].([]byte)
	if !ok || len(signature) == 0 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("attestation statement sig")),
		)
	}

	message := append(append([]byte{}, attestationObject.RawAuthenticatorData...), clientDataHash...)

	certificates, err := statementCertificates(statement)
	if err != nil {
		return nil, fmt.Errorf("statement certificates: %w", err)
	}

	if len(certificates) == 0 {
		// Self attestation: the signature is produced with the credential private key, and the
		// algorithm must match the credential public key's.
		if cose.Algorithm(algorithm) != attestedCredential.PublicKeyAlgorithm {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf(
					"%w: %w: attestation statement alg %d does not match credential alg %d",
					altshiftErrors.ErrValidationError,
					webauthnErrors.ErrPublicKeyAlgorithmMismatch,
					algorithm,
					attestedCredential.PublicKeyAlgorithm,
				),
			)
		}

		if err := verifySignature(cose.Algorithm(algorithm), attestedCredential.PublicKey, message, signature); err != nil {
			return nil, fmt.Errorf("verify signature (self attestation): %w", err)
		}

		return &AttestationVerificationResult{Type: AttestationTypeSelf}, nil
	}

	leafCertificate := certificates[0]
	if err := verifySignature(cose.Algorithm(algorithm), leafCertificate.PublicKey, message, signature); err != nil {
		return nil, fmt.Errorf("verify signature (basic attestation): %w", err)
	}

	if err := verifyAttestationCertificate(leafCertificate, attestedCredential.Aaguid); err != nil {
		return nil, fmt.Errorf("verify attestation certificate: %w", err)
	}

	return &AttestationVerificationResult{Type: AttestationTypeBasic, TrustPath: certificates}, nil
}

func verifyAppleStatement(
	attestationObject *AttestationObject,
	attestedCredential *AttestedCredentialData,
	clientDataHash []byte,
) (*AttestationVerificationResult, error) {
	certificates, err := statementCertificates(attestationObject.AttestationStatement)
	if err != nil {
		return nil, fmt.Errorf("statement certificates: %w", err)
	}
	if len(certificates) == 0 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("attestation statement x5c")),
		)
	}
	leafCertificate := certificates[0]

	// The leaf carries a nonce over the attested data, binding the certificate to this ceremony.
	expectedNonce := sha256.Sum256(
		append(append([]byte{}, attestationObject.RawAuthenticatorData...), clientDataHash...),
	)

	var nonce []byte
	for _, extension := range leafCertificate.Extensions {
		if !extension.Id.Equal(oidAppleAnonymousAttestation) {
			continue
		}

		var wrapper struct {
			Nonce []byte `asn1:"tag:1,explicit"`
		}
		if _, err := asn1.Unmarshal(extension.Value, &wrapper); err != nil {
			return nil, altshiftErrors.New(
				fmt.Errorf(
					"%w: asn1 unmarshal (apple nonce extension): %w",
					altshiftErrors.ErrValidationError,
					err,
				),
				extension.Value,
			)
		}
		nonce = wrapper.Nonce
	}

	if !bytes.Equal(nonce, expectedNonce[:]) {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: %w",
				altshiftErrors.ErrValidationError,
				mismatch_error.New("apple nonce", nonce, expectedNonce[:]),
			),
		)
	}

	leafPublicKey, ok := leafCertificate.PublicKey.(interface {
		Equal(x crypto.PublicKey) bool
	})
	if !ok || !leafPublicKey.Equal(attestedCredential.PublicKey) {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: %w: certificate key does not match credential key",
				altshiftErrors.ErrValidationError,
				webauthnErrors.ErrPublicKeyMismatch,
			),
		)
	}

	return &AttestationVerificationResult{Type: AttestationTypeAnonymizationCa, TrustPath: certificates}, nil
}

func verifyFidoU2fStatement(
	attestationObject *AttestationObject,
	attestedCredential *AttestedCredentialData,
	clientDataHash []byte,
	rpIdHash []byte,
) (*AttestationVerificationResult, error) {
	statement := attestationObject.AttestationStatement

	signature, ok := statement["sig"].([]byte)
	if !ok || len(signature) == 0 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("attestation statement sig")),
		)
	}

	certificates, err := statementCertificates(statement)
	if err != nil {
		return nil, fmt.Errorf("statement certificates: %w", err)
	}
	if len(certificates) != 1 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: fido-u2f requires exactly one certificate", altshiftErrors.ErrValidationError),
		)
	}
	leafCertificate := certificates[0]

	credentialPublicKey, ok := attestedCredential.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: fido-u2f requires an ec2 credential key", altshiftErrors.ErrValidationError),
		)
	}
	ecdhPublicKey, err := credentialPublicKey.ECDH()
	if err != nil {
		return nil, altshiftErrors.New(
			fmt.Errorf("%w: ecdh: %w", altshiftErrors.ErrValidationError, err),
			credentialPublicKey,
		)
	}
	// Uncompressed point: 0x04 || X || Y (the U2F public key representation).
	publicKeyU2f := ecdhPublicKey.Bytes()
	if len(publicKeyU2f) != 65 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: fido-u2f requires a p-256 credential key", altshiftErrors.ErrValidationError),
		)
	}

	var verificationData []byte
	verificationData = append(verificationData, 0x00)
	verificationData = append(verificationData, rpIdHash...)
	verificationData = append(verificationData, clientDataHash...)
	verificationData = append(verificationData, attestedCredential.CredentialId...)
	verificationData = append(verificationData, publicKeyU2f...)

	if err := verifySignature(cose.Algorithm(-7), leafCertificate.PublicKey, verificationData, signature); err != nil {
		return nil, fmt.Errorf("verify signature (fido-u2f): %w", err)
	}

	return &AttestationVerificationResult{Type: AttestationTypeBasic, TrustPath: certificates}, nil
}

// VerifyAttestationStatement verifies the attestation statement of a parsed attestation object
// against its format's verification procedure. Supported formats are "none", "packed", "apple",
// and "fido-u2f"; other formats are rejected rather than accepted unverified. Trust-path
// evaluation against acceptable roots is left to the caller via the returned result.
//
// TODO: Support the "tpm" and "android-key" attestation statement formats.
func VerifyAttestationStatement(
	attestationObject *AttestationObject,
	rawClientDataJson []byte,
) (*AttestationVerificationResult, error) {
	if attestationObject == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("attestation object"))
	}

	if len(rawClientDataJson) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("raw client data json"))
	}

	statement := attestationObject.AttestationStatement
	if statement == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("attestation statement"))
	}

	format := attestationObject.Format
	if format == "none" {
		if len(statement) != 0 {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf(
					"%w: the none format requires an empty attestation statement",
					altshiftErrors.ErrValidationError,
				),
			)
		}

		return &AttestationVerificationResult{Type: AttestationTypeNone}, nil
	}

	// Reject unsupported formats before requiring attested credential data, so an unknown format
	// is reported as such rather than as a missing credential.
	switch format {
	case "packed", "apple", "fido-u2f":
	default:
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: %w: %q",
				altshiftErrors.ErrValidationError,
				webauthnErrors.ErrUnsupportedAttestationFormat,
				format,
			),
		)
	}

	authenticatorData := attestationObject.AuthenticatorData
	if authenticatorData == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("authenticator data"))
	}

	attestedCredential := authenticatorData.AttestedCredential
	if attestedCredential == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("attested credential"))
	}

	clientDataHash := sha256.Sum256(rawClientDataJson)

	switch format {
	case "packed":
		return verifyPackedStatement(attestationObject, attestedCredential, clientDataHash[:])
	case "apple":
		return verifyAppleStatement(attestationObject, attestedCredential, clientDataHash[:])
	case "fido-u2f":
		return verifyFidoU2fStatement(attestationObject, attestedCredential, clientDataHash[:], authenticatorData.RpIdHash)
	default:
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: %w: %q",
				altshiftErrors.ErrValidationError,
				webauthnErrors.ErrUnsupportedAttestationFormat,
				format,
			),
		)
	}
}
