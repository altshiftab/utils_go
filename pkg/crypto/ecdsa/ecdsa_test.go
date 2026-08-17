package ecdsa

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"

	altshiftCryptoErrors "github.com/altshiftab/utils_go/pkg/crypto/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

func mustGenerate(t *testing.T, curve elliptic.Curve) *ecdsa.PrivateKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate: %v", err)
	}
	return priv
}

func mustEncodeEcPrivateKeyPem(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal ec private key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}))
}

func TestNew_BothNilReturnsNil(t *testing.T) {
	t.Parallel()

	m, err := New(nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if m != nil {
		t.Fatalf("got %v want nil", m)
	}
}

func TestNew_PrivateKeyOnly(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, elliptic.P256())
	m, err := New(priv, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if m == nil {
		t.Fatal("got nil method")
	}
	if m.GetName() != "ES256" {
		t.Fatalf("name: got %q want ES256", m.GetName())
	}
}

func TestNew_PublicKeyOnly(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, elliptic.P384())
	m, err := New(nil, &priv.PublicKey)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if m == nil {
		t.Fatal("got nil method")
	}
	if m.GetName() != "ES384" {
		t.Fatalf("name: got %q want ES384", m.GetName())
	}
}

func TestNew_CurveMismatch(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, elliptic.P256())
	other := mustGenerate(t, elliptic.P384())

	_, err := New(priv, &other.PublicKey)
	if err == nil {
		t.Fatal("expected curve mismatch error")
	}
	if !errors.Is(err, altshiftCryptoErrors.ErrCurveMismatch) {
		t.Fatalf("expected ErrCurveMismatch, got %v", err)
	}
}

func TestNew_NilCurve(t *testing.T) {
	t.Parallel()

	priv := &ecdsa.PrivateKey{} // both Curve and PublicKey.Curve are nil
	_, err := New(priv, nil)
	if err == nil {
		t.Fatal("expected nil curve error")
	}
	ne, ok := errors.AsType[*nil_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *nil_error.Error", err, err)
	}
	if ne.Field != "curve" {
		t.Errorf("Field = %q, want %q", ne.Field, "curve")
	}
}

func TestNew_UnsupportedCurve(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, elliptic.P224())
	_, err := New(priv, nil)
	if err == nil {
		t.Fatal("expected unsupported curve error")
	}
	if !errors.Is(err, altshiftCryptoErrors.ErrUnsupportedCurve) {
		t.Fatalf("expected ErrUnsupportedCurve, got %v", err)
	}
}

func TestFromPublicKey(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, elliptic.P521())
	m, err := FromPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if m == nil {
		t.Fatal("got nil method")
	}
	if m.GetName() != "ES512" {
		t.Fatalf("name: got %q want ES512", m.GetName())
	}
}

func TestFromPrivateKey(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, elliptic.P256())
	m, err := FromPrivateKey(priv)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if m == nil {
		t.Fatal("got nil method")
	}
	if m.PrivateKey != priv {
		t.Fatal("private key not retained")
	}
}

func TestSignVerify_RoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		curve elliptic.Curve
	}{
		{name: "P-256", curve: elliptic.P256()},
		{name: "P-384", curve: elliptic.P384()},
		{name: "P-521", curve: elliptic.P521()},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			priv := mustGenerate(t, tc.curve)
			m, err := New(priv, &priv.PublicKey)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			message := []byte("hello world")
			sig, err := m.Sign(message)
			if err != nil {
				t.Fatalf("Sign: %v", err)
			}
			if len(sig) != 2*m.size {
				t.Fatalf("sig length: got %d want %d", len(sig), 2*m.size)
			}

			if err := m.Verify(message, sig); err != nil {
				t.Fatalf("Verify: %v", err)
			}
		})
	}
}

func TestSign_NoPrivateKey(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, elliptic.P256())
	verifierOnly, err := FromPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("FromPublicKey: %v", err)
	}

	_, err = verifierOnly.Sign([]byte("hello"))
	if err == nil {
		t.Fatal("expected error for missing private key")
	}
	ee, ok := errors.AsType[*empty_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
	}
	if ee.Field != "secret" {
		t.Errorf("Field = %q, want %q", ee.Field, "secret")
	}
}

func TestVerify_DerivesPublicKeyFromPrivate(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, elliptic.P256())
	signerOnly, err := FromPrivateKey(priv)
	if err != nil {
		t.Fatalf("FromPrivateKey: %v", err)
	}

	sig, err := signerOnly.Sign([]byte("hello"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// Verify uses derived public key from PrivateKey
	if err := signerOnly.Verify([]byte("hello"), sig); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerify_WrongLengthIsMismatch(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, elliptic.P256())
	m, err := New(priv, &priv.PublicKey)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = m.Verify([]byte("hello"), []byte{0x01, 0x02})
	if !errors.Is(err, altshiftCryptoErrors.ErrSignatureMismatch) {
		t.Fatalf("expected ErrSignatureMismatch, got %v", err)
	}
}

func TestVerify_DifferentKeyFails(t *testing.T) {
	t.Parallel()

	signerKey := mustGenerate(t, elliptic.P256())
	signer, err := New(signerKey, &signerKey.PublicKey)
	if err != nil {
		t.Fatalf("New (signer): %v", err)
	}

	verifierKey := mustGenerate(t, elliptic.P256())
	verifier, err := FromPublicKey(&verifierKey.PublicKey)
	if err != nil {
		t.Fatalf("FromPublicKey (verifier): %v", err)
	}

	sig, err := signer.Sign([]byte("hello"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	err = verifier.Verify([]byte("hello"), sig)
	if !errors.Is(err, altshiftCryptoErrors.ErrSignatureMismatch) {
		t.Fatalf("expected ErrSignatureMismatch, got %v", err)
	}
}

func TestVerify_NoPublicKey(t *testing.T) {
	t.Parallel()

	m := &Method{size: 32, curve: elliptic.P256()}
	err := m.Verify([]byte("hello"), make([]byte, 64))
	if err == nil {
		t.Fatal("expected error for missing public key")
	}
	ee, ok := errors.AsType[*empty_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
	}
	if ee.Field != "public key" {
		t.Errorf("Field = %q, want %q", ee.Field, "public key")
	}
}

func TestAsn1DerEncodedMethod_RoundTrip(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, elliptic.P256())
	base, err := New(priv, &priv.PublicKey)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if base == nil {
		t.Fatal("nil method")
	}

	m := &Asn1DerEncodedMethod{Method: *base}
	message := []byte("hello world")
	sig, err := m.Sign(message)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := m.Verify(message, sig); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestAsn1DerEncodedMethod_VerifyGarbage(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, elliptic.P256())
	base, err := New(priv, &priv.PublicKey)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if base == nil {
		t.Fatal("nil method")
	}

	m := &Asn1DerEncodedMethod{Method: *base}
	err = m.Verify([]byte("hello"), []byte("not asn1"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFromPem_HappyPath(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, elliptic.P256())
	pemKey := mustEncodeEcPrivateKeyPem(t, priv)

	m, err := FromPem(pemKey)
	if err != nil {
		t.Fatalf("FromPem: %v", err)
	}
	if m == nil {
		t.Fatal("got nil method")
	}
	if m.GetName() != "ES256" {
		t.Fatalf("name: got %q want ES256", m.GetName())
	}

	message := []byte("hello")
	sig, err := m.Sign(message)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := m.Verify(message, sig); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestFromPem_GarbageReturnsNilBlockError(t *testing.T) {
	t.Parallel()

	_, err := FromPem("not a pem block")
	if err == nil {
		t.Fatal("expected error")
	}
	ne, ok := errors.AsType[*nil_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *nil_error.Error", err, err)
	}
	if ne.Field != "block" {
		t.Errorf("Field = %q, want %q", ne.Field, "block")
	}
}

func TestFromPem_InvalidBlockBytes(t *testing.T) {
	t.Parallel()

	bad := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("garbage")}))
	_, err := FromPem(bad)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetName(t *testing.T) {
	t.Parallel()

	m := &Method{Name: "custom"}
	if got := m.GetName(); got != "custom" {
		t.Fatalf("GetName: got %q want %q", got, "custom")
	}
}
