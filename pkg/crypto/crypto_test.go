package crypto

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

func mustGenerateRsaKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa generate key: %v", err)
	}
	return key
}

func mustEncodePrivateKeyAsPem(t *testing.T, key any) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8 private key: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	return string(pem.EncodeToMemory(block))
}

func mustEncodePublicKeyAsPem(t *testing.T, key any) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		t.Fatalf("marshal pkix public key: %v", err)
	}
	block := &pem.Block{Type: "PUBLIC KEY", Bytes: der}
	return string(pem.EncodeToMemory(block))
}

func mustMarshalPkixDer(t *testing.T, key any) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		t.Fatalf("marshal pkix public key: %v", err)
	}
	return der
}

func mustGenerateSelfSignedCertificate(t *testing.T) *x509.Certificate {
	t.Helper()
	key := mustGenerateRsaKey(t)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert
}

func TestMakeRawDerCertificateChain(t *testing.T) {
	t.Parallel()

	t.Run("nil and empty raw entries are skipped", func(t *testing.T) {
		t.Parallel()

		cert := mustGenerateSelfSignedCertificate(t)
		certs := []*x509.Certificate{nil, cert, {Raw: nil}, {Raw: []byte{}}}

		chain := MakeRawDerCertificateChain(certs)
		if len(chain) != 1 {
			t.Fatalf("chain length: got %d want 1", len(chain))
		}
		if string(chain[0]) != string(cert.Raw) {
			t.Fatalf("chain[0] mismatch")
		}
	})

	t.Run("empty input returns empty chain", func(t *testing.T) {
		t.Parallel()

		chain := MakeRawDerCertificateChain(nil)
		if len(chain) != 0 {
			t.Fatalf("chain length: got %d want 0", len(chain))
		}
	})
}

func TestMakeTlsCertificateFromX509Certificates(t *testing.T) {
	t.Parallel()

	t.Run("empty input returns nil", func(t *testing.T) {
		t.Parallel()

		got := MakeTlsCertificateFromX509Certificates(nil, nil)
		if got != nil {
			t.Fatalf("got = %v, want nil", got)
		}
	})

	t.Run("populates chain leaf and key", func(t *testing.T) {
		t.Parallel()

		cert := mustGenerateSelfSignedCertificate(t)
		key := mustGenerateRsaKey(t)

		got := MakeTlsCertificateFromX509Certificates([]*x509.Certificate{cert}, key)
		if got == nil {
			t.Fatal("got nil tls.Certificate")
		}
		if len(got.Certificate) != 1 {
			t.Fatalf("certificate len: got %d want 1", len(got.Certificate))
		}
		if got.Leaf != cert {
			t.Fatal("leaf is not the input cert")
		}
		if got.PrivateKey != key {
			t.Fatal("private key is not the input key")
		}
	})
}

func TestPublicKeyFromDer(t *testing.T) {
	t.Parallel()

	t.Run("rsa", func(t *testing.T) {
		t.Parallel()
		key := mustGenerateRsaKey(t)
		der := mustMarshalPkixDer(t, &key.PublicKey)

		got, err := PublicKeyFromDer[*rsa.PublicKey](der)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got == nil || got.N.Cmp(key.N) != 0 {
			t.Fatalf("public key mismatch")
		}
	})

	t.Run("ed25519", func(t *testing.T) {
		t.Parallel()
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("ed25519 generate: %v", err)
		}
		der := mustMarshalPkixDer(t, pub)

		got, err := PublicKeyFromDer[ed25519.PublicKey](der)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if string(got) != string(pub) {
			t.Fatalf("public key bytes mismatch")
		}
	})

	t.Run("ecdsa P-256", func(t *testing.T) {
		t.Parallel()
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("ecdsa generate: %v", err)
		}
		der := mustMarshalPkixDer(t, &priv.PublicKey)

		got, err := PublicKeyFromDer[*ecdsa.PublicKey](der)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got == nil || !got.Equal(&priv.PublicKey) {
			t.Fatalf("public key mismatch")
		}
	})

	t.Run("invalid der", func(t *testing.T) {
		t.Parallel()
		_, err := PublicKeyFromDer[*rsa.PublicKey]([]byte("garbage"))
		if err == nil {
			t.Fatal("expected error for invalid DER")
		}
	})

	t.Run("type mismatch yields conversion error", func(t *testing.T) {
		t.Parallel()
		key := mustGenerateRsaKey(t)
		der := mustMarshalPkixDer(t, &key.PublicKey)

		_, err := PublicKeyFromDer[ed25519.PublicKey](der)
		if err == nil {
			t.Fatal("expected error for type mismatch")
		}
	})
}

func TestPublicKeyFromPem(t *testing.T) {
	t.Parallel()

	t.Run("rsa happy path", func(t *testing.T) {
		t.Parallel()
		key := mustGenerateRsaKey(t)
		pemKey := mustEncodePublicKeyAsPem(t, &key.PublicKey)

		got, err := PublicKeyFromPem[*rsa.PublicKey](pemKey)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got == nil || got.N.Cmp(key.N) != 0 {
			t.Fatalf("public key mismatch")
		}
	})

	t.Run("garbage pem produces nil block error", func(t *testing.T) {
		t.Parallel()
		_, err := PublicKeyFromPem[*rsa.PublicKey]("not a pem block")
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
	})

	t.Run("invalid block bytes propagates parse error", func(t *testing.T) {
		t.Parallel()

		// Valid PEM wrapper but garbage payload
		block := &pem.Block{Type: "PUBLIC KEY", Bytes: []byte("garbage")}
		bad := string(pem.EncodeToMemory(block))

		_, err := PublicKeyFromPem[*rsa.PublicKey](bad)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("type mismatch yields conversion error", func(t *testing.T) {
		t.Parallel()
		key := mustGenerateRsaKey(t)
		pemKey := mustEncodePublicKeyAsPem(t, &key.PublicKey)

		_, err := PublicKeyFromPem[ed25519.PublicKey](pemKey)
		if err == nil {
			t.Fatal("expected conversion error")
		}
	})
}

func TestPrivateKeyFromPem(t *testing.T) {
	t.Parallel()

	t.Run("rsa happy path", func(t *testing.T) {
		t.Parallel()
		key := mustGenerateRsaKey(t)
		pemKey := mustEncodePrivateKeyAsPem(t, key)

		got, err := PrivateKeyFromPem[*rsa.PrivateKey](pemKey)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got == nil || got.N.Cmp(key.N) != 0 {
			t.Fatalf("private key mismatch")
		}
	})

	t.Run("ed25519 happy path", func(t *testing.T) {
		t.Parallel()
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("ed25519 generate: %v", err)
		}
		pemKey := mustEncodePrivateKeyAsPem(t, priv)

		got, err := PrivateKeyFromPem[ed25519.PrivateKey](pemKey)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if string(got) != string(priv) {
			t.Fatalf("private key mismatch")
		}
	})

	t.Run("garbage pem produces nil block error", func(t *testing.T) {
		t.Parallel()
		_, err := PrivateKeyFromPem[*rsa.PrivateKey]("not a pem block")
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
	})

	t.Run("invalid block bytes propagates parse error", func(t *testing.T) {
		t.Parallel()

		block := &pem.Block{Type: "PRIVATE KEY", Bytes: []byte("garbage")}
		bad := string(pem.EncodeToMemory(block))

		_, err := PrivateKeyFromPem[*rsa.PrivateKey](bad)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("type mismatch yields conversion error", func(t *testing.T) {
		t.Parallel()
		key := mustGenerateRsaKey(t)
		pemKey := mustEncodePrivateKeyAsPem(t, key)

		_, err := PrivateKeyFromPem[ed25519.PrivateKey](pemKey)
		if err == nil {
			t.Fatal("expected conversion error")
		}
	})
}
