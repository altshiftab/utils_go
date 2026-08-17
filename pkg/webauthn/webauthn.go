// Package webauthn implements the server-side (relying party) parts of WebAuthn used for
// passkey registration and authentication: ceremony option types, credential wire-format
// parsing (see the transport subpackage), CBOR attestation-object and authenticator-data
// parsing with COSE credential-key extraction, and ceremony validation.
//
// Attestation statements are verified on demand via VerifyAttestationStatement ("none",
// "packed", "apple", and "fido-u2f" formats); the validation functions themselves do not
// evaluate attestation, matching relying parties that request attestation "none".
package webauthn

import (
	"encoding/json/v2"
	"fmt"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// CollectedClientData is the parsed client data of a ceremony (WebAuthn §5.8.1).
type CollectedClientData struct {
	Type        string
	Challenge   []byte
	Origin      string
	CrossOrigin bool
	// TopOrigin is not part of the specification, but is sent by clients.
	TopOrigin string
}

// AuthenticatorAssertionResponse is an authenticator's response to an authentication ceremony
// (WebAuthn §5.2.2).
type AuthenticatorAssertionResponse struct {
	ClientDataJson    CollectedClientData
	AuthenticatorData AuthenticatorData
	Signature         []byte
	UserHandle        []byte
}

func (a AuthenticatorAssertionResponse) GetClientDataJson() *CollectedClientData {
	return &a.ClientDataJson
}

func (a AuthenticatorAssertionResponse) GetAuthenticatorData() *AuthenticatorData {
	return &a.AuthenticatorData
}

// AuthenticatorAttestationResponse is an authenticator's response to a registration ceremony
// (WebAuthn §5.2.1). The authenticator data is the one embedded in the attestation object.
type AuthenticatorAttestationResponse struct {
	ClientDataJson    CollectedClientData
	AttestationObject *AttestationObject
	Transports        []string
}

func (a AuthenticatorAttestationResponse) GetClientDataJson() *CollectedClientData {
	return &a.ClientDataJson
}

func (a AuthenticatorAttestationResponse) GetAuthenticatorData() *AuthenticatorData {
	if attestationObject := a.AttestationObject; attestationObject != nil {
		return attestationObject.AuthenticatorData
	}
	return nil
}

// AuthenticatorResponse is implemented by both authenticator response types.
type AuthenticatorResponse interface {
	GetClientDataJson() *CollectedClientData
	GetAuthenticatorData() *AuthenticatorData
}

type AssertionPublicKeyCredential = PublicKeyCredential[AuthenticatorAssertionResponse]
type AttestationPublicKeyCredential = PublicKeyCredential[AuthenticatorAttestationResponse]

// PublicKeyCredential is a parsed credential from a navigator.credentials call (WebAuthn §5.1).
type PublicKeyCredential[T AuthenticatorAttestationResponse | AuthenticatorAssertionResponse] struct {
	Id              []byte
	Type            string
	RawId           []byte
	Response        T
	ClientExtension map[string]any
}

// RelyingParty identifies the relying party in creation options (WebAuthn §5.4.2).
type RelyingParty struct {
	Name string `json:"name"`
	Id   string `json:"id,omitzero"`
}

// PublicKeyCredentialParam expresses an accepted credential type and COSE algorithm
// (WebAuthn §5.3).
type PublicKeyCredentialParam struct {
	Type string `json:"type"`
	Alg  int    `json:"alg"`
}

// AuthenticatorSelection restricts which authenticators may take part in a registration
// ceremony (WebAuthn §5.4.4).
type AuthenticatorSelection struct {
	AuthenticatorAttachment string `json:"authenticatorAttachment,omitzero"`
	ResidentKey             string `json:"residentKey,omitzero"`
	RequireResidentKey      bool   `json:"requireResidentKey,omitzero"`
}

func (a *AuthenticatorSelection) MarshalJSON() ([]byte, error) {
	type Alias AuthenticatorSelection
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(a),
	}

	aux.RequireResidentKey = a.ResidentKey == "required"

	data, err := json.Marshal(aux)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json marshal: %w", err))
	}

	return data, nil
}

// PublicKeyCredentialUserEntity identifies the user account a credential is registered to
// (WebAuthn §5.4.3).
type PublicKeyCredentialUserEntity struct {
	Name        string `json:"name"`
	Id          []byte `json:"id"`
	DisplayName string `json:"displayName"`
}

// PublicKeyCredentialDescriptor identifies an existing credential (WebAuthn §5.8.3).
type PublicKeyCredentialDescriptor struct {
	Id         string   `json:"id"`
	Type       string   `json:"type"`
	Transports []string `json:"transports,omitzero"`
}

// PublicKeyCredentialCreationOptions are the options of a registration ceremony
// (WebAuthn §5.4).
type PublicKeyCredentialCreationOptions struct {
	RelyingParty *RelyingParty                  `json:"rp"`
	User         *PublicKeyCredentialUserEntity `json:"user"`

	Challenge        []byte                      `json:"challenge"`
	PubKeyCredParams []*PublicKeyCredentialParam `json:"pubKeyCredParams"`

	Timeout                uint64                           `json:"timeout,omitzero"`
	ExcludeCredentials     []*PublicKeyCredentialDescriptor `json:"excludeCredentials,omitzero"`
	AuthenticatorSelection *AuthenticatorSelection          `json:"authenticatorSelection,omitzero"`
	Attestation            string                           `json:"attestation,omitzero"`
	Extensions             map[string]any                   `json:"extensions,omitzero"`
}

// PublicKeyCredentialRequestOptions are the options of an authentication ceremony
// (WebAuthn §5.5).
type PublicKeyCredentialRequestOptions struct {
	Challenge          []byte                           `json:"challenge"`
	Timeout            uint64                           `json:"timeout,omitzero"`
	RpId               string                           `json:"rpId,omitzero"`
	AllowedCredentials []*PublicKeyCredentialDescriptor `json:"allowedCredentials,omitzero"`
	UserVerification   string                           `json:"userVerification,omitzero"`
	Extensions         map[string]any                   `json:"extensions,omitzero"`
}
