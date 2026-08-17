// Package jwe implements JSON Web Encryption (RFC 7516) compact
// serialization for the ECDH-ES key agreement algorithm (RFC 7518
// Section 4.6) with A256GCM content encryption (RFC 7518 Section 5.3),
// using NIST P-256, P-384 and P-521 keys.
package jwe

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key"
)

type KeyAlgorithm string

const KeyAlgorithmEcdhEs KeyAlgorithm = "ECDH-ES"

type ContentEncryption string

const ContentEncryptionA256Gcm ContentEncryption = "A256GCM"

const (
	initializationVectorSize = 12
	tagSize                  = 16
	contentEncryptionKeyBits = 256
)

var (
	ErrUnsupportedKeyAlgorithm      = errors.New("unsupported key algorithm")
	ErrUnsupportedContentEncryption = errors.New("unsupported content encryption")
	ErrUnsupportedKeyType           = errors.New("unsupported key type")
	ErrUnsupportedCompression       = errors.New("unsupported compression")
	ErrUnexpectedEncryptedKey       = errors.New("unexpected encrypted key for direct key agreement")
)

// Header is a JWE protected header.
type Header struct {
	Algorithm           KeyAlgorithm      `json:"alg"`
	ContentEncryption   ContentEncryption `json:"enc"`
	EphemeralPublicKey  *key.Key          `json:"epk,omitzero"`
	KeyId               string            `json:"kid,omitzero"`
	ContentType         string            `json:"cty,omitzero"`
	AgreementPartyUInfo string            `json:"apu,omitzero"`
	AgreementPartyVInfo string            `json:"apv,omitzero"`
	Compression         string            `json:"zip,omitzero"`
}

// UnmarshalJSON exists because key.Key is constructed from a map rather
// than unmarshalled directly.
func (header *Header) UnmarshalJSON(data []byte) error {
	var raw struct {
		Algorithm           KeyAlgorithm      `json:"alg"`
		ContentEncryption   ContentEncryption `json:"enc"`
		EphemeralPublicKey  map[string]any    `json:"epk"`
		KeyId               string            `json:"kid"`
		ContentType         string            `json:"cty"`
		AgreementPartyUInfo string            `json:"apu"`
		AgreementPartyVInfo string            `json:"apv"`
		Compression         string            `json:"zip"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal: %w", err))
	}

	var ephemeralPublicKey *key.Key
	if raw.EphemeralPublicKey != nil {
		var err error
		ephemeralPublicKey, err = key.New(raw.EphemeralPublicKey)
		if err != nil {
			return altshiftErrors.New(fmt.Errorf("key new (epk): %w", err), raw.EphemeralPublicKey)
		}
	}

	*header = Header{
		Algorithm:           raw.Algorithm,
		ContentEncryption:   raw.ContentEncryption,
		EphemeralPublicKey:  ephemeralPublicKey,
		KeyId:               raw.KeyId,
		ContentType:         raw.ContentType,
		AgreementPartyUInfo: raw.AgreementPartyUInfo,
		AgreementPartyVInfo: raw.AgreementPartyVInfo,
		Compression:         raw.Compression,
	}

	return nil
}

// concatKdf derives key material with the Concat KDF (NIST SP 800-56A
// Section 5.8.1) as profiled by RFC 7518 Section 4.6.2. A single SHA-256
// round suffices for the supported key sizes.
func concatKdf(z []byte, algorithmId string, partyUInfo []byte, partyVInfo []byte, keyDataLengthBits uint32) ([]byte, error) {
	hash := sha256.New()
	hash.Write(binary.BigEndian.AppendUint32(nil, 1))
	hash.Write(z)

	for _, field := range [][]byte{[]byte(algorithmId), partyUInfo, partyVInfo} {
		fieldLength := len(field)
		if uint64(fieldLength) > math.MaxUint32 {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: field too long: %d", altshiftErrors.ErrValidationError, fieldLength),
			)
		}
		hash.Write(binary.BigEndian.AppendUint32(nil, uint32(fieldLength)))
		hash.Write(field)
	}

	hash.Write(binary.BigEndian.AppendUint32(nil, keyDataLengthBits))

	return hash.Sum(nil)[:keyDataLengthBits/8], nil
}

func ecdhPublicKey(publicKey *ecdsa.PublicKey) (*ecdh.PublicKey, error) {
	if publicKey == nil {
		return nil, nil_error.New("public key")
	}

	ecdhKey, err := publicKey.ECDH()
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("ecdsa public key ecdh: %w", err))
	}

	return ecdhKey, nil
}

// makeContentEncryptionKey performs the ECDH-ES direct key agreement and
// derives the content encryption key.
func makeContentEncryptionKey(
	privateKey *ecdh.PrivateKey,
	publicKey *ecdh.PublicKey,
	contentEncryption ContentEncryption,
	partyUInfo []byte,
	partyVInfo []byte,
) ([]byte, error) {
	sharedSecret, err := privateKey.ECDH(publicKey)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("ecdh: %w", err))
	}

	contentEncryptionKey, err := concatKdf(
		sharedSecret,
		string(contentEncryption),
		partyUInfo,
		partyVInfo,
		contentEncryptionKeyBits,
	)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("concat kdf: %w", err))
	}

	return contentEncryptionKey, nil
}

func makeAead(contentEncryptionKey []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(contentEncryptionKey)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("aes new cipher: %w", err))
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("cipher new gcm: %w", err))
	}

	return aead, nil
}

// Encrypter encrypts plaintexts to a recipient public key, producing JWE
// compact serializations.
type Encrypter struct {
	KeyAlgorithm       KeyAlgorithm
	ContentEncryption  ContentEncryption
	RecipientPublicKey *ecdsa.PublicKey
	KeyId              string
	ContentType        string
}

// NewEncrypter validates the algorithms and the recipient public key and
// returns an Encrypter.
func NewEncrypter(
	keyAlgorithm KeyAlgorithm,
	contentEncryption ContentEncryption,
	recipientPublicKey *ecdsa.PublicKey,
) (*Encrypter, error) {
	if keyAlgorithm != KeyAlgorithmEcdhEs {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %q", ErrUnsupportedKeyAlgorithm, keyAlgorithm))
	}
	if contentEncryption != ContentEncryptionA256Gcm {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %q", ErrUnsupportedContentEncryption, contentEncryption))
	}
	if recipientPublicKey == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("recipient public key"))
	}

	if _, err := ecdhPublicKey(recipientPublicKey); err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("ecdh public key: %w", err))
	}

	return &Encrypter{
		KeyAlgorithm:       keyAlgorithm,
		ContentEncryption:  contentEncryption,
		RecipientPublicKey: recipientPublicKey,
	}, nil
}

// Encrypt encrypts plaintext and returns the JWE compact serialization.
func (encrypter *Encrypter) Encrypt(plaintext []byte) (string, error) {
	if encrypter.KeyAlgorithm != KeyAlgorithmEcdhEs {
		return "", altshiftErrors.NewWithTrace(fmt.Errorf("%w: %q", ErrUnsupportedKeyAlgorithm, encrypter.KeyAlgorithm))
	}
	if encrypter.ContentEncryption != ContentEncryptionA256Gcm {
		return "", altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %q", ErrUnsupportedContentEncryption, encrypter.ContentEncryption),
		)
	}

	recipientPublicKey := encrypter.RecipientPublicKey
	if recipientPublicKey == nil {
		return "", altshiftErrors.NewWithTrace(nil_error.New("recipient public key"))
	}

	recipientEcdhPublicKey, err := ecdhPublicKey(recipientPublicKey)
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("ecdh public key (recipient): %w", err))
	}

	ephemeralPrivateKey, err := ecdsa.GenerateKey(recipientPublicKey.Curve, rand.Reader)
	if err != nil {
		return "", altshiftErrors.NewWithTrace(fmt.Errorf("ecdsa generate key: %w", err))
	}

	ephemeralEcdhPrivateKey, err := ephemeralPrivateKey.ECDH()
	if err != nil {
		return "", altshiftErrors.NewWithTrace(fmt.Errorf("ecdsa private key ecdh: %w", err))
	}

	contentEncryptionKey, err := makeContentEncryptionKey(
		ephemeralEcdhPrivateKey,
		recipientEcdhPublicKey,
		encrypter.ContentEncryption,
		nil,
		nil,
	)
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("make content encryption key: %w", err))
	}

	ephemeralPublicKey, err := key.NewFromPublicKey(&ephemeralPrivateKey.PublicKey, "", "", "")
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("key new from public key (ephemeral): %w", err))
	}

	headerData, err := json.Marshal(
		&Header{
			Algorithm:          encrypter.KeyAlgorithm,
			ContentEncryption:  encrypter.ContentEncryption,
			EphemeralPublicKey: ephemeralPublicKey,
			KeyId:              encrypter.KeyId,
			ContentType:        encrypter.ContentType,
		},
	)
	if err != nil {
		return "", altshiftErrors.NewWithTrace(fmt.Errorf("json marshal (header): %w", err))
	}
	protected := base64.RawURLEncoding.EncodeToString(headerData)

	initializationVector := make([]byte, initializationVectorSize)
	if _, err := rand.Read(initializationVector); err != nil {
		return "", altshiftErrors.NewWithTrace(fmt.Errorf("rand read (initialization vector): %w", err))
	}

	aead, err := makeAead(contentEncryptionKey)
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("make aead: %w", err))
	}

	sealed := aead.Seal(make([]byte, 0, len(plaintext)+tagSize), initializationVector, plaintext, []byte(protected))
	ciphertext := sealed[:len(sealed)-tagSize]
	tag := sealed[len(sealed)-tagSize:]

	return strings.Join(
		[]string{
			protected,
			"",
			base64.RawURLEncoding.EncodeToString(initializationVector),
			base64.RawURLEncoding.EncodeToString(ciphertext),
			base64.RawURLEncoding.EncodeToString(tag),
		},
		".",
	), nil
}

// Encryption is a parsed JWE.
type Encryption struct {
	// Header is the parsed protected header.
	Header *Header

	protected            string
	initializationVector []byte
	ciphertext           []byte
	tag                  []byte
	agreementPartyUInfo  []byte
	agreementPartyVInfo  []byte
}

func decodePart(serialization string, name string) ([]byte, error) {
	data, err := base64.RawURLEncoding.DecodeString(serialization)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: base64 raw url encoding decode string (%s): %w", altshiftErrors.ErrParseError, name, err),
		)
	}

	return data, nil
}

// ParseCompact parses a JWE compact serialization, requiring the key
// algorithm and content encryption of the protected header to be in the
// given allowlists.
func ParseCompact(
	serialization string,
	allowedKeyAlgorithms []KeyAlgorithm,
	allowedContentEncryptions []ContentEncryption,
) (*Encryption, error) {
	if serialization == "" {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrParseError, empty_error.New("serialization")),
		)
	}

	parts := strings.Split(serialization, ".")
	if len(parts) != 5 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: unexpected number of parts: %d", altshiftErrors.ErrParseError, len(parts)),
		)
	}

	protected := parts[0]
	headerData, err := decodePart(protected, "protected header")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("decode part (protected header): %w", err))
	}

	var header Header
	if err := json.Unmarshal(headerData, &header); err != nil {
		return nil, altshiftErrors.New(
			fmt.Errorf("%w: json unmarshal (protected header): %w", altshiftErrors.ErrParseError, err),
			string(headerData),
		)
	}

	if !slices.Contains(allowedKeyAlgorithms, header.Algorithm) {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w: %q", altshiftErrors.ErrValidationError, ErrUnsupportedKeyAlgorithm, header.Algorithm),
		)
	}
	if !slices.Contains(allowedContentEncryptions, header.ContentEncryption) {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: %w: %q",
				altshiftErrors.ErrValidationError, ErrUnsupportedContentEncryption, header.ContentEncryption,
			),
		)
	}
	if header.Compression != "" {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w: %q", altshiftErrors.ErrValidationError, ErrUnsupportedCompression, header.Compression),
		)
	}
	if header.EphemeralPublicKey == nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, nil_error.New("epk")),
		)
	}

	// ECDH-ES uses direct key agreement; the encrypted key part must be empty.
	if parts[1] != "" {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, ErrUnexpectedEncryptedKey),
		)
	}

	initializationVector, err := decodePart(parts[2], "initialization vector")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("decode part (initialization vector): %w", err))
	}
	if len(initializationVector) != initializationVectorSize {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: unexpected initialization vector length: %d",
				altshiftErrors.ErrParseError, len(initializationVector),
			),
		)
	}

	ciphertext, err := decodePart(parts[3], "ciphertext")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("decode part (ciphertext): %w", err))
	}

	tag, err := decodePart(parts[4], "tag")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("decode part (tag): %w", err))
	}
	if len(tag) != tagSize {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: unexpected tag length: %d", altshiftErrors.ErrParseError, len(tag)),
		)
	}

	agreementPartyUInfo, err := decodePart(header.AgreementPartyUInfo, "apu")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("decode part (apu): %w", err))
	}

	agreementPartyVInfo, err := decodePart(header.AgreementPartyVInfo, "apv")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("decode part (apv): %w", err))
	}

	return &Encryption{
		Header:               &header,
		protected:            protected,
		initializationVector: initializationVector,
		ciphertext:           ciphertext,
		tag:                  tag,
		agreementPartyUInfo:  agreementPartyUInfo,
		agreementPartyVInfo:  agreementPartyVInfo,
	}, nil
}

func ecdhPrivateKey(privateKey any) (*ecdh.PrivateKey, error) {
	switch typedPrivateKey := privateKey.(type) {
	case *ecdh.PrivateKey:
		return typedPrivateKey, nil
	case *ecdsa.PrivateKey:
		ecdhKey, err := typedPrivateKey.ECDH()
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("ecdsa private key ecdh: %w", err))
		}
		return ecdhKey, nil
	case nil:
		return nil, altshiftErrors.NewWithTrace(nil_error.New("private key"))
	default:
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %T", ErrUnsupportedKeyType, privateKey))
	}
}

// Decrypt decrypts the JWE with the recipient private key, which must be
// a *ecdsa.PrivateKey or a *ecdh.PrivateKey. Failures caused by the
// content not matching the key match altshiftErrors.ErrVerificationError
// with errors.Is.
func (encryption *Encryption) Decrypt(privateKey any) ([]byte, error) {
	recipientEcdhPrivateKey, err := ecdhPrivateKey(privateKey)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("ecdh private key: %w", err))
	}

	header := encryption.Header
	if header == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("header"))
	}
	if header.EphemeralPublicKey == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("epk"))
	}

	ephemeralPublicKey, err := header.EphemeralPublicKey.Material.PublicKey()
	if err != nil {
		return nil, altshiftErrors.New(
			fmt.Errorf("%w: public key (epk): %w", altshiftErrors.ErrValidationError, err),
		)
	}

	ephemeralEcdsaPublicKey, ok := ephemeralPublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w (epk): %T", altshiftErrors.ErrValidationError, ErrUnsupportedKeyType, ephemeralPublicKey),
		)
	}

	ephemeralEcdhPublicKey, err := ephemeralEcdsaPublicKey.ECDH()
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: ecdsa public key ecdh (epk): %w", altshiftErrors.ErrValidationError, err),
		)
	}

	contentEncryptionKey, err := makeContentEncryptionKey(
		recipientEcdhPrivateKey,
		ephemeralEcdhPublicKey,
		header.ContentEncryption,
		encryption.agreementPartyUInfo,
		encryption.agreementPartyVInfo,
	)
	if err != nil {
		return nil, altshiftErrors.New(
			fmt.Errorf("%w: make content encryption key: %w", altshiftErrors.ErrVerificationError, err),
		)
	}

	aead, err := makeAead(contentEncryptionKey)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("make aead: %w", err))
	}

	sealed := slices.Concat(encryption.ciphertext, encryption.tag)
	plaintext, err := aead.Open(
		make([]byte, 0, len(encryption.ciphertext)),
		encryption.initializationVector,
		sealed,
		[]byte(encryption.protected),
	)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: aead open: %w", altshiftErrors.ErrVerificationError, err),
		)
	}

	return plaintext, nil
}
