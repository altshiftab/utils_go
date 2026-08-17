package key

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/json/v2"
	"fmt"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"

	motmedelCryptoEcdsa "github.com/altshiftab/utils_go/pkg/crypto/ecdsa"
	motmedelCryptoInterfaces "github.com/altshiftab/utils_go/pkg/crypto/interfaces"
	motmedelCryptoRsa "github.com/altshiftab/utils_go/pkg/crypto/rsa"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelJwkErrors "github.com/altshiftab/utils_go/pkg/json/jose/jwk/errors"
	ecKey "github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key/ec"
	rsaKey "github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key/rsa"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type Key struct {
	Alg string `json:"alg,omitempty"`
	Kty string `json:"kty,omitempty"`
	Kid string `json:"kid,omitempty"`
	Use string `json:"use,omitempty"`

	Material interface {
		PublicKey() (crypto.PublicKey, error)
	} `json:"-"`
}

// MarshalJSON ensures that the material fields (e.g., EC or RSA parameters)
// are serialized at the same top-level as the common JWK fields (alg, kty, kid, use).
func (k *Key) MarshalJSON() ([]byte, error) {
	// Start with base fields
	m := make(map[string]any)
	if k.Alg != "" {
		m["alg"] = k.Alg
	}
	if k.Kty != "" {
		m["kty"] = k.Kty
	}
	if k.Kid != "" {
		m["kid"] = k.Kid
	}
	if k.Use != "" {
		m["use"] = k.Use
	}

	// Merge material (if present) at the same level
	if k.Material != nil {
		b, err := json.Marshal(k.Material)
		if err != nil {
			return nil, motmedelErrors.New(fmt.Errorf("json marshal (material): %w", err), k.Material)
		}
		var mat map[string]any
		if err := json.Unmarshal(b, &mat); err != nil {
			return nil, motmedelErrors.New(fmt.Errorf("json unmarshal (material to map): %w", err), string(b))
		}
		for key, val := range mat {
			m[key] = val
		}
	}

	return json.Marshal(m)
}

func (k *Key) NamedVerifier() (motmedelCryptoInterfaces.NamedVerifier, error) {
	material := k.Material
	if material == nil {
		return nil, nil
	}

	publicKey, err := material.PublicKey()
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("public key: %w", err), material)
	}

	switch typedPublicKey := publicKey.(type) {
	case *ecdsa.PublicKey:
		method, err := motmedelCryptoEcdsa.FromPublicKey(typedPublicKey)
		if err != nil {
			return nil, motmedelErrors.New(fmt.Errorf("ecdsa from public key: %w", err), typedPublicKey)
		}
		return method, nil
	case *rsa.PublicKey:
		alg := k.Alg
		if alg == "" {
			// Default to using RS256
			alg = "RS256"
		}

		method, err := motmedelCryptoRsa.New(alg, nil, typedPublicKey)
		if err != nil {
			return nil, motmedelErrors.New(fmt.Errorf("rsa new: %w", err), alg)
		}
		return method, nil
	default:
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("%w: unsupported public key type: %T", motmedelErrors.ErrUnexpectedType, publicKey))
	}
}

func (k *Key) ThumbprintSHA256() (string, error) {
	switch k.Kty {
	case "EC":
		if mat, ok := k.Material.(*ecKey.Key); ok && mat != nil {
			return mat.Thumbprint(), nil
		}
		return "", motmedelErrors.NewWithTrace(fmt.Errorf("%w: invalid EC material type: %T", motmedelErrors.ErrUnexpectedType, k.Material))
	case "RSA":
		if mat, ok := k.Material.(*rsaKey.Key); ok && mat != nil {
			return mat.Thumbprint(), nil
		}
		return "", motmedelErrors.NewWithTrace(fmt.Errorf("%w: invalid RSA material type: %T", motmedelErrors.ErrUnexpectedType, k.Material))
	default:
		return "", motmedelErrors.NewWithTrace(motmedelJwkErrors.ErrUnsupportedKty, k.Kty)
	}
}

func New(m map[string]any) (*Key, error) {
	if m == nil {
		return nil, nil
	}

	kty, err := utils.MapGetConvert[string](m, "kty")
	if err != nil {
		var wrappedErr error = motmedelErrors.New(fmt.Errorf("map get convert: %w", err), m)
		if motmedelErrors.IsAny(err, motmedelErrors.ErrConversionNotOk, motmedelErrors.ErrNotInMap) {
			wrappedErr = fmt.Errorf("%w: %w", motmedelErrors.ErrValidationError, wrappedErr)
		}
		return nil, wrappedErr
	}

	var material interface {
		PublicKey() (crypto.PublicKey, error)
	}

	switch kty {
	case "RSA":
		material, err = rsaKey.New(m)
		if err != nil {
			var wrappedErr error = motmedelErrors.New(fmt.Errorf("rsa new: %w", err), m)
			if motmedelErrors.IsAny(err, motmedelErrors.ErrConversionNotOk, motmedelErrors.ErrNotInMap) {
				wrappedErr = fmt.Errorf("%w: %w", motmedelErrors.ErrValidationError, wrappedErr)
			}
			return nil, wrappedErr
		}
	case "EC":
		material, err = ecKey.New(m)
		if err != nil {
			var wrappedErr error = motmedelErrors.New(fmt.Errorf("ec new: %w", err), m)
			if motmedelErrors.IsAny(err, motmedelErrors.ErrConversionNotOk, motmedelErrors.ErrNotInMap) {
				wrappedErr = fmt.Errorf("%w: %w", motmedelErrors.ErrValidationError, wrappedErr)
			}
			return nil, wrappedErr
		}
	default:
		return nil, motmedelErrors.NewWithTrace(motmedelJwkErrors.ErrUnsupportedKty, kty)
	}

	alg, _ := m["alg"].(string)
	kid, _ := m["kid"].(string)
	use, _ := m["use"].(string)

	return &Key{Alg: alg, Kty: kty, Kid: kid, Use: use, Material: material}, nil
}

// NewFromPublicKey constructs a JWK key from a Go public key. It sets Kty and Material
// based on the public key type, and populates Alg/Kid/Use from the provided arguments.
// For RSA keys, Alg must be non-empty (e.g., "RS256") because verification requires it.
func NewFromPublicKey(publicKey crypto.PublicKey, alg, kid, use string) (*Key, error) {
	if publicKey == nil {
		return nil, nil
	}

	switch pk := publicKey.(type) {
	case *ecdsa.PublicKey:
		mat, err := ecKey.NewFromPublicKey(pk)
		if err != nil {
			return nil, motmedelErrors.New(fmt.Errorf("ec new from public key: %w", err))
		}
		return &Key{
			Alg:      alg,
			Kty:      "EC",
			Kid:      kid,
			Use:      use,
			Material: mat,
		}, nil
	case *rsa.PublicKey:
		if alg == "" {
			return nil, motmedelErrors.NewWithTrace(empty_error.New("alg"))
		}
		mat, err := rsaKey.NewFromPublicKey(pk)
		if err != nil {
			return nil, motmedelErrors.New(fmt.Errorf("rsa new from public key: %w", err))
		}
		return &Key{
			Alg:      alg,
			Kty:      "RSA",
			Kid:      kid,
			Use:      use,
			Material: mat,
		}, nil
	default:
		return nil, motmedelErrors.NewWithTrace(motmedelJwkErrors.ErrUnsupportedKty, fmt.Sprintf("%T", publicKey))
	}
}
