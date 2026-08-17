package webauthn

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"slices"

	"github.com/altshiftab/utils_go/pkg/cose"
	altshiftCryptoInterfaces "github.com/altshiftab/utils_go/pkg/crypto/interfaces"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/utils"
	webauthnErrors "github.com/altshiftab/utils_go/pkg/webauthn/errors"
)

const (
	WebauthnCreateType     = "webauthn.create"
	WebauthnGetType        = "webauthn.get"
	ExpectedCredentialType = "public-key"
)

// The classification lists group validation errors that are caused by bad client input, for
// mapping to Bad Request responses.
var (
	CommonBadRequestErrors = []error{
		webauthnErrors.ErrCredentialTypeMismatch,
		webauthnErrors.ErrCollectedClientDataTypeMismatch,
		webauthnErrors.ErrChallengeMismatch,
		webauthnErrors.ErrOriginMismatch,
		webauthnErrors.ErrRpIdHashMismatch,
		webauthnErrors.ErrUserNotPresent,
		webauthnErrors.ErrUserNotVerified,
		webauthnErrors.ErrUnexpectedSignatureCount,
	}
	AttestationBadRequestErrors = []error{
		webauthnErrors.ErrPublicKeyAlgorithmMismatch,
	}
	AssertionBadRequestErrors = []error{
		webauthnErrors.ErrSignatureVerifyFailure,
	}
)

type authenticatorResponseTypes interface {
	AuthenticatorResponse
	AuthenticatorAttestationResponse | AuthenticatorAssertionResponse
}

func validatePublicKeyCredential[T authenticatorResponseTypes](
	credential *PublicKeyCredential[T],
	expectedCollectedClientDataType string,
	expectedCollectedClientDataChallenge []byte,
	expectedCollectedClientDataOrigin string,
	expectedRpId string,
	previousSignatureCount uint32,
	attestCredentialData bool,
) error {
	if expectedCollectedClientDataType == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("expected collected client data type"))
	}

	if len(expectedCollectedClientDataChallenge) == 0 {
		return altshiftErrors.NewWithTrace(empty_error.New("expected challenge"))
	}

	if expectedCollectedClientDataOrigin == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("expected origin"))
	}

	if expectedRpId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("expected rp id"))
	}

	if credential == nil {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, nil_error.New("public key credential"))
	}

	if len(credential.Id) == 0 {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("credential id"))
	}

	observedCredentialType := credential.Type
	if observedCredentialType != ExpectedCredentialType {
		return altshiftErrors.New(
			fmt.Errorf(
				"%w: %w: %q",
				altshiftErrors.ErrValidationError,
				webauthnErrors.ErrCredentialTypeMismatch,
				observedCredentialType,
			),
			observedCredentialType,
			ExpectedCredentialType,
		)
	}

	response := credential.Response

	collectedClientData := response.GetClientDataJson()
	if collectedClientData == nil {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, nil_error.New("collected client data"))
	}

	authenticatorData := response.GetAuthenticatorData()
	if authenticatorData == nil {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, nil_error.New("authenticator data"))
	}

	err := ValidateCollectedClientData(
		collectedClientData,
		expectedCollectedClientDataType,
		expectedCollectedClientDataChallenge,
		expectedCollectedClientDataOrigin,
	)
	if err != nil {
		return altshiftErrors.New(fmt.Errorf("validate collected client data: %w", err), collectedClientData)
	}

	err = ValidateAuthenticatorData(
		authenticatorData,
		expectedRpId,
		previousSignatureCount,
		attestCredentialData,
		false,
	)
	if err != nil {
		return altshiftErrors.New(fmt.Errorf("validate authenticator data: %w", err), authenticatorData)
	}

	return nil
}

// ValidateAttestationPublicKeyCredential validates a registration ceremony's credential. The
// allowed algorithms are checked against the alg parameter of the COSE credential public key in
// the attestation object's authenticator data.
// Attestation statements are not evaluated here; use VerifyAttestationStatement for that.
func ValidateAttestationPublicKeyCredential(
	credential *AttestationPublicKeyCredential,
	expectedCollectedClientDataChallenge []byte,
	expectedCollectedClientDataOrigin string,
	expectedRpId string,
	allowedPublicKeyAlgorithms []cose.Algorithm,
) error {
	if len(expectedCollectedClientDataChallenge) == 0 {
		return altshiftErrors.NewWithTrace(empty_error.New("expected challenge"))
	}

	if expectedCollectedClientDataOrigin == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("expected origin"))
	}

	if expectedRpId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("expected rp id"))
	}

	if len(allowedPublicKeyAlgorithms) == 0 {
		return altshiftErrors.NewWithTrace(empty_error.New("allowed public key algorithms"))
	}

	if credential == nil {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, nil_error.New("public key credential"))
	}

	err := validatePublicKeyCredential(
		credential,
		WebauthnCreateType,
		expectedCollectedClientDataChallenge,
		expectedCollectedClientDataOrigin,
		expectedRpId,
		0,
		true,
	)
	if err != nil {
		return fmt.Errorf("validate public key credential: %w", err)
	}

	response := credential.Response

	if len(response.Transports) == 0 {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("transports"))
	}

	// validatePublicKeyCredential has established the attested credential's presence; the
	// guards are repeated for the benefit of static nil analysis.
	authenticatorData := response.GetAuthenticatorData()
	if authenticatorData == nil {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, nil_error.New("authenticator data"))
	}
	attestedCredential := authenticatorData.AttestedCredential
	if attestedCredential == nil {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, nil_error.New("attested credential"))
	}

	observedPublicKeyAlgorithm := attestedCredential.PublicKeyAlgorithm
	if !slices.Contains(allowedPublicKeyAlgorithms, observedPublicKeyAlgorithm) {
		return altshiftErrors.New(
			fmt.Errorf(
				"%w: %w: %d",
				altshiftErrors.ErrValidationError,
				webauthnErrors.ErrPublicKeyAlgorithmMismatch,
				observedPublicKeyAlgorithm,
			),
			observedPublicKeyAlgorithm,
			allowedPublicKeyAlgorithms,
		)
	}

	return nil
}

// ValidateAssertionPublicKeyCredential validates an authentication ceremony's credential,
// including its signature over the raw authenticator data and client data hash.
func ValidateAssertionPublicKeyCredential(
	credential *AssertionPublicKeyCredential,
	rawClientDataJson []byte,
	rawAuthenticatorData []byte,
	expectedCollectedClientDataChallenge []byte,
	expectedCollectedClientDataOrigin string,
	expectedRpId string,
	previousSignatureCount uint32,
	verifier altshiftCryptoInterfaces.Verifier,
) error {
	if len(expectedCollectedClientDataChallenge) == 0 {
		return altshiftErrors.NewWithTrace(empty_error.New("expected challenge"))
	}

	if expectedCollectedClientDataOrigin == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("expected origin"))
	}

	if expectedRpId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("expected rp id"))
	}

	if utils.IsNil(verifier) {
		return altshiftErrors.NewWithTrace(nil_error.New("verifier"))
	}

	if credential == nil {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, nil_error.New("public key credential"))
	}

	if len(rawClientDataJson) == 0 {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("raw client data json"))
	}

	if len(rawAuthenticatorData) == 0 {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("raw authenticator data"))
	}

	err := validatePublicKeyCredential(
		credential,
		WebauthnGetType,
		expectedCollectedClientDataChallenge,
		expectedCollectedClientDataOrigin,
		expectedRpId,
		previousSignatureCount,
		false,
	)
	if err != nil {
		return fmt.Errorf("validate public key credential: %w", err)
	}

	response := credential.Response

	signature := response.Signature
	if len(signature) == 0 {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("signature"))
	}

	if len(response.UserHandle) == 0 {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("user handle"))
	}

	clientDataJsonHash := sha256.Sum256(rawClientDataJson)
	message := bytes.Join([][]byte{rawAuthenticatorData, clientDataJsonHash[:]}, nil)

	if err := verifier.Verify(message, signature); err != nil {
		// Any verification failure rejects the assertion; the error is classified as a bad
		// request regardless of whether the signature mismatched or could not be processed.
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

// ValidateCollectedClientData validates the client data of a ceremony against the expected
// type, challenge, and origin.
func ValidateCollectedClientData(
	clientData *CollectedClientData,
	expectedType string,
	expectedChallenge []byte,
	expectedOrigin string,
) error {
	if expectedType == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("expected collected client data type"))
	}

	if len(expectedChallenge) == 0 {
		return altshiftErrors.NewWithTrace(empty_error.New("expected challenge"))
	}

	if expectedOrigin == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("expected origin"))
	}

	if clientData == nil {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, nil_error.New("collected client data"))
	}

	observedType := clientData.Type
	if observedType != expectedType {
		return altshiftErrors.New(
			fmt.Errorf(
				"%w: %w: %q",
				altshiftErrors.ErrValidationError,
				webauthnErrors.ErrCollectedClientDataTypeMismatch,
				observedType,
			),
			observedType,
			expectedType,
		)
	}

	observedChallenge := clientData.Challenge
	if !bytes.Equal(expectedChallenge, observedChallenge) {
		return altshiftErrors.New(
			fmt.Errorf(
				"%w: %w",
				altshiftErrors.ErrValidationError,
				webauthnErrors.ErrChallengeMismatch,
			),
			observedChallenge,
			expectedChallenge,
		)
	}

	observedOrigin := clientData.Origin
	if observedOrigin != expectedOrigin {
		return altshiftErrors.New(
			fmt.Errorf(
				"%w: %w: %q",
				altshiftErrors.ErrValidationError,
				webauthnErrors.ErrOriginMismatch,
				observedOrigin,
			),
			observedOrigin,
			expectedOrigin,
		)
	}

	return nil
}

// ValidateAuthenticatorData validates authenticator data against the expected relying party id,
// signature count, and flag requirements.
func ValidateAuthenticatorData(
	authenticatorData *AuthenticatorData,
	expectedRpId string,
	previousSignatureCount uint32,
	validateAttestedCredential bool,
	verifyUser bool,
) error {
	if expectedRpId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("expected rp id"))
	}

	if authenticatorData == nil {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, nil_error.New("authenticator data"))
	}

	if validateAttestedCredential {
		attestedCredential := authenticatorData.AttestedCredential
		if attestedCredential == nil {
			return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, nil_error.New("attested credential"))
		}

		if len(attestedCredential.CredentialId) == 0 {
			return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("credential id"))
		}

		if len(attestedCredential.RawPublicKey) == 0 || utils.IsNil(attestedCredential.PublicKey) {
			return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("credential public key"))
		}

		if len(attestedCredential.Aaguid) == 0 {
			return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("aaguid"))
		}
	}

	observedRpIdHash := authenticatorData.RpIdHash
	if len(observedRpIdHash) == 0 {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("rp id hash"))
	}

	expectedRpIdHash := sha256.Sum256([]byte(expectedRpId))
	if !bytes.Equal(observedRpIdHash, expectedRpIdHash[:]) {
		return altshiftErrors.New(
			fmt.Errorf(
				"%w: %w",
				altshiftErrors.ErrValidationError,
				webauthnErrors.ErrRpIdHashMismatch,
			),
			observedRpIdHash,
			expectedRpIdHash,
		)
	}

	if !authenticatorData.UserPresent() {
		return fmt.Errorf(
			"%w: %w",
			altshiftErrors.ErrValidationError,
			webauthnErrors.ErrUserNotPresent,
		)
	}

	if verifyUser && !authenticatorData.UserVerified() {
		return fmt.Errorf(
			"%w: %w",
			altshiftErrors.ErrValidationError,
			webauthnErrors.ErrUserNotVerified,
		)
	}

	observedSignCount := authenticatorData.SignCount
	if previousSignatureCount != 0 && observedSignCount <= previousSignatureCount {
		return altshiftErrors.New(
			fmt.Errorf(
				"%w: %w",
				altshiftErrors.ErrValidationError,
				webauthnErrors.ErrUnexpectedSignatureCount,
			),
			observedSignCount,
			previousSignatureCount,
		)
	}

	return nil
}
