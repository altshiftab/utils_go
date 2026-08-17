package webauthn

import (
	"crypto"
	"encoding/binary"
	"fmt"

	"github.com/altshiftab/utils_go/pkg/cbor"
	"github.com/altshiftab/utils_go/pkg/cose"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// Authenticator data flag bits (WebAuthn §6.1).
const (
	FlagUserPresent            byte = 0x01
	FlagUserVerified           byte = 0x04
	FlagBackupEligible         byte = 0x08
	FlagBackedUp               byte = 0x10
	FlagAttestedCredentialData byte = 0x40
	FlagExtensionData          byte = 0x80
)

const (
	rpIdHashLength             = 32
	minAuthenticatorDataLength = rpIdHashLength + 1 + 4
	aaguidLength               = 16
	credentialIdLengthLength   = 2
)

// AttestedCredentialData is the attested credential data of a registration ceremony's
// authenticator data (WebAuthn §6.5.1).
type AttestedCredentialData struct {
	Aaguid       []byte
	CredentialId []byte
	// PublicKey is the parsed COSE credential public key.
	PublicKey crypto.PublicKey
	// PublicKeyAlgorithm is the alg parameter of the COSE credential public key, or 0 when the
	// key carries none.
	PublicKeyAlgorithm cose.Algorithm
	// RawPublicKey is the CBOR-encoded COSE_Key exactly as carried in the authenticator data.
	RawPublicKey []byte
}

// AuthenticatorData is parsed authenticator data (WebAuthn §6.1).
type AuthenticatorData struct {
	RpIdHash           []byte
	Flags              byte
	SignCount          uint32
	AttestedCredential *AttestedCredentialData
	Extensions         map[any]any
}

func (a *AuthenticatorData) UserPresent() bool {
	return a.Flags&FlagUserPresent != 0
}

func (a *AuthenticatorData) UserVerified() bool {
	return a.Flags&FlagUserVerified != 0
}

// ParseAuthenticatorData parses the binary authenticator data format. The embedded COSE
// credential public key is delimited by CBOR decoding, separating it from any extension data
// that follows it.
func ParseAuthenticatorData(data []byte) (*AuthenticatorData, error) {
	if len(data) < minAuthenticatorDataLength {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: authenticator data too short (%d bytes)", motmedelErrors.ErrParseError, len(data)),
		)
	}

	authenticatorData := &AuthenticatorData{
		RpIdHash:  data[:rpIdHashLength],
		Flags:     data[rpIdHashLength],
		SignCount: binary.BigEndian.Uint32(data[rpIdHashLength+1 : minAuthenticatorDataLength]),
	}

	rest := data[minAuthenticatorDataLength:]

	if authenticatorData.Flags&FlagAttestedCredentialData != 0 {
		if len(rest) < aaguidLength+credentialIdLengthLength {
			return nil, motmedelErrors.NewWithTrace(
				fmt.Errorf("%w: authenticator data too short for attested credential data", motmedelErrors.ErrParseError),
			)
		}

		aaguid := rest[:aaguidLength]
		credentialIdLength := int(binary.BigEndian.Uint16(rest[aaguidLength : aaguidLength+credentialIdLengthLength]))
		rest = rest[aaguidLength+credentialIdLengthLength:]

		if len(rest) < credentialIdLength {
			return nil, motmedelErrors.NewWithTrace(
				fmt.Errorf("%w: credential id length exceeds authenticator data", motmedelErrors.ErrParseError),
			)
		}
		credentialId := rest[:credentialIdLength]
		rest = rest[credentialIdLength:]

		keyValue, remaining, err := cbor.DecodeFirst(rest)
		if err != nil {
			return nil, motmedelErrors.New(
				fmt.Errorf("%w: cbor decode first (credential public key): %w", motmedelErrors.ErrParseError, err),
				rest,
			)
		}

		keyMap, ok := keyValue.(map[any]any)
		if !ok {
			return nil, motmedelErrors.NewWithTrace(
				fmt.Errorf("%w: credential public key is not a map", motmedelErrors.ErrParseError),
			)
		}

		publicKey, err := cose.PublicKey(keyMap)
		if err != nil {
			return nil, motmedelErrors.New(
				fmt.Errorf("%w: cose public key: %w", motmedelErrors.ErrParseError, err),
				keyMap,
			)
		}

		publicKeyAlgorithm, _ := cose.KeyAlgorithm(keyMap)

		authenticatorData.AttestedCredential = &AttestedCredentialData{
			Aaguid:             aaguid,
			CredentialId:       credentialId,
			PublicKey:          publicKey,
			PublicKeyAlgorithm: publicKeyAlgorithm,
			RawPublicKey:       rest[:len(rest)-len(remaining)],
		}

		rest = remaining
	}

	if authenticatorData.Flags&FlagExtensionData != 0 {
		extensionsValue, err := cbor.Decode(rest)
		if err != nil {
			return nil, motmedelErrors.New(
				fmt.Errorf("%w: cbor decode (extensions): %w", motmedelErrors.ErrParseError, err),
				rest,
			)
		}

		extensions, ok := extensionsValue.(map[any]any)
		if !ok {
			return nil, motmedelErrors.NewWithTrace(
				fmt.Errorf("%w: extensions are not a map", motmedelErrors.ErrParseError),
			)
		}

		authenticatorData.Extensions = extensions
	} else if len(rest) != 0 {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: trailing authenticator data", motmedelErrors.ErrParseError),
		)
	}

	return authenticatorData, nil
}
