package crypto

import (
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftUtils "github.com/altshiftab/utils_go/pkg/utils"
)

const (
	CoseAlgEs256 = -7
	CoseAlgEs384 = -35
	CoseAlgEs512 = -36

	CoseAlgEdDsa = -8

	CoseAlgRs256 = -257
	CoseAlgRs384 = -258
	CoseAlgRs512 = -259

	CoseAlgPs256 = -37
	CoseAlgPs384 = -38
	CoseAlgPs512 = -39

	AlgEs256 = "ES256"
	AlgEs384 = "ES384"
	AlgEs512 = "ES512"

	AlgEdDsa = "EdDSA"

	AlgRs256 = "RS256"
	AlgRs384 = "RS384"
	AlgRs512 = "RS512"

	AlgPs256 = "PS256"
	AlgPs384 = "PS384"
	AlgPs512 = "PS512"
)

var CoseAlgNames = map[int]string{
	CoseAlgEs256: AlgEs256,
	CoseAlgEs384: AlgEs384,
	CoseAlgEs512: AlgEs512,

	CoseAlgEdDsa: AlgEdDsa,

	CoseAlgRs256: AlgRs256,
	CoseAlgRs384: AlgRs384,
	CoseAlgRs512: AlgRs512,

	CoseAlgPs256: AlgPs256,
	CoseAlgPs384: AlgPs384,
	CoseAlgPs512: AlgPs512,
}

func MakeRawDerCertificateChain(certificates []*x509.Certificate) [][]byte {
	var certificateChain [][]byte

	for _, certificate := range certificates {
		if certificate == nil {
			continue
		}
		if raw := certificate.Raw; len(raw) != 0 {
			certificateChain = append(certificateChain, raw)
		}
	}

	return certificateChain
}

func MakeTlsCertificateFromX509Certificates(certificates []*x509.Certificate, key crypto.PrivateKey) *tls.Certificate {
	if len(certificates) == 0 {
		return nil
	}

	return &tls.Certificate{
		Certificate: MakeRawDerCertificateChain(certificates),
		PrivateKey:  key,
		Leaf:        certificates[0],
	}
}

func PrivateKeyFromPem[T any](pemKey string) (T, error) {
	var zero T
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return zero, altshiftErrors.NewWithTrace(nil_error.New("block"))
	}

	blockBytes := block.Bytes
	privateKey, err := x509.ParsePKCS8PrivateKey(blockBytes)
	if err != nil {
		return zero, altshiftErrors.NewWithTrace(fmt.Errorf("x509 parse pkcs8 private key: %w", err), blockBytes)
	}

	convertedPrivateKey, err := altshiftUtils.Convert[T](privateKey)
	if err != nil {
		return zero, altshiftErrors.New(fmt.Errorf("convert (private key): %w", err), privateKey)
	}

	return convertedPrivateKey, nil
}

func PublicKeyFromDer[T any](der []byte) (T, error) {
	var zero T

	publicKey, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return zero, altshiftErrors.NewWithTrace(fmt.Errorf("x509 parse pkix public key: %w", err), der)
	}

	convertedPublicKey, err := altshiftUtils.Convert[T](publicKey)
	if err != nil {
		return zero, altshiftErrors.New(fmt.Errorf("convert (public key): %w", err), publicKey)
	}

	return convertedPublicKey, nil
}

func PublicKeyFromPem[T any](pemKey string) (T, error) {
	var zero T
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return zero, altshiftErrors.NewWithTrace(nil_error.New("block"))
	}

	publicKey, err := PublicKeyFromDer[T](block.Bytes)
	if err != nil {
		return zero, fmt.Errorf("public key from der: %w", err)
	}

	return publicKey, nil
}
