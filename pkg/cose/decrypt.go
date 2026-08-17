package cose

import (
	"bytes"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/altshiftab/utils_go/pkg/cbor"
)

type DecryptOptions struct {
	// ExternalAad is additional authenticated data not carried in the message.
	ExternalAad []byte
}

type DecryptResult struct {
	Plaintext []byte
	// ContentType is the content protected header's content type: a string, an int64, or nil if
	// absent.
	ContentType any
	// KeyIdentifier is the key identifier of the recipient that was used for decryption, or nil
	// if absent.
	KeyIdentifier []byte
	// Protected is the decoded content protected header map.
	Protected map[any]any
}

func parseRecipientMessage(value any) (*recipientMessage, error) {
	recipientArray, ok := value.([]any)
	if ok && len(recipientArray) < 3 {
		ok = false
	}
	if !ok {
		return nil, fmt.Errorf("%w: malformed recipient", ErrMalformedMessage)
	}

	protected, ok := recipientArray[0].([]byte)
	if !ok {
		return nil, fmt.Errorf("%w: malformed recipient protected header", ErrMalformedMessage)
	}

	unprotected, ok := recipientArray[1].(map[any]any)
	if !ok {
		return nil, fmt.Errorf("%w: malformed recipient unprotected header", ErrMalformedMessage)
	}

	return &recipientMessage{Protected: protected, Unprotected: unprotected}, nil
}

func recipientContentEncryptionKey(
	recipient *recipientMessage,
	privateKey *ecdh.PrivateKey,
	contentAlgorithm Algorithm,
	keyBits int,
) ([]byte, error) {
	recipientProtectedMap, err := decodeHeaderMap(recipient.Protected)
	if err != nil {
		return nil, fmt.Errorf("decode header map (recipient protected): %w", err)
	}

	algorithmValue, ok := headerValue(recipientProtectedMap, HeaderLabelAlgorithm)
	if !ok {
		return nil, fmt.Errorf("%w: missing recipient algorithm", ErrMalformedMessage)
	}
	algorithm, ok := toInt64(algorithmValue)
	if !ok {
		return nil, fmt.Errorf("%w: malformed recipient algorithm", ErrMalformedMessage)
	}
	if Algorithm(algorithm) != AlgorithmEcdhEsHkdf256 {
		return nil, fmt.Errorf("%w: recipient algorithm %d", ErrUnsupportedAlgorithm, algorithm)
	}

	ephemeralKeyValue, ok := headerValue(recipient.Unprotected, HeaderLabelEphemeralKey)
	if !ok {
		if ephemeralKeyValue, ok = headerValue(recipientProtectedMap, HeaderLabelEphemeralKey); !ok {
			return nil, fmt.Errorf("%w: missing ephemeral key", ErrMalformedMessage)
		}
	}

	ephemeralKeyMap, ok := ephemeralKeyValue.(map[any]any)
	if !ok {
		return nil, fmt.Errorf("%w: malformed ephemeral key", ErrMalformedMessage)
	}

	ephemeralPublicKey, err := publicKeyFromEc2Key(ephemeralKeyMap)
	if err != nil {
		return nil, fmt.Errorf("public key from ec2 key: %w", err)
	}

	sharedSecret, err := privateKey.ECDH(ephemeralPublicKey)
	if err != nil {
		return nil, fmt.Errorf("ecdh: %w", err)
	}

	context, err := kdfContext(contentAlgorithm, keyBits, recipient.Protected)
	if err != nil {
		return nil, fmt.Errorf("kdf context: %w", err)
	}

	contentEncryptionKey, err := hkdf.Key(sha256.New, sharedSecret, nil, string(context), keyBits/8)
	if err != nil {
		return nil, fmt.Errorf("hkdf key: %w", err)
	}

	return contentEncryptionKey, nil
}

// Decrypt decrypts a COSE_Encrypt message (CBOR tag 96, tagged or untagged) using direct ECDH-ES
// key agreement with HKDF-256.
func Decrypt(message []byte, privateKey *ecdh.PrivateKey, options *DecryptOptions) (*DecryptResult, error) {
	if privateKey == nil {
		return nil, fmt.Errorf("%w: nil private key", ErrMalformedKey)
	}

	if options == nil {
		options = &DecryptOptions{}
	}

	// The decoded envelope values (ciphertext, iv, ephemeral key coordinates) are consumed within
	// this function, so they can alias the message buffer instead of being copied.
	decodedMessage, err := cbor.DecodeNoCopy(message)
	if err != nil {
		return nil, fmt.Errorf("%w: cbor decode (message): %w", ErrMalformedMessage, err)
	}

	if tag, ok := decodedMessage.(cbor.Tag); ok {
		if tag.Number != encryptMessageTag {
			return nil, fmt.Errorf("%w: unexpected tag %d", ErrMalformedMessage, tag.Number)
		}
		decodedMessage = tag.Content
	}

	messageArray, ok := decodedMessage.([]any)
	if ok && len(messageArray) != 4 {
		ok = false
	}
	if !ok {
		return nil, fmt.Errorf("%w: message is not a four-element array", ErrMalformedMessage)
	}

	contentProtected, ok := messageArray[0].([]byte)
	if !ok {
		return nil, fmt.Errorf("%w: malformed content protected header", ErrMalformedMessage)
	}

	contentUnprotected, ok := messageArray[1].(map[any]any)
	if !ok {
		return nil, fmt.Errorf("%w: malformed content unprotected header", ErrMalformedMessage)
	}

	ciphertext, ok := messageArray[2].([]byte)
	if !ok {
		return nil, fmt.Errorf("%w: malformed ciphertext", ErrMalformedMessage)
	}

	recipientValues, ok := messageArray[3].([]any)
	if !ok {
		return nil, fmt.Errorf("%w: malformed recipients", ErrMalformedMessage)
	}

	contentProtectedMap, err := decodeHeaderMap(contentProtected)
	if err != nil {
		return nil, fmt.Errorf("decode header map (content protected): %w", err)
	}

	algorithmValue, ok := headerValue(contentProtectedMap, HeaderLabelAlgorithm)
	if !ok {
		return nil, fmt.Errorf("%w: missing content algorithm", ErrMalformedMessage)
	}
	algorithm, ok := toInt64(algorithmValue)
	if !ok {
		return nil, fmt.Errorf("%w: malformed content algorithm", ErrMalformedMessage)
	}

	contentAlgorithm := Algorithm(algorithm)
	contentEncryption, ok := contentEncryptionRegistry[contentAlgorithm]
	if !ok {
		return nil, fmt.Errorf("%w: content encryption %d", ErrUnsupportedAlgorithm, algorithm)
	}

	nonce, ok := headerBytes(contentUnprotected, HeaderLabelIv)
	if !ok {
		if nonce, ok = headerBytes(contentProtectedMap, HeaderLabelIv); !ok {
			return nil, fmt.Errorf("%w: missing iv", ErrMalformedMessage)
		}
	}

	additionalData, err := encStructure(contentProtected, options.ExternalAad)
	if err != nil {
		return nil, fmt.Errorf("enc structure: %w", err)
	}

	var contentType any
	if contentTypeValue, ok := headerValue(contentProtectedMap, HeaderLabelContentType); ok {
		if intContentType, ok := toInt64(contentTypeValue); ok {
			contentType = intContentType
		} else {
			contentType = contentTypeValue
		}
	}

	var recipientErrors []error

	for _, recipientValue := range recipientValues {
		recipient, err := parseRecipientMessage(recipientValue)
		if err != nil {
			recipientErrors = append(recipientErrors, err)
			continue
		}

		contentEncryptionKey, err := recipientContentEncryptionKey(
			recipient,
			privateKey,
			contentAlgorithm,
			contentEncryption.KeyBits,
		)
		if err != nil {
			recipientErrors = append(recipientErrors, err)
			continue
		}

		aead, err := contentEncryption.NewAead(contentEncryptionKey)
		if err != nil {
			recipientErrors = append(recipientErrors, fmt.Errorf("new aead: %w", err))
			continue
		}

		plaintext, err := aead.Open([]byte{}, nonce, ciphertext, additionalData)
		if err != nil {
			recipientErrors = append(recipientErrors, fmt.Errorf("aead open: %w", err))
			continue
		}

		// Cloned because the envelope decode aliases the message buffer, which the result must
		// not retain.
		keyIdentifier, _ := headerBytes(recipient.Unprotected, HeaderLabelKeyIdentifier)

		return &DecryptResult{
			Plaintext:     plaintext,
			ContentType:   contentType,
			KeyIdentifier: bytes.Clone(keyIdentifier),
			Protected:     contentProtectedMap,
		}, nil
	}

	return nil, fmt.Errorf("%w: %w", ErrNoUsableRecipient, errors.Join(recipientErrors...))
}
