package webauthn

import (
	"fmt"

	"github.com/altshiftab/utils_go/pkg/cbor"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// AttestationObject is a parsed attestation object (WebAuthn §6.5.4). The attestation statement
// is kept as decoded CBOR without being verified.
type AttestationObject struct {
	Format               string
	AttestationStatement map[any]any
	AuthenticatorData    *AuthenticatorData
	RawAuthenticatorData []byte
}

// ParseAttestationObject parses a CBOR-encoded attestation object and the authenticator data
// within it.
func ParseAttestationObject(data []byte) (*AttestationObject, error) {
	value, err := cbor.Decode(data)
	if err != nil {
		return nil, altshiftErrors.New(
			fmt.Errorf("%w: cbor decode: %w", altshiftErrors.ErrParseError, err),
			data,
		)
	}

	attestationObjectMap, ok := value.(map[any]any)
	if !ok {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: attestation object is not a map", altshiftErrors.ErrParseError),
		)
	}

	format, ok := attestationObjectMap["fmt"].(string)
	if !ok || format == "" {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: missing attestation statement format", altshiftErrors.ErrParseError),
		)
	}

	attestationStatement, ok := attestationObjectMap["attStmt"].(map[any]any)
	if !ok {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: missing attestation statement", altshiftErrors.ErrParseError),
		)
	}

	rawAuthenticatorData, ok := attestationObjectMap["authData"].([]byte)
	if !ok || len(rawAuthenticatorData) == 0 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: missing authenticator data", altshiftErrors.ErrParseError),
		)
	}

	authenticatorData, err := ParseAuthenticatorData(rawAuthenticatorData)
	if err != nil {
		return nil, altshiftErrors.New(
			fmt.Errorf("parse authenticator data: %w", err),
			rawAuthenticatorData,
		)
	}

	return &AttestationObject{
		Format:               format,
		AttestationStatement: attestationStatement,
		AuthenticatorData:    authenticatorData,
		RawAuthenticatorData: rawAuthenticatorData,
	}, nil
}
