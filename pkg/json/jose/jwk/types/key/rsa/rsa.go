package rsa

import (
	"crypto"
	rsa2 "crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	motmedelJwkErrors "github.com/altshiftab/utils_go/pkg/json/jose/jwk/errors"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type Key struct {
	N string `json:"n"`
	E string `json:"e"`
}

func (k *Key) PublicKey() (crypto.PublicKey, error) {
	n := k.N
	nBytes, err := base64.RawURLEncoding.DecodeString(n)
	if err != nil {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf(
				"base64 raw url encoding decode string (n): %w",
				err,
			),
			n,
		)
	}

	e := k.E
	eBytes, err := base64.RawURLEncoding.DecodeString(e)
	if err != nil {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf(
				"base64 raw url encoding decode string (e): %w",
				err,
			),
			e,
		)
	}

	var exponent int
	for i := range eBytes {
		exponent = exponent<<8 + int(eBytes[i])
	}

	return &rsa2.PublicKey{N: new(big.Int).SetBytes(nBytes), E: exponent}, nil
}

func New(m map[string]any) (*Key, error) {
	if m == nil {
		return nil, nil
	}

	kty, err := utils.MapGetConvert[string](m, "kty")
	if err != nil {
		return nil, fmt.Errorf("map get convert (kty): %w", err)
	}

	if kty != "RSA" {
		return nil, motmedelErrors.NewWithTrace(motmedelJwkErrors.ErrKtyMismatch)
	}

	n, err := utils.MapGetConvert[string](m, "n")
	if err != nil {
		return nil, fmt.Errorf("map get convert (n): %w", err)
	}

	e, err := utils.MapGetConvert[string](m, "e")
	if err != nil {
		return nil, fmt.Errorf("map get convert (e): %w", err)
	}

	return &Key{N: n, E: e}, nil
}

// intToBigEndianBytes encodes an int into a minimal-length big-endian byte slice.
func intToBigEndianBytes(e int) []byte {
	if e == 0 {
		return []byte{0}
	}
	tmp := e
	n := 0
	for tmp > 0 {
		n++
		tmp >>= 8
	}
	b := make([]byte, n)
	for i := n - 1; i >= 0; i-- {
		b[i] = byte(e & 0xff)
		e >>= 8
	}
	return b
}

// NewFromPublicKey constructs RSA JWK material from a Go *rsa.PublicKey.
// It encodes N as base64url(big-endian bytes) and E as base64url(minimal big-endian bytes).
func NewFromPublicKey(publicKey *rsa2.PublicKey) (*Key, error) {
	if publicKey == nil {
		return nil, nil
	}

	if publicKey.N == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("public key N"))
	}

	nB64 := base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes())
	eB64 := base64.RawURLEncoding.EncodeToString(intToBigEndianBytes(publicKey.E))

	return &Key{N: nB64, E: eB64}, nil
}

func (k *Key) Thumbprint() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("{\"e\":\"%s\",\"kty\":\"RSA\",\"n\":\"%s\"}", k.E, k.N)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
