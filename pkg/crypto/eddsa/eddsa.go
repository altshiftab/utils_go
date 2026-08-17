package eddsa

import (
	"crypto/ed25519"
	"fmt"

	altshiftCrypto "github.com/altshiftab/utils_go/pkg/crypto"
	altshiftCryptoErrors "github.com/altshiftab/utils_go/pkg/crypto/errors"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	altshiftUtils "github.com/altshiftab/utils_go/pkg/utils"
)

const Name = "EdDSA"

type Method struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
}

func (method *Method) Sign(message []byte) ([]byte, error) {
	privateKey := method.PrivateKey
	if len(privateKey) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("private key"))
	}

	return ed25519.Sign(privateKey, message), nil
}

func (method *Method) Verify(message []byte, signature []byte) error {
	publicKey := method.PublicKey

	if privateKey := method.PrivateKey; len(publicKey) == 0 && len(privateKey) != 0 {
		var err error
		privateKeyPublic := privateKey.Public()
		publicKey, err = altshiftUtils.Convert[ed25519.PublicKey](privateKeyPublic)
		if err != nil {
			return altshiftErrors.NewWithTrace(
				fmt.Errorf("convert (private key public): %w", err),
				privateKeyPublic,
			)
		}
	}

	if len(publicKey) == 0 {
		return altshiftErrors.NewWithTrace(empty_error.New("public key"))
	}

	if ok := ed25519.Verify(publicKey, message, signature); ok {
		return nil
	} else {
		return altshiftErrors.NewWithTrace(altshiftCryptoErrors.ErrSignatureMismatch)
	}
}

func (method *Method) GetName() string {
	return Name
}

func FromPem(pemKey string) (*Method, error) {
	privateKey, err := altshiftCrypto.PrivateKeyFromPem[ed25519.PrivateKey](pemKey)
	if err != nil {
		return nil, fmt.Errorf("private key from pem: %w", err)
	}
	if len(privateKey) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("private key"))
	}

	publicKey := privateKey.Public()
	eddsaPublicKey, err := altshiftUtils.Convert[ed25519.PublicKey](publicKey)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("convert (public key): %w", err), publicKey)
	}

	return &Method{PrivateKey: privateKey, PublicKey: eddsaPublicKey}, nil
}
