package cloud_kms

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
	"slices"

	altshiftCrypto "github.com/altshiftab/utils_go/pkg/crypto"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

// signerAlgorithms are the key algorithms a Signer takes: those that sign a SHA-256 digest, ECDSA or RSA PKCS #1 v1.5, which is what AsymmetricSign sends and what a CMS signature names.
var signerAlgorithms = []string{
	"EC_SIGN_P256_SHA256",
	"RSA_SIGN_PKCS1_2048_SHA256",
	"RSA_SIGN_PKCS1_3072_SHA256",
	"RSA_SIGN_PKCS1_4096_SHA256",
}

var (
	ErrUnsupportedAlgorithm = errors.New("unsupported key algorithm")
	ErrSignatureInvalid     = errors.New("the signature does not verify under the key's public half")
)

// Signer is a crypto.Signer whose private key is a Cloud KMS asymmetric signing key version, which never leaves KMS.
type Signer struct {
	client     *Client
	keyVersion string
	publicKey  crypto.PublicKey
	// ctx is what Sign calls KMS under, since crypto.Signer's Sign takes no context.
	ctx context.Context //nolint:containedctx // crypto.Signer has no context of its own.
}

// NewSigner reads the key version's public half and algorithm, and refuses an algorithm a Signer cannot sign with.
func NewSigner(ctx context.Context, client *Client, keyVersion string) (*Signer, error) {
	if client == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("cloud kms client"))
	}
	if keyVersion == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("key version"))
	}

	publicKey, err := client.GetPublicKey(ctx, keyVersion)
	if err != nil {
		return nil, fmt.Errorf("get public key: %w", err)
	}
	if !slices.Contains(signerAlgorithms, publicKey.Algorithm) {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %q", ErrUnsupportedAlgorithm, publicKey.Algorithm), keyVersion)
	}

	parsed, err := altshiftCrypto.PublicKeyFromPem[crypto.PublicKey](publicKey.Pem)
	if err != nil {
		return nil, fmt.Errorf("public key from pem: %w", err)
	}
	switch parsed.(type) {
	case *ecdsa.PublicKey, *rsa.PublicKey:
	default:
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: a %T public key", ErrUnsupportedAlgorithm, parsed), keyVersion)
	}

	return &Signer{client: client, keyVersion: keyVersion, publicKey: parsed, ctx: ctx}, nil
}

// Public returns the key version's public half.
func (s *Signer) Public() crypto.PublicKey {
	return s.publicKey
}

// Sign signs a SHA-256 digest in KMS, and checks the signature against the public half before returning it, so a corrupted answer is never passed on as a signature.
func (s *Signer) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if s == nil || s.client == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("cloud kms signer"))
	}
	if opts == nil || opts.HashFunc() != crypto.SHA256 {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: only a SHA-256 digest is signed", ErrUnsupportedAlgorithm))
	}
	if _, pss := opts.(*rsa.PSSOptions); pss {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: the key signs PKCS #1 v1.5, not PSS", ErrUnsupportedAlgorithm))
	}

	signature, err := s.client.AsymmetricSign(s.ctx, s.keyVersion, digest)
	if err != nil {
		return nil, fmt.Errorf("asymmetric sign: %w", err)
	}

	switch key := s.publicKey.(type) {
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(key, digest, signature) {
			return nil, altshiftErrors.NewWithTrace(ErrSignatureInvalid, s.keyVersion)
		}
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest, signature); err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %w", ErrSignatureInvalid, err), s.keyVersion)
		}
	}
	return signature, nil
}
