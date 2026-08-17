package transport

import (
	"github.com/altshiftab/utils_go/pkg/webauthn"
)

// PublicKeyCredentialUserEntity is the wire format of a user entity (WebAuthn §5.4.3).
type PublicKeyCredentialUserEntity struct {
	Name        string     `json:"name"`
	Id          *Base64URL `json:"id"`
	DisplayName string     `json:"displayName"`
}

// PublicKeyCredentialCreationOptions is the wire format of registration ceremony options
// (WebAuthn §5.4).
type PublicKeyCredentialCreationOptions struct {
	RelyingParty *webauthn.RelyingParty         `json:"rp"`
	User         *PublicKeyCredentialUserEntity `json:"user"`

	Challenge        *Base64URL                           `json:"challenge"`
	PubKeyCredParams []*webauthn.PublicKeyCredentialParam `json:"pubKeyCredParams"`

	Timeout                uint64                                    `json:"timeout,omitzero"`
	ExcludeCredentials     []*webauthn.PublicKeyCredentialDescriptor `json:"excludeCredentials,omitzero"`
	AuthenticatorSelection *webauthn.AuthenticatorSelection          `json:"authenticatorSelection,omitzero"`
	Attestation            string                                    `json:"attestation,omitzero"`
	Extensions             map[string]any                            `json:"extensions,omitzero"`
}

// PublicKeyCredentialRequestOptions is the wire format of authentication ceremony options
// (WebAuthn §5.5).
type PublicKeyCredentialRequestOptions struct {
	Challenge          *Base64URL                                `json:"challenge"`
	Timeout            uint64                                    `json:"timeout,omitzero"`
	RpId               string                                    `json:"rpId,omitzero"`
	AllowedCredentials []*webauthn.PublicKeyCredentialDescriptor `json:"allowedCredentials,omitzero"`
	UserVerification   string                                    `json:"userVerification,omitzero"`
	Extensions         map[string]any                            `json:"extensions,omitzero"`
}
