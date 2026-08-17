package hmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"

	altshiftCryptoErrors "github.com/altshiftab/utils_go/pkg/crypto/errors"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

type Method struct {
	Secret   []byte
	HashFunc func() hash.Hash
	Name     string
}

func (method *Method) Sign(message []byte) ([]byte, error) {
	secret := method.Secret
	if len(secret) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("secret"))
	}

	mac := hmac.New(method.HashFunc, secret)
	_, err := mac.Write(message)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(err)
	}

	return mac.Sum(nil), nil
}

func (method *Method) Verify(message []byte, signature []byte) error {
	secret := method.Secret
	if len(secret) == 0 {
		return altshiftErrors.NewWithTrace(empty_error.New("secret"))
	}

	expectedMac := hmac.New(method.HashFunc, secret)
	_, err := expectedMac.Write(message)
	if err != nil {
		return altshiftErrors.NewWithTrace(err)
	}

	if hmac.Equal(expectedMac.Sum(nil), signature) {
		return nil
	} else {
		return altshiftErrors.NewWithTrace(altshiftCryptoErrors.ErrSignatureMismatch)
	}
}

func (method *Method) GetName() string {
	return method.Name
}

func New(algorithm string, secret []byte) (*Method, error) {
	switch algorithm {
	case "HS256":
		return &Method{Secret: secret, HashFunc: sha256.New, Name: "HS256"}, nil
	case "HS384":
		return &Method{Secret: secret, HashFunc: sha512.New384, Name: "HS384"}, nil
	case "HS512":
		return &Method{Secret: secret, HashFunc: sha512.New, Name: "HS512"}, nil
	default:
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %q", altshiftCryptoErrors.ErrUnsupportedAlgorithm, algorithm),
		)
	}
}
