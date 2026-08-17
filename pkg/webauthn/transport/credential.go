package transport

import (
	"bytes"
	"crypto/x509"
	"encoding/json/v2"
	"fmt"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/utils"
	"github.com/altshiftab/utils_go/pkg/webauthn"
	webauthnErrors "github.com/altshiftab/utils_go/pkg/webauthn/errors"
)

// CollectedClientData is the wire format of clientDataJSON (WebAuthn §5.8.1).
type CollectedClientData struct {
	Type        string     `json:"type"`
	Challenge   *Base64URL `json:"challenge"`
	Origin      string     `json:"origin"`
	CrossOrigin bool       `json:"crossOrigin,omitzero"`
	// TopOrigin is not part of the specification, but is sent by clients.
	TopOrigin string `json:"topOrigin,omitzero"`
	// TODO: Add `TokenBinding`
}

// CollectedClientDataFromBytes parses raw clientDataJSON into domain client data.
func CollectedClientDataFromBytes(data []byte) (*webauthn.CollectedClientData, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var transportCollectedClientData CollectedClientData
	if err := json.Unmarshal(data, &transportCollectedClientData); err != nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: json unmarshal: %w", altshiftErrors.ErrParseError, err),
		)
	}

	challenge := transportCollectedClientData.Challenge
	if challenge == nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrParseError, nil_error.New("challenge")),
		)
	}

	return &webauthn.CollectedClientData{
		Type:        transportCollectedClientData.Type,
		Challenge:   *challenge,
		Origin:      transportCollectedClientData.Origin,
		CrossOrigin: transportCollectedClientData.CrossOrigin,
		TopOrigin:   transportCollectedClientData.TopOrigin,
	}, nil
}

// AuthenticatorAssertionResponse is the wire format of an authentication ceremony response
// (WebAuthn §5.2.2).
type AuthenticatorAssertionResponse struct {
	ClientDataJson    *Base64URL `json:"clientDataJSON"`
	AuthenticatorData *Base64URL `json:"authenticatorData"`
	Signature         *Base64URL `json:"signature"`
	// NOTE: Not required in spec, but I don't see why it mustn't be.
	UserHandle *Base64URL `json:"userHandle"`
}

func (t AuthenticatorAssertionResponse) GetClientDataJson() []byte {
	if clientDataJson := t.ClientDataJson; clientDataJson != nil {
		return *clientDataJson
	}
	return nil
}

func (t AuthenticatorAssertionResponse) GetAuthenticatorData() []byte {
	if authenticatorData := t.AuthenticatorData; authenticatorData != nil {
		return *authenticatorData
	}
	return nil
}

func (t AuthenticatorAssertionResponse) MakeAuthenticatorResponse() (*webauthn.AuthenticatorAssertionResponse, error) {
	if t.ClientDataJson == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("client data json"))
	}
	if t.AuthenticatorData == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("authenticator data"))
	}
	if t.Signature == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("signature"))
	}
	if t.UserHandle == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("user handle"))
	}

	collectedClientData, err := CollectedClientDataFromBytes(*t.ClientDataJson)
	if err != nil {
		return nil, fmt.Errorf("collected client data from bytes: %w", err)
	}
	if collectedClientData == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("collected client data"))
	}

	authenticatorData, err := webauthn.ParseAuthenticatorData(*t.AuthenticatorData)
	if err != nil {
		return nil, fmt.Errorf("parse authenticator data: %w", err)
	}

	return &webauthn.AuthenticatorAssertionResponse{
		ClientDataJson:    *collectedClientData,
		AuthenticatorData: *authenticatorData,
		Signature:         *t.Signature,
		UserHandle:        *t.UserHandle,
	}, nil
}

// AuthenticatorAttestationResponse is the wire format of a registration ceremony response
// (WebAuthn §5.2.1). The authoritative credential public key is the COSE key within the
// attestation object; the client-derived authenticatorData, publicKey, and publicKeyAlgorithm
// convenience fields are optional and cross-checked against it when present.
type AuthenticatorAttestationResponse struct {
	ClientDataJson    *Base64URL `json:"clientDataJSON"`
	Transports        []string   `json:"transports"`
	AttestationObject *Base64URL `json:"attestationObject"`

	AuthenticatorData  *Base64URL `json:"authenticatorData,omitzero"`
	PublicKey          *Base64URL `json:"publicKey,omitzero"`
	PublicKeyAlgorithm int        `json:"publicKeyAlgorithm,omitzero"`
}

func (t AuthenticatorAttestationResponse) GetClientDataJson() []byte {
	if clientDataJson := t.ClientDataJson; clientDataJson != nil {
		return *clientDataJson
	}
	return nil
}

func (t AuthenticatorAttestationResponse) GetAuthenticatorData() []byte {
	if authenticatorData := t.AuthenticatorData; authenticatorData != nil {
		return *authenticatorData
	}
	return nil
}

func (t AuthenticatorAttestationResponse) MakeAuthenticatorResponse() (*webauthn.AuthenticatorAttestationResponse, error) {
	if t.ClientDataJson == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("client data json"))
	}
	if t.AttestationObject == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("attestation object"))
	}

	collectedClientData, err := CollectedClientDataFromBytes(*t.ClientDataJson)
	if err != nil {
		return nil, fmt.Errorf("collected client data from bytes: %w", err)
	}
	if collectedClientData == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("collected client data"))
	}

	attestationObject, err := webauthn.ParseAttestationObject(*t.AttestationObject)
	if err != nil {
		return nil, fmt.Errorf("parse attestation object: %w", err)
	}

	if authenticatorData := t.AuthenticatorData; authenticatorData != nil {
		if !bytes.Equal(*authenticatorData, attestationObject.RawAuthenticatorData) {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf(
					"%w: %w (attestation object)",
					altshiftErrors.ErrValidationError,
					webauthnErrors.ErrAuthenticatorDataMismatch,
				),
			)
		}
	}

	if attestedCredential := attestationObject.AuthenticatorData.AttestedCredential; attestedCredential != nil {
		if publicKey := t.PublicKey; publicKey != nil {
			if utils.IsNil(attestedCredential.PublicKey) {
				return nil, altshiftErrors.NewWithTrace(nil_error.New("attested credential public key"))
			}

			attestedDer, err := x509.MarshalPKIXPublicKey(attestedCredential.PublicKey)
			if err != nil {
				return nil, altshiftErrors.New(
					fmt.Errorf("x509 marshal pkix public key: %w", err),
					attestedCredential.PublicKey,
				)
			}

			if !bytes.Equal(*publicKey, attestedDer) {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf(
						"%w: %w (attested credential)",
						altshiftErrors.ErrValidationError,
						webauthnErrors.ErrPublicKeyMismatch,
					),
				)
			}
		}

		if publicKeyAlgorithm := t.PublicKeyAlgorithm; publicKeyAlgorithm != 0 {
			if int64(publicKeyAlgorithm) != int64(attestedCredential.PublicKeyAlgorithm) {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf(
						"%w: %w (attested credential)",
						altshiftErrors.ErrValidationError,
						webauthnErrors.ErrPublicKeyAlgorithmMismatch,
					),
					publicKeyAlgorithm,
					attestedCredential.PublicKeyAlgorithm,
				)
			}
		}
	}

	return &webauthn.AuthenticatorAttestationResponse{
		ClientDataJson:    *collectedClientData,
		AttestationObject: attestationObject,
		Transports:        t.Transports,
	}, nil
}

type AssertionPublicKeyCredential = PublicKeyCredential[AuthenticatorAssertionResponse]
type AttestationPublicKeyCredential = PublicKeyCredential[AuthenticatorAttestationResponse]

// PublicKeyCredential is the wire format of a credential from a navigator.credentials call
// (WebAuthn §5.1).
type PublicKeyCredential[T AuthenticatorAttestationResponse | AuthenticatorAssertionResponse] struct {
	Id              *Base64URL     `json:"id"`
	Type            string         `json:"type"`
	RawId           *Base64URL     `json:"rawId"`
	Response        T              `json:"response"`
	ClientExtension map[string]any `json:"clientExtension,omitzero"`
}

// MakeAttestationPublicKeyCredential converts a wire-format registration credential into its
// parsed domain counterpart.
func MakeAttestationPublicKeyCredential(
	transportCredential *AttestationPublicKeyCredential,
) (*webauthn.AttestationPublicKeyCredential, error) {
	if transportCredential == nil {
		return nil, nil
	}

	id := transportCredential.Id
	if id == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("id"))
	}

	rawId := transportCredential.RawId
	if rawId == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("raw id"))
	}

	transportResponse := transportCredential.Response
	authenticatorResponse, err := transportResponse.MakeAuthenticatorResponse()
	if err != nil {
		return nil, altshiftErrors.New(
			fmt.Errorf("transport make authenticator response: %w", err),
			transportResponse,
		)
	}

	return &webauthn.AttestationPublicKeyCredential{
		Id:              *id,
		Type:            transportCredential.Type,
		RawId:           *rawId,
		Response:        *authenticatorResponse,
		ClientExtension: transportCredential.ClientExtension,
	}, nil
}

// MakeAssertionPublicKeyCredential converts a wire-format authentication credential into its
// parsed domain counterpart.
func MakeAssertionPublicKeyCredential(
	transportCredential *AssertionPublicKeyCredential,
) (*webauthn.AssertionPublicKeyCredential, error) {
	if transportCredential == nil {
		return nil, nil
	}

	id := transportCredential.Id
	if id == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("id"))
	}

	rawId := transportCredential.RawId
	if rawId == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("raw id"))
	}

	transportResponse := transportCredential.Response
	authenticatorResponse, err := transportResponse.MakeAuthenticatorResponse()
	if err != nil {
		return nil, altshiftErrors.New(
			fmt.Errorf("transport make authenticator response: %w", err),
			transportResponse,
		)
	}

	return &webauthn.AssertionPublicKeyCredential{
		Id:              *id,
		Type:            transportCredential.Type,
		RawId:           *rawId,
		Response:        *authenticatorResponse,
		ClientExtension: transportCredential.ClientExtension,
	}, nil
}
