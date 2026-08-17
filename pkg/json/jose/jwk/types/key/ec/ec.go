package ec

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelJwkErrors "github.com/altshiftab/utils_go/pkg/json/jose/jwk/errors"
	"github.com/altshiftab/utils_go/pkg/utils"
)

func curveFromCrv(crv string) elliptic.Curve {
	switch crv {
	case "P-256":
		return elliptic.P256()
	case "P-384":
		return elliptic.P384()
	case "P-521":
		return elliptic.P521()
	}

	return nil
}

type Key struct {
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (k *Key) PublicKey() (crypto.PublicKey, error) {
	x := k.X
	xBytes, err := base64.RawURLEncoding.DecodeString(x)
	if err != nil {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf(
				"base64 raw url encoding decode string (x): %w",
				err,
			),
			x,
		)
	}

	y := k.Y
	yBytes, err := base64.RawURLEncoding.DecodeString(y)
	if err != nil {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf(
				"base64 raw url encoding decode string (y): %w",
				err,
			),
			y,
		)
	}

	crv := k.Crv
	curve := curveFromCrv(crv)
	if utils.IsNil(curve) {
		return nil, motmedelErrors.NewWithTrace(motmedelJwkErrors.ErrUnsupportedCrv, crv)
	}

	return &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(xBytes), Y: new(big.Int).SetBytes(yBytes)}, nil
}

func New(m map[string]any) (*Key, error) {
	if m == nil {
		return nil, nil
	}

	kty, err := utils.MapGetConvert[string](m, "kty")
	if err != nil {
		return nil, fmt.Errorf("map get convert (kty): %w", err)
	}

	if kty != "EC" {
		return nil, motmedelErrors.NewWithTrace(motmedelJwkErrors.ErrKtyMismatch)
	}

	crv, err := utils.MapGetConvert[string](m, "crv")
	if err != nil {
		return nil, fmt.Errorf("map get convert (crv): %w", err)
	}

	x, err := utils.MapGetConvert[string](m, "x")
	if err != nil {
		return nil, fmt.Errorf("map get convert (x): %w", err)
	}

	y, err := utils.MapGetConvert[string](m, "y")
	if err != nil {
		return nil, fmt.Errorf("map get convert (y): %w", err)
	}

	return &Key{Crv: crv, X: x, Y: y}, nil
}

func NewFromPublicKey(publicKey *ecdsa.PublicKey) (*Key, error) {
	if publicKey == nil {
		return nil, nil
	}

	var crv string
	var size int
	switch publicKey.Curve {
	case elliptic.P256():
		crv = "P-256"
		size = 32
	case elliptic.P384():
		crv = "P-384"
		size = 48
	case elliptic.P521():
		crv = "P-521"
		size = 66
	default:
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("%w: %T", motmedelJwkErrors.ErrUnsupportedCrv, publicKey.Curve))
	}

	// Bytes returns the uncompressed SEC1 point (0x04 || X || Y), each coordinate
	// left-padded to size bytes. This replaces reading the deprecated
	// PublicKey.X / PublicKey.Y big.Int fields.
	point, err := publicKey.Bytes()
	if err != nil {
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("ecdsa public key bytes: %w", err))
	}
	if len(point) != 1+2*size {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: unexpected ecdsa public key point length %d", motmedelErrors.ErrUnexpectedType, len(point)),
		)
	}

	return &Key{
		Crv: crv,
		X:   base64.RawURLEncoding.EncodeToString(point[1 : 1+size]),
		Y:   base64.RawURLEncoding.EncodeToString(point[1+size:]),
	}, nil
}

func (k *Key) Thumbprint() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("{\"crv\":\"%s\",\"kty\":\"EC\",\"x\":\"%s\",\"y\":\"%s\"}", k.Crv, k.X, k.Y)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
