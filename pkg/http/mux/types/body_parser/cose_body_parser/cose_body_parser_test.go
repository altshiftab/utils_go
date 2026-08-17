package cose_body_parser

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cose"
)

func testKey(t *testing.T) *ecdh.PrivateKey {
	t.Helper()

	privateKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	return privateKey
}

func encryptedMessage(t *testing.T, privateKey *ecdh.PrivateKey, plaintext []byte) []byte {
	t.Helper()

	message, err := cose.Encrypt(
		plaintext,
		privateKey.PublicKey(),
		&cose.EncryptOptions{
			ContentType:   "application/cbor",
			KeyIdentifier: []byte("test-kid"),
		},
	)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	return message
}

func newParser(t *testing.T, privateKey *ecdh.PrivateKey, options ...Option) *Parser {
	t.Helper()

	parser, err := New(privateKey, options...)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	return parser
}

func TestParseValid(t *testing.T) {
	t.Parallel()

	privateKey := testKey(t)
	plaintext := []byte("plaintext")

	parser := newParser(
		t,
		privateKey,
		WithKeyIdentifier([]byte("test-kid")),
		WithPlaintextContentType("application/cbor"),
	)

	decrypted, responseError := parser.Parse(nil, encryptedMessage(t, privateKey, plaintext))
	if responseError != nil {
		t.Fatalf("parse: %v", responseError)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("plaintext: got %q, want %q", decrypted, plaintext)
	}
}

func TestParseMalformedMessage(t *testing.T) {
	t.Parallel()

	_, responseError := newParser(t, testKey(t)).Parse(nil, []byte{0x9f})
	if responseError == nil {
		t.Fatal("expected a response error")
	}

	if problemDetail := responseError.ProblemDetail; problemDetail == nil || problemDetail.Status != http.StatusBadRequest {
		t.Errorf("expected a 400 problem detail, got %#v", problemDetail)
	}
}

func TestParseWrongKey(t *testing.T) {
	t.Parallel()

	privateKey := testKey(t)
	otherKey := testKey(t)

	_, responseError := newParser(t, otherKey).Parse(nil, encryptedMessage(t, privateKey, []byte("x")))
	if responseError == nil {
		t.Fatal("expected a response error")
	}

	if problemDetail := responseError.ProblemDetail; problemDetail == nil || problemDetail.Status != http.StatusBadRequest {
		t.Errorf("expected a 400 problem detail, got %#v", problemDetail)
	}
}

func TestParseKeyIdentifierMismatch(t *testing.T) {
	t.Parallel()

	privateKey := testKey(t)

	parser := newParser(t, privateKey, WithKeyIdentifier([]byte("other-kid")))

	_, responseError := parser.Parse(nil, encryptedMessage(t, privateKey, []byte("x")))
	if responseError == nil {
		t.Fatal("expected a response error")
	}

	if problemDetail := responseError.ProblemDetail; problemDetail == nil || problemDetail.Status != http.StatusBadRequest {
		t.Errorf("expected a 400 problem detail, got %#v", problemDetail)
	}
}

func TestParsePlaintextContentTypeMismatch(t *testing.T) {
	t.Parallel()

	privateKey := testKey(t)

	parser := newParser(t, privateKey, WithPlaintextContentType("application/json"))

	_, responseError := parser.Parse(nil, encryptedMessage(t, privateKey, []byte("x")))
	if responseError == nil {
		t.Fatal("expected a response error")
	}

	if problemDetail := responseError.ProblemDetail; problemDetail == nil || problemDetail.Status != http.StatusBadRequest {
		t.Errorf("expected a 400 problem detail, got %#v", problemDetail)
	}
}

func TestNewNilKey(t *testing.T) {
	t.Parallel()

	if _, err := New(nil); err == nil {
		t.Error("expected an error for a nil private key")
	}
}
