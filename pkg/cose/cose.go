// Package cose implements COSE (RFC 9052, RFC 9053) encryption. The COSE_Encrypt structure with
// direct ECDH-ES key agreement (ECDH-ES + HKDF-256) and AES-GCM content encryption is supported;
// additional content-encryption algorithms can be added via RegisterContentEncryption.
package cose

import (
	"errors"
	"fmt"

	"github.com/altshiftab/utils_go/pkg/cbor"
)

const encryptMessageTag = 96

var (
	ErrUnsupportedAlgorithm = errors.New("unsupported algorithm")
	ErrMalformedMessage     = errors.New("malformed message")
	ErrMalformedKey         = errors.New("malformed key")
	ErrNoUsableRecipient    = errors.New("no usable recipient")
)

// recipientMessage is a COSE_recipient structure.
type recipientMessage struct {
	Protected   []byte
	Unprotected map[any]any
}

func encodeHeaderMap(headerMap map[int64]any) ([]byte, error) {
	if len(headerMap) == 0 {
		return []byte{}, nil
	}

	data, err := cbor.Encode(headerMap)
	if err != nil {
		return nil, fmt.Errorf("cbor encode (header map): %w", err)
	}

	return data, nil
}

func decodeHeaderMap(data []byte) (map[any]any, error) {
	if len(data) == 0 {
		return nil, nil
	}

	value, err := cbor.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("%w: cbor decode (header map): %w", ErrMalformedMessage, err)
	}

	headerMap, ok := value.(map[any]any)
	if !ok {
		return nil, fmt.Errorf("%w: header map is not a map", ErrMalformedMessage)
	}

	return headerMap, nil
}

// encStructure builds the Enc_structure (RFC 9052, Section 5.3) serving as additional
// authenticated data for the content encryption.
func encStructure(bodyProtected []byte, externalAad []byte) ([]byte, error) {
	if bodyProtected == nil {
		bodyProtected = []byte{}
	}
	if externalAad == nil {
		externalAad = []byte{}
	}

	data, err := cbor.Encode([]any{"Encrypt", bodyProtected, externalAad})
	if err != nil {
		return nil, fmt.Errorf("cbor encode (enc structure): %w", err)
	}

	return data, nil
}

// kdfContext builds the COSE_KDF_Context (RFC 9053, Section 5.2) used as the HKDF info input.
func kdfContext(contentAlgorithm Algorithm, keyBits int, recipientProtected []byte) ([]byte, error) {
	if recipientProtected == nil {
		recipientProtected = []byte{}
	}

	data, err := cbor.Encode(
		[]any{
			int64(contentAlgorithm),
			[]any{nil, nil, nil},
			[]any{nil, nil, nil},
			[]any{uint64(keyBits), recipientProtected}, //nolint:gosec // keyBits is a positive key-size constant
		},
	)
	if err != nil {
		return nil, fmt.Errorf("cbor encode (kdf context): %w", err)
	}

	return data, nil
}
