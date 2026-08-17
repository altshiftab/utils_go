package dkim

import (
	"crypto"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	altshiftCrypto "github.com/altshiftab/utils_go/pkg/crypto"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

const Prefix = "v=DKIM1"

var ErrBadEd25519Length = errors.New("bad ed25519 length")

type Record struct {
	Version                  int      `json:"version,omitzero"`
	AcceptableHashAlgorithms []string `json:"acceptable_hash_algorithms,omitzero"`
	KeyType                  string   `json:"key_type,omitzero"`
	Notes                    string   `json:"notes,omitzero"`
	PublicKeyData            string   `json:"public_key_data,omitzero"`
	ServiceType              string   `json:"service_type,omitzero"`
	Flags                    []string `json:"flags,omitzero"`

	Raw        string      `json:"raw,omitzero"`
	Domain     string      `json:"domain,omitzero"`
	Selector   string      `json:"selector,omitzero"`
	Extensions [][2]string `json:"extensions,omitzero"`
}

func (r *Record) GetVersion() int {
	if r.Version == 0 {
		return 1
	}

	return r.Version
}

func (r *Record) GetKeyType() string {
	if r.KeyType == "" {
		return "rsa"
	}

	return r.KeyType
}

func (r *Record) GetServiceType() string {
	if r.ServiceType == "" {
		return "*"
	}

	return r.ServiceType
}

func (r *Record) GetPublicKey() (crypto.PublicKey, error) {
	publicKeyData := r.PublicKeyData
	if len(publicKeyData) == 0 {
		return nil, nil
	}

	keyType := r.GetKeyType()
	key, err := ParseKey(publicKeyData, keyType)
	if err != nil {
		return nil, altshiftErrors.New(
			fmt.Errorf("parse key: %w", err),
			publicKeyData, keyType,
		)
	}

	return key, nil
}

type Header struct {
	Version                 int         `json:"version,omitzero"`
	Algorithm               string      `json:"algorithm,omitzero"`
	Signature               string      `json:"signature,omitzero"`
	Hash                    string      `json:"hash,omitzero"`
	MessageCanonicalization string      `json:"message_canonicalization,omitzero"`
	SigningDomainIdentifier string      `json:"signing_domain_identifier,omitzero"`
	SignedHeaderFields      []string    `json:"signed_header_fields,omitzero"`
	AgentOrUserIdentifier   string      `json:"agent_or_user_identifier"`
	BodyLengthCount         string      `json:"body_length_count,omitzero"`
	QueryMethods            []string    `json:"query_methods,omitzero"`
	Selector                string      `json:"selector,omitzero"`
	SignatureTimestamp      string      `json:"signature_timestamp,omitzero"`
	SignatureExpiration     string      `json:"signature_expiration,omitzero"`
	CopiedHeaderFields      [][2]string `json:"copied_header_fields,omitzero"`

	Raw        string      `json:"raw,omitzero"`
	Domain     string      `json:"domain,omitzero"`
	Extensions [][2]string `json:"extensions,omitzero"`
}

func GetKeyData(data string) ([]byte, error) {
	if data == "" {
		return nil, nil
	}

	keyData, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("base64 std encoding decode string: %w", err),
			data,
		)
	}

	return keyData, nil
}

func ParseKey(data string, keyType string) (crypto.PublicKey, error) {
	if data == "" {
		return nil, nil
	}

	if keyType == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("key type"))
	}

	keyData, err := GetKeyData(data)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get key data: %w", err), data)
	}

	switch strings.ToLower(keyType) {
	case "rsa":
		key, err := altshiftCrypto.PublicKeyFromDer[crypto.PublicKey](keyData)
		if err != nil {
			return nil, fmt.Errorf("public key from der: %w", err)
		}

		return key, nil
	case "ed25519":
		if len(keyData) != ed25519.PublicKeySize {
			return nil, altshiftErrors.NewWithTrace(ErrBadEd25519Length, keyData)
		}

		return ed25519.PublicKey(keyData), nil
	default:
		return keyData, nil
	}
}
