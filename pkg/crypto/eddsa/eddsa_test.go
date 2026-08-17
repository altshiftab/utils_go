package eddsa

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"

	altshiftCryptoErrors "github.com/altshiftab/utils_go/pkg/crypto/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

func mustGenerate(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519 generate: %v", err)
	}
	return pub, priv
}

func mustEncodePrivateKeyAsPem(t *testing.T, key any) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8 private key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestSignVerify_RoundTrip(t *testing.T) {
	t.Parallel()

	pub, priv := mustGenerate(t)
	m := &Method{PrivateKey: priv, PublicKey: pub}

	message := []byte("hello world")
	sig, err := m.Sign(message)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := m.Verify(message, sig); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerify_DerivesPublicKeyFromPrivate(t *testing.T) {
	t.Parallel()

	_, priv := mustGenerate(t)
	signer := &Method{PrivateKey: priv}
	sig, err := signer.Sign([]byte("hello"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	verifier := &Method{PrivateKey: priv} // no PublicKey set; should derive
	if err := verifier.Verify([]byte("hello"), sig); err != nil {
		t.Fatalf("Verify (derived public): %v", err)
	}
}

func TestSign_EmptyPrivateKey(t *testing.T) {
	t.Parallel()

	m := &Method{}
	_, err := m.Sign([]byte("hello"))
	if err == nil {
		t.Fatal("expected error for empty private key")
	}
	ee, ok := errors.AsType[*empty_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
	}
	if ee.Field != "private key" {
		t.Errorf("Field = %q, want %q", ee.Field, "private key")
	}
}

func TestVerify_EmptyPublicKey(t *testing.T) {
	t.Parallel()

	m := &Method{} // both keys empty
	err := m.Verify([]byte("hello"), []byte{0x00})
	if err == nil {
		t.Fatal("expected error for empty public key")
	}
	ee, ok := errors.AsType[*empty_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
	}
	if ee.Field != "public key" {
		t.Errorf("Field = %q, want %q", ee.Field, "public key")
	}
}

func TestVerify_SignatureMismatch(t *testing.T) {
	t.Parallel()

	pubA, privA := mustGenerate(t)
	pubB, _ := mustGenerate(t)
	_ = pubA

	signer := &Method{PrivateKey: privA, PublicKey: pubA}
	sig, err := signer.Sign([]byte("hello"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	verifier := &Method{PublicKey: pubB}
	err = verifier.Verify([]byte("hello"), sig)
	if !errors.Is(err, altshiftCryptoErrors.ErrSignatureMismatch) {
		t.Fatalf("expected ErrSignatureMismatch, got %v", err)
	}
}

func TestGetName(t *testing.T) {
	t.Parallel()

	m := &Method{}
	if got := m.GetName(); got != Name {
		t.Fatalf("GetName: got %q want %q", got, Name)
	}
}

func TestFromPem_HappyPath(t *testing.T) {
	t.Parallel()

	_, priv := mustGenerate(t)
	pemKey := mustEncodePrivateKeyAsPem(t, priv)

	m, err := FromPem(pemKey)
	if err != nil {
		t.Fatalf("FromPem: %v", err)
	}
	if m == nil {
		t.Fatal("got nil method")
	}
	if string(m.PrivateKey) != string(priv) {
		t.Fatal("private key mismatch")
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

func TestFromPem_GarbageReturnsError(t *testing.T) {
	t.Parallel()

	_, err := FromPem("not a pem block")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFromPem_WrongKeyTypeReturnsError(t *testing.T) {
	t.Parallel()

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa generate key: %v", err)
	}
	pemKey := mustEncodePrivateKeyAsPem(t, rsaKey)

	_, err = FromPem(pemKey)
	if err == nil {
		t.Fatal("expected error for non-ed25519 PEM")
	}
}
