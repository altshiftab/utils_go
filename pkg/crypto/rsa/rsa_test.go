package rsa

import (
	"crypto/rand"
	crsa "crypto/rsa"
	"errors"
	"math/big"
	"testing"

	altshiftCryptoErrors "github.com/altshiftab/utils_go/pkg/crypto/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

func mustGenerate(t *testing.T, bits int) *crsa.PrivateKey {
	t.Helper()
	key, err := crsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatalf("rsa generate key: %v", err)
	}
	return key
}

func makeFakePublicKey(bits int) *crsa.PublicKey {
	one := big.NewInt(1)
	n := new(big.Int).Lsh(one, uint(bits))
	n.Sub(n, one)
	return &crsa.PublicKey{N: n, E: 65537}
}

func TestNew_Algorithms(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		alg     string
		wantPSS bool
	}{
		{name: "RS256", alg: "RS256"},
		{name: "RS384", alg: "RS384"},
		{name: "RS512", alg: "RS512"},
		{name: "PS256", alg: "PS256", wantPSS: true},
		{name: "PS384", alg: "PS384", wantPSS: true},
		{name: "PS512", alg: "PS512", wantPSS: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, err := New(tc.alg, nil, nil)
			if err != nil {
				t.Fatalf("New(%q): %v", tc.alg, err)
			}
			if m == nil {
				t.Fatal("got nil method")
			}
			if m.GetName() != tc.alg {
				t.Fatalf("name: got %q want %q", m.GetName(), tc.alg)
			}
			if m.pss != tc.wantPSS {
				t.Fatalf("pss: got %v want %v", m.pss, tc.wantPSS)
			}
		})
	}
}

func TestNew_UnsupportedAlgorithm(t *testing.T) {
	t.Parallel()

	_, err := New("RS999", nil, nil)
	if err == nil {
		t.Fatal("expected error for unsupported algorithm")
	}
	if !errors.Is(err, altshiftCryptoErrors.ErrUnsupportedAlgorithm) {
		t.Fatalf("expected ErrUnsupportedAlgorithm, got %v", err)
	}
}

func TestNewFromPublicKey_Nil(t *testing.T) {
	t.Parallel()

	_, err := NewFromPublicKey(nil)
	if err == nil {
		t.Fatal("expected error for nil public key")
	}
	ee, ok := errors.AsType[*empty_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
	}
	if ee.Field != "public key" {
		t.Errorf("Field = %q, want %q", ee.Field, "public key")
	}
}

func TestNewFromPublicKey_InferenceBySize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		bits int
		want string
	}{
		{2048, "RS256"},
		{3072, "RS384"},
		{4096, "RS512"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			pk := makeFakePublicKey(tc.bits)
			m, err := NewFromPublicKey(pk)
			if err != nil {
				t.Fatalf("NewFromPublicKey(%d bits): %v", tc.bits, err)
			}
			if got := m.GetName(); got != tc.want {
				t.Fatalf("NewFromPublicKey(%d bits): got %q want %q", tc.bits, got, tc.want)
			}
		})
	}
}

func TestSignVerify_RoundTrip_PKCS1v15(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, 2048)
	algs := []string{"RS256", "RS384", "RS512"}
	for _, alg := range algs {
		alg := alg
		t.Run(alg, func(t *testing.T) {
			t.Parallel()

			m, err := New(alg, priv, &priv.PublicKey)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			message := []byte("hello world")
			sig, err := m.Sign(message)
			if err != nil {
				t.Fatalf("Sign: %v", err)
			}
			if len(sig) == 0 {
				t.Fatal("empty signature")
			}
			if err := m.Verify(message, sig); err != nil {
				t.Fatalf("Verify: %v", err)
			}
		})
	}
}

func TestSignVerify_RoundTrip_PSS(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, 2048)
	algs := []string{"PS256", "PS384", "PS512"}
	for _, alg := range algs {
		alg := alg
		t.Run(alg, func(t *testing.T) {
			t.Parallel()

			m, err := New(alg, priv, &priv.PublicKey)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			message := []byte("hello world")
			sig, err := m.Sign(message)
			if err != nil {
				t.Fatalf("Sign: %v", err)
			}
			if err := m.Verify(message, sig); err != nil {
				t.Fatalf("Verify: %v", err)
			}
		})
	}
}

func TestVerify_DerivesPublicKeyFromPrivate(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, 2048)
	signer, err := New("RS256", priv, nil) // no public key
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	sig, err := signer.Sign([]byte("hello"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := signer.Verify([]byte("hello"), sig); err != nil {
		t.Fatalf("Verify (derived public key): %v", err)
	}
}

func TestSign_NoPrivateKey(t *testing.T) {
	t.Parallel()

	priv := mustGenerate(t, 2048)
	m, err := New("RS256", nil, &priv.PublicKey)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = m.Sign([]byte("hello"))
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

func TestVerify_NoPublicKey(t *testing.T) {
	t.Parallel()

	m, err := New("RS256", nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = m.Verify([]byte("hello"), []byte{0x00})
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

func TestVerify_DifferentKeyFails_PKCS1v15(t *testing.T) {
	t.Parallel()

	signerKey := mustGenerate(t, 2048)
	verifierKey := mustGenerate(t, 2048)

	signer, err := New("RS256", signerKey, &signerKey.PublicKey)
	if err != nil {
		t.Fatalf("New (signer): %v", err)
	}
	verifier, err := New("RS256", nil, &verifierKey.PublicKey)
	if err != nil {
		t.Fatalf("New (verifier): %v", err)
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

func TestVerify_DifferentKeyFails_PSS(t *testing.T) {
	t.Parallel()

	signerKey := mustGenerate(t, 2048)
	verifierKey := mustGenerate(t, 2048)

	signer, err := New("PS256", signerKey, &signerKey.PublicKey)
	if err != nil {
		t.Fatalf("New (signer): %v", err)
	}
	verifier, err := New("PS256", nil, &verifierKey.PublicKey)
	if err != nil {
		t.Fatalf("New (verifier): %v", err)
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

func TestGetName(t *testing.T) {
	t.Parallel()

	m := &Method{Name: "custom"}
	if got := m.GetName(); got != "custom" {
		t.Fatalf("GetName: got %q want %q", got, "custom")
	}
}
