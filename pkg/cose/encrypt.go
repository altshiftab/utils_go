package cose

import (
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/altshiftab/utils_go/pkg/cbor"
)

type EncryptOptions struct {
	// ContentEncryptionAlgorithm defaults to A256GCM.
	ContentEncryptionAlgorithm Algorithm
	// KeyIdentifier identifies the recipient public key and is placed in the recipient
	// unprotected header.
	KeyIdentifier []byte
	// ContentType describes the plaintext and is placed in the content protected header. It must
	// be a media type string or a CoAP Content-Format integer.
	ContentType any
	// ExternalAad is additional authenticated data not carried in the message.
	ExternalAad []byte
	// Rand defaults to crypto/rand.Reader.
	Rand io.Reader
}

// Encrypt produces a COSE_Encrypt message (CBOR tag 96) for a single recipient, using direct
// ECDH-ES key agreement with HKDF-256 and the configured content-encryption algorithm.
func Encrypt(plaintext []byte, recipientPublicKey *ecdh.PublicKey, options *EncryptOptions) ([]byte, error) {
	if recipientPublicKey == nil {
		return nil, fmt.Errorf("%w: nil recipient public key", ErrMalformedKey)
	}

	if options == nil {
		options = &EncryptOptions{}
	}

	contentAlgorithm := options.ContentEncryptionAlgorithm
	if contentAlgorithm == 0 {
		contentAlgorithm = AlgorithmA256GCM
	}

	contentEncryption, ok := contentEncryptionRegistry[contentAlgorithm]
	if !ok {
		return nil, fmt.Errorf("%w: content encryption %d", ErrUnsupportedAlgorithm, contentAlgorithm)
	}

	randReader := options.Rand
	if randReader == nil {
		randReader = rand.Reader
	}

	contentProtectedMap := map[int64]any{HeaderLabelAlgorithm: int64(contentAlgorithm)}
	if contentType := options.ContentType; contentType != nil {
		switch contentType.(type) {
		case string, int, int64, uint64:
			contentProtectedMap[HeaderLabelContentType] = contentType
		default:
			return nil, fmt.Errorf("%w: content type must be a string or an integer", ErrMalformedMessage)
		}
	}

	contentProtected, err := encodeHeaderMap(contentProtectedMap)
	if err != nil {
		return nil, fmt.Errorf("encode header map (content protected): %w", err)
	}

	recipientProtected, err := encodeHeaderMap(
		map[int64]any{HeaderLabelAlgorithm: int64(AlgorithmEcdhEsHkdf256)},
	)
	if err != nil {
		return nil, fmt.Errorf("encode header map (recipient protected): %w", err)
	}

	ephemeralPrivateKey, err := recipientPublicKey.Curve().GenerateKey(randReader)
	if err != nil {
		return nil, fmt.Errorf("ecdh generate key: %w", err)
	}

	sharedSecret, err := ephemeralPrivateKey.ECDH(recipientPublicKey)
	if err != nil {
		return nil, fmt.Errorf("ecdh: %w", err)
	}

	context, err := kdfContext(contentAlgorithm, contentEncryption.KeyBits, recipientProtected)
	if err != nil {
		return nil, fmt.Errorf("kdf context: %w", err)
	}

	contentEncryptionKey, err := hkdf.Key(sha256.New, sharedSecret, nil, string(context), contentEncryption.KeyBits/8)
	if err != nil {
		return nil, fmt.Errorf("hkdf key: %w", err)
	}

	aead, err := contentEncryption.NewAead(contentEncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("new aead: %w", err)
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(randReader, nonce); err != nil {
		return nil, fmt.Errorf("read nonce: %w", err)
	}

	additionalData, err := encStructure(contentProtected, options.ExternalAad)
	if err != nil {
		return nil, fmt.Errorf("enc structure: %w", err)
	}

	ciphertext := aead.Seal([]byte{}, nonce, plaintext, additionalData)

	ephemeralKeyMap, err := ec2KeyFromPublicKey(ephemeralPrivateKey.PublicKey())
	if err != nil {
		return nil, fmt.Errorf("ec2 key from public key: %w", err)
	}

	recipientUnprotected := map[int64]any{HeaderLabelEphemeralKey: ephemeralKeyMap}
	if keyIdentifier := options.KeyIdentifier; len(keyIdentifier) > 0 {
		recipientUnprotected[HeaderLabelKeyIdentifier] = keyIdentifier
	}

	message := []any{
		contentProtected,
		map[int64]any{HeaderLabelIv: nonce},
		ciphertext,
		[]any{
			[]any{recipientProtected, recipientUnprotected, []byte{}},
		},
	}

	data, err := cbor.Encode(cbor.Tag{Number: encryptMessageTag, Content: message})
	if err != nil {
		return nil, fmt.Errorf("cbor encode (message): %w", err)
	}

	return data, nil
}
