package eddsa

import (
	"crypto/ed25519"
	"fmt"

	motmedelCrypto "github.com/altshiftab/utils_go/pkg/crypto"
	motmedelCryptoErrors "github.com/altshiftab/utils_go/pkg/crypto/errors"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	motmedelUtils "github.com/altshiftab/utils_go/pkg/utils"
)

const Name = "EdDSA"

type Method struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
}

func (method *Method) Sign(message []byte) ([]byte, error) {
	privateKey := method.PrivateKey
	if len(privateKey) == 0 {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("private key"))
	}

	return ed25519.Sign(privateKey, message), nil
}

func (method *Method) Verify(message []byte, signature []byte) error {
	publicKey := method.PublicKey

	if privateKey := method.PrivateKey; len(publicKey) == 0 && len(privateKey) != 0 {
		var err error
		privateKeyPublic := privateKey.Public()
		publicKey, err = motmedelUtils.Convert[ed25519.PublicKey](privateKeyPublic)
		if err != nil {
			return motmedelErrors.NewWithTrace(
				fmt.Errorf("convert (private key public): %w", err),
				privateKeyPublic,
			)
		}
	}

	if len(publicKey) == 0 {
		return motmedelErrors.NewWithTrace(empty_error.New("public key"))
	}

	if ok := ed25519.Verify(publicKey, message, signature); ok {
		return nil
	} else {
		return motmedelErrors.NewWithTrace(motmedelCryptoErrors.ErrSignatureMismatch)
	}
}

func (method *Method) GetName() string {
	return Name
}

func FromPem(pemKey string) (*Method, error) {
	privateKey, err := motmedelCrypto.PrivateKeyFromPem[ed25519.PrivateKey](pemKey)
	if err != nil {
		return nil, fmt.Errorf("private key from pem: %w", err)
	}
	if len(privateKey) == 0 {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("private key"))
	}

	publicKey := privateKey.Public()
	eddsaPublicKey, err := motmedelUtils.Convert[ed25519.PublicKey](publicKey)
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("convert (public key): %w", err), publicKey)
	}

	return &Method{PrivateKey: privateKey, PublicKey: eddsaPublicKey}, nil
}
