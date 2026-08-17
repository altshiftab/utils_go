package ecdsa

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"hash"
	"math/big"

	altshiftCryptoErrors "github.com/altshiftab/utils_go/pkg/crypto/errors"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/utils"
)

func copyWithLeftPad(dst []byte, x *big.Int, size int) {
	b := x.Bytes()
	pad := size - len(b)
	for i := range pad {
		dst[i] = 0
	}
	copy(dst[pad:], b)
}

func canonicalizeS(s, n *big.Int) *big.Int {
	halfOrder := new(big.Int).Rsh(n, 1)
	if s.Cmp(halfOrder) == 1 {
		return new(big.Int).Sub(n, s)
	}
	return s
}

type Method struct {
	PrivateKey *ecdsa.PrivateKey
	PublicKey  *ecdsa.PublicKey
	HashFunc   func() hash.Hash
	Name       string

	curve elliptic.Curve
	size  int // byte length for R or S
}

func (m *Method) hash(message []byte) ([]byte, error) {
	h := m.HashFunc()
	if _, err := h.Write(message); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func (m *Method) Sign(message []byte) ([]byte, error) {
	if m == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.NewWithInstance("method", "signer"))
	}

	if m.PrivateKey == nil {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("secret"))
	}

	digest, err := m.hash(message)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(err)
	}

	r, s, err := ecdsa.Sign(rand.Reader, m.PrivateKey, digest)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(err)
	}

	// Canonicalize S to low-S form for interoperability
	s = canonicalizeS(s, m.curve.Params().N)

	// Serialize as fixed-length R||S
	sig := make([]byte, m.size*2)
	copyWithLeftPad(sig[:m.size], r, m.size)
	copyWithLeftPad(sig[m.size:], s, m.size)

	return sig, nil
}

func (m *Method) Verify(message []byte, signature []byte) error {
	if m == nil {
		return altshiftErrors.NewWithTrace(nil_error.NewWithInstance("method", "verifier"))
	}

	pub := m.PublicKey
	if pub == nil && m.PrivateKey != nil {
		pub = &m.PrivateKey.PublicKey
	}
	if pub == nil {
		return altshiftErrors.NewWithTrace(empty_error.New("public key"))
	}

	// Expect R||S with fixed lengths
	if len(signature) != 2*m.size {
		// Use signature mismatch to avoid leaking details about expected sizes
		return altshiftErrors.NewWithTrace(altshiftCryptoErrors.ErrSignatureMismatch)
	}

	r := new(big.Int).SetBytes(signature[:m.size])
	s := new(big.Int).SetBytes(signature[m.size:])

	digest, err := m.hash(message)
	if err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("hash: %w", err))
	}

	if !ecdsa.Verify(pub, digest, r, s) {
		return altshiftErrors.NewWithTrace(altshiftCryptoErrors.ErrSignatureMismatch)
	}

	return nil
}

func (m *Method) GetName() string {
	return m.Name
}

func FromPublicKey(publicKey *ecdsa.PublicKey) (*Method, error) {
	if publicKey == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("public key"))
	}

	method, err := New(nil, publicKey)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("method"))
	}

	return method, nil
}

func FromPrivateKey(privateKey *ecdsa.PrivateKey) (*Method, error) {
	if privateKey == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("private key"))
	}

	method, err := New(privateKey, nil)
	if err != nil {
		return nil, err
	}
	if method == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("method"))
	}

	return method, nil
}

// deriveAlgFromCurve picks the JOSE alg name and hash function based on the curve.
func deriveAlgFromCurveParams(curveParams *elliptic.CurveParams) (string, func() hash.Hash, error) {
	if curveParams == nil {
		return "", nil, nil
	}

	switch curveName := curveParams.Name; curveName {
	case "P-256":
		return "ES256", sha256.New, nil
	case "P-384":
		return "ES384", sha512.New384, nil
	case "P-521":
		return "ES512", sha512.New, nil
	default:
		return "", nil, altshiftErrors.NewWithTrace(
			altshiftCryptoErrors.ErrUnsupportedCurve,
			curveName,
		)
	}
}

func getCurveParams(curve elliptic.Curve) *elliptic.CurveParams {
	if utils.IsNil(curve) {
		return nil
	}

	return curve.Params()
}

func New(privateKey *ecdsa.PrivateKey, publicKey *ecdsa.PublicKey) (*Method, error) {
	if privateKey == nil && publicKey == nil {
		return nil, nil
	}

	var curve elliptic.Curve
	var privateKeyCurveParams *elliptic.CurveParams
	var publicKeyCurveParams *elliptic.CurveParams

	if privateKey != nil {
		privateKeyCurveParams = getCurveParams(privateKey.Curve)

		curve = privateKey.Curve
	}

	if publicKey != nil {
		publicKeyCurve := publicKey.Curve
		publicKeyCurveParams = getCurveParams(publicKeyCurve)

		if utils.IsNil(curve) {
			curve = publicKeyCurve
		}
	}

	// The curve == nil check is redundant with utils.IsNil but lets static analysis
	// narrow curve to non-nil for the curve.Params() call below.
	if curve == nil || utils.IsNil(curve) {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("curve"))
	}

	if privateKeyCurveParams != nil && publicKeyCurveParams != nil && privateKeyCurveParams.Name != publicKeyCurveParams.Name {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w (private/public)", altshiftCryptoErrors.ErrCurveMismatch),
			privateKeyCurveParams.Name, publicKeyCurveParams.Name,
		)
	}

	curveParams := curve.Params()

	name, hashFunc, err := deriveAlgFromCurveParams(curveParams)
	if err != nil {
		return nil, err
	}

	size := (curveParams.BitSize + 7) / 8

	return &Method{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
		HashFunc:   hashFunc,
		Name:       name,
		curve:      curve,
		size:       size,
	}, nil
}

type Asn1DerEncodedMethod struct {
	Method
}

func (m *Asn1DerEncodedMethod) Sign(message []byte) ([]byte, error) {
	raw, err := m.Method.Sign(message)
	if err != nil {
		return nil, fmt.Errorf("method sign: %w", err)
	}

	if len(raw) != 2*m.size {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: invalid signature length: %d", altshiftErrors.ErrValidationError, len(raw)))
	}

	r := new(big.Int).SetBytes(raw[:m.size])
	s := new(big.Int).SetBytes(raw[m.size:])

	data, err := asn1.Marshal(struct{ R, S *big.Int }{R: r, S: s})
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("asn1 marshal: %w", err), r, s)
	}

	return data, nil
}

func (m *Asn1DerEncodedMethod) Verify(message []byte, signature []byte) error {
	publicKey := m.PublicKey
	if publicKey == nil && m.PrivateKey != nil {
		publicKey = &m.PrivateKey.PublicKey
	}
	if publicKey == nil {
		return altshiftErrors.NewWithTrace(empty_error.New("public key"))
	}

	var decodedSignature struct {
		R, S *big.Int
	}

	if _, err := asn1.Unmarshal(signature, &decodedSignature); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("asn1 unmarshal: %w", err))
	}

	digest, err := m.hash(message)
	if err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("hash: %w", err))
	}

	if !ecdsa.Verify(publicKey, digest, decodedSignature.R, decodedSignature.S) {
		return altshiftErrors.NewWithTrace(altshiftCryptoErrors.ErrSignatureMismatch)
	}

	return nil
}

func FromPem(pemKey string) (*Method, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("block"))
	}

	blockBytes := block.Bytes
	privateKey, err := x509.ParseECPrivateKey(blockBytes)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("x509 parse pkcs8 private key: %w", err), blockBytes)
	}
	if privateKey == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("private key"))
	}

	method, err := New(privateKey, &privateKey.PublicKey)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("new method: %w", err), privateKey)
	}

	return method, nil
}
