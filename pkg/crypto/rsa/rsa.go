package rsa

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"

	motmedelCrypto "github.com/altshiftab/utils_go/pkg/crypto"
	motmedelCryptoErrors "github.com/altshiftab/utils_go/pkg/crypto/errors"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

// NOTE: Not tested (AI-generated...)

type Method struct {
	PrivateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
	HashFunc   func() hash.Hash
	Hash       crypto.Hash
	Name       string

	pss bool // true => RSASSA-PSS, false => RSASSA-PKCS1-v1_5
}

func (m *Method) hash(message []byte) ([]byte, error) {
	h := m.HashFunc()
	if _, err := h.Write(message); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func (m *Method) Sign(message []byte) ([]byte, error) {
	if m.PrivateKey == nil {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("secret"))
	}

	digest, err := m.hash(message)
	if err != nil {
		return nil, motmedelErrors.NewWithTrace(err)
	}

	if m.pss {
		sig, err := rsa.SignPSS(rand.Reader, m.PrivateKey, m.Hash, digest, &rsa.PSSOptions{
			SaltLength: m.Hash.Size(), // per RFC 7518: salt length == hash length
			Hash:       m.Hash,
		})
		if err != nil {
			return nil, motmedelErrors.NewWithTrace(err)
		}
		return sig, nil
	}

	sig, err := rsa.SignPKCS1v15(rand.Reader, m.PrivateKey, m.Hash, digest)
	if err != nil {
		return nil, motmedelErrors.NewWithTrace(err)
	}
	return sig, nil
}

func (m *Method) Verify(message []byte, signature []byte) error {
	pub := m.PublicKey
	if pub == nil && m.PrivateKey != nil {
		pub = &m.PrivateKey.PublicKey
	}
	if pub == nil {
		return motmedelErrors.NewWithTrace(empty_error.New("public key"))
	}

	digest, err := m.hash(message)
	if err != nil {
		return motmedelErrors.NewWithTrace(err)
	}

	if m.pss {
		if err := rsa.VerifyPSS(pub, m.Hash, digest, signature, &rsa.PSSOptions{
			SaltLength: m.Hash.Size(),
			Hash:       m.Hash,
		}); err != nil {
			return motmedelErrors.NewWithTrace(motmedelCryptoErrors.ErrSignatureMismatch)
		}
		return nil
	}

	if err := rsa.VerifyPKCS1v15(pub, m.Hash, digest, signature); err != nil {
		return motmedelErrors.NewWithTrace(motmedelCryptoErrors.ErrSignatureMismatch)
	}
	return nil
}

func (m *Method) GetName() string {
	return m.Name
}

func New(algorithm string, privateKey *rsa.PrivateKey, publicKey *rsa.PublicKey) (*Method, error) {
	var (
		hashFunc func() hash.Hash
		hash     crypto.Hash
		pss      bool
		name     string
	)

	switch algorithm {
	case motmedelCrypto.AlgRs256:
		hashFunc = sha256.New
		hash = crypto.SHA256
		pss = false
		name = motmedelCrypto.AlgRs256
	case motmedelCrypto.AlgRs384:
		hashFunc = sha512.New384
		hash = crypto.SHA384
		pss = false
		name = motmedelCrypto.AlgRs384
	case motmedelCrypto.AlgRs512:
		hashFunc = sha512.New
		hash = crypto.SHA512
		pss = false
		name = motmedelCrypto.AlgRs512
	case motmedelCrypto.AlgPs256:
		hashFunc = sha256.New
		hash = crypto.SHA256
		pss = true
		name = motmedelCrypto.AlgPs256
	case motmedelCrypto.AlgPs384:
		hashFunc = sha512.New384
		hash = crypto.SHA384
		pss = true
		name = motmedelCrypto.AlgPs384
	case motmedelCrypto.AlgPs512:
		hashFunc = sha512.New
		hash = crypto.SHA512
		pss = true
		name = motmedelCrypto.AlgPs512
	default:
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: %q", motmedelCryptoErrors.ErrUnsupportedAlgorithm, algorithm),
		)
	}

	return &Method{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
		HashFunc:   hashFunc,
		Hash:       hash,
		Name:       name,
		pss:        pss,
	}, nil
}

// NewFromPublicKey constructs a verifying Method from an RSA public key by
// heuristically selecting a JOSE RSA algorithm based on modulus size.
//
// Heuristics (PKCS#1 v1.5):
//   - n bits >= 4096  -> RS512
//   - n bits >= 3072  -> RS384
//   - otherwise       -> RS256
//
// Note: The choice between PKCS#1 v1.5 (RS*) and PSS (PS*) cannot be inferred
// from the key alone. This helper defaults to RS* for maximum interoperability.
func NewFromPublicKey(publicKey *rsa.PublicKey) (*Method, error) {
	if publicKey == nil {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("public key"))
	}

	bits := publicKey.N.BitLen()
	var alg string
	switch {
	case bits >= 4096:
		alg = motmedelCrypto.AlgRs512
	case bits >= 3072:
		alg = motmedelCrypto.AlgRs384
	default:
		alg = motmedelCrypto.AlgRs256
	}

	return New(alg, nil, publicKey)
}
