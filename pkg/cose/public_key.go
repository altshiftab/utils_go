package cose

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"fmt"
	"math"
	"math/big"

	"github.com/altshiftab/utils_go/pkg/cbor"
)

// Key type identifiers from the IANA COSE Key Types registry.
const (
	keyTypeOkp int64 = 1
	keyTypeRsa int64 = 3
)

const (
	keyParameterAlg int64 = 3

	// RSA key parameters (RFC 8230).
	keyParameterRsaN int64 = -1
	keyParameterRsaE int64 = -2
)

// CurveEd25519 is the Ed25519 identifier from the IANA COSE Elliptic Curves registry.
const CurveEd25519 int64 = 6

var ellipticCurveRegistry = map[int64]elliptic.Curve{
	CurveP256: elliptic.P256(),
	CurveP384: elliptic.P384(),
	CurveP521: elliptic.P521(),
}

// KeyAlgorithm returns the alg parameter of a COSE_Key map.
func KeyAlgorithm(keyMap map[any]any) (Algorithm, bool) {
	value, ok := headerValue(keyMap, keyParameterAlg)
	if !ok {
		return 0, false
	}

	algorithm, ok := toInt64(value)
	if !ok {
		return 0, false
	}

	return Algorithm(algorithm), true
}

func keyByteParameter(keyMap map[any]any, label int64, name string) ([]byte, error) {
	value, ok := headerValue(keyMap, label)
	if !ok {
		return nil, fmt.Errorf("%w: missing %s", ErrMalformedKey, name)
	}

	data, ok := value.([]byte)
	if !ok || len(data) == 0 {
		return nil, fmt.Errorf("%w: malformed %s", ErrMalformedKey, name)
	}

	return data, nil
}

func ecdsaPublicKeyFromEc2Key(keyMap map[any]any) (*ecdsa.PublicKey, error) {
	crvValue, ok := headerValue(keyMap, keyParameterCrv)
	if !ok {
		return nil, fmt.Errorf("%w: missing crv", ErrMalformedKey)
	}
	crv, ok := toInt64(crvValue)
	if !ok {
		return nil, fmt.Errorf("%w: malformed crv", ErrMalformedKey)
	}

	curve, ok := ellipticCurveRegistry[crv]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported curve %d", ErrUnsupportedAlgorithm, crv)
	}
	coordinateSize := (curve.Params().BitSize + 7) / 8

	x, err := keyByteParameter(keyMap, keyParameterX, "x coordinate")
	if err != nil {
		return nil, err
	}

	y, err := keyByteParameter(keyMap, keyParameterY, "y coordinate")
	if err != nil {
		return nil, err
	}

	if len(x) > coordinateSize || len(y) > coordinateSize {
		return nil, fmt.Errorf("%w: oversized coordinate", ErrMalformedKey)
	}

	publicKey := &ecdsa.PublicKey{
		Curve: curve,
		X:     new(big.Int).SetBytes(x),
		Y:     new(big.Int).SetBytes(y),
	}

	// ECDH conversion validates that the point is on the curve.
	if _, err := publicKey.ECDH(); err != nil {
		return nil, fmt.Errorf("%w: invalid point: %w", ErrMalformedKey, err)
	}

	return publicKey, nil
}

func ed25519PublicKeyFromOkpKey(keyMap map[any]any) (ed25519.PublicKey, error) {
	crvValue, ok := headerValue(keyMap, keyParameterCrv)
	if !ok {
		return nil, fmt.Errorf("%w: missing crv", ErrMalformedKey)
	}
	if crv, ok := toInt64(crvValue); !ok || crv != CurveEd25519 {
		return nil, fmt.Errorf("%w: unsupported curve %v", ErrUnsupportedAlgorithm, crvValue)
	}

	x, err := keyByteParameter(keyMap, keyParameterX, "x coordinate")
	if err != nil {
		return nil, err
	}

	if len(x) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: unexpected public key length %d", ErrMalformedKey, len(x))
	}

	return ed25519.PublicKey(x), nil
}

func rsaPublicKeyFromRsaKey(keyMap map[any]any) (*rsa.PublicKey, error) {
	n, err := keyByteParameter(keyMap, keyParameterRsaN, "modulus")
	if err != nil {
		return nil, err
	}

	e, err := keyByteParameter(keyMap, keyParameterRsaE, "exponent")
	if err != nil {
		return nil, err
	}

	exponent := new(big.Int).SetBytes(e)
	if !exponent.IsInt64() || exponent.Int64() < 3 || exponent.Int64() > math.MaxInt32 {
		return nil, fmt.Errorf("%w: unsupported exponent", ErrMalformedKey)
	}

	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exponent.Int64())}, nil
}

// PublicKey converts a decoded COSE_Key map into the corresponding standard library public key:
// *ecdsa.PublicKey for EC2 keys, ed25519.PublicKey for OKP Ed25519 keys, and *rsa.PublicKey for
// RSA keys.
func PublicKey(keyMap map[any]any) (crypto.PublicKey, error) {
	ktyValue, ok := headerValue(keyMap, keyParameterKty)
	if !ok {
		return nil, fmt.Errorf("%w: missing kty", ErrMalformedKey)
	}
	kty, ok := toInt64(ktyValue)
	if !ok {
		return nil, fmt.Errorf("%w: malformed kty", ErrMalformedKey)
	}

	switch kty {
	case keyTypeEc2:
		return ecdsaPublicKeyFromEc2Key(keyMap)
	case keyTypeOkp:
		return ed25519PublicKeyFromOkpKey(keyMap)
	case keyTypeRsa:
		return rsaPublicKeyFromRsaKey(keyMap)
	default:
		return nil, fmt.Errorf("%w: unsupported key type %d", ErrUnsupportedAlgorithm, kty)
	}
}

// ParsePublicKey decodes CBOR-encoded COSE_Key bytes and converts them with PublicKey.
func ParsePublicKey(data []byte) (crypto.PublicKey, error) {
	value, err := cbor.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("%w: cbor decode: %w", ErrMalformedKey, err)
	}

	keyMap, ok := value.(map[any]any)
	if !ok {
		return nil, fmt.Errorf("%w: not a map", ErrMalformedKey)
	}

	return PublicKey(keyMap)
}
