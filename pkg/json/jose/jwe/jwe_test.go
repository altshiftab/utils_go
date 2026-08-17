package jwe

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// TestConcatKdf checks the Concat KDF against the test vector in
// RFC 7518 Appendix C.
func TestConcatKdf(t *testing.T) {
	t.Parallel()

	z := []byte{
		158, 86, 217, 29, 129, 113, 53, 211, 114, 131, 66, 131, 191, 132, 38, 156,
		251, 49, 110, 163, 218, 128, 106, 72, 246, 218, 167, 121, 140, 254, 144, 196,
	}

	derivedKey, err := concatKdf(z, "A128GCM", []byte("Alice"), []byte("Bob"), 128)
	if err != nil {
		t.Fatalf("concat kdf: %v", err)
	}

	expected := "VqqN6vgjbSBcIijNcacQGg"
	if got := base64.RawURLEncoding.EncodeToString(derivedKey); got != expected {
		t.Errorf("derived key = %q, want %q", got, expected)
	}
}

func mustGenerateKey(t *testing.T, curve elliptic.Curve) *ecdsa.PrivateKey {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}

	return privateKey
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		curve       elliptic.Curve
		plaintext   []byte
		keyId       string
		contentType string
	}{
		{
			name:        "p-256 with key id and content type",
			curve:       elliptic.P256(),
			plaintext:   []byte(`{"message":"hello"}`),
			keyId:       "key-1",
			contentType: "application/json",
		},
		{
			name:      "p-256 without optional headers",
			curve:     elliptic.P256(),
			plaintext: []byte("plain text"),
		},
		{
			name:      "p-256 empty plaintext",
			curve:     elliptic.P256(),
			plaintext: []byte{},
		},
		{
			name:      "p-256 large plaintext",
			curve:     elliptic.P256(),
			plaintext: bytes.Repeat([]byte("0123456789abcdef"), 1024),
		},
		{
			name:      "p-384",
			curve:     elliptic.P384(),
			plaintext: []byte("p-384 plaintext"),
		},
		{
			name:      "p-521",
			curve:     elliptic.P521(),
			plaintext: []byte("p-521 plaintext"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			recipientPrivateKey := mustGenerateKey(t, testCase.curve)

			encrypter, err := NewEncrypter(KeyAlgorithmEcdhEs, ContentEncryptionA256Gcm, &recipientPrivateKey.PublicKey)
			if err != nil {
				t.Fatalf("new encrypter: %v", err)
			}
			encrypter.KeyId = testCase.keyId
			encrypter.ContentType = testCase.contentType

			serialization, err := encrypter.Encrypt(testCase.plaintext)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}

			encryption, err := ParseCompact(
				serialization,
				[]KeyAlgorithm{KeyAlgorithmEcdhEs},
				[]ContentEncryption{ContentEncryptionA256Gcm},
			)
			if err != nil {
				t.Fatalf("parse compact: %v", err)
			}

			header := encryption.Header
			if header.KeyId != testCase.keyId {
				t.Errorf("header key id = %q, want %q", header.KeyId, testCase.keyId)
			}
			if header.ContentType != testCase.contentType {
				t.Errorf("header content type = %q, want %q", header.ContentType, testCase.contentType)
			}

			for _, privateKey := range []any{recipientPrivateKey, mustEcdh(t, recipientPrivateKey)} {
				plaintext, err := encryption.Decrypt(privateKey)
				if err != nil {
					t.Fatalf("decrypt (%T): %v", privateKey, err)
				}
				if !bytes.Equal(plaintext, testCase.plaintext) {
					t.Errorf("plaintext (%T) = %q, want %q", privateKey, plaintext, testCase.plaintext)
				}
			}
		})
	}
}

func mustEcdh(t *testing.T, privateKey *ecdsa.PrivateKey) any {
	t.Helper()

	ecdhPrivateKey, err := privateKey.ECDH()
	if err != nil {
		t.Fatalf("ecdsa private key ecdh: %v", err)
	}

	return ecdhPrivateKey
}

func mustEncrypt(t *testing.T, recipientPrivateKey *ecdsa.PrivateKey, plaintext []byte) string {
	t.Helper()

	encrypter, err := NewEncrypter(KeyAlgorithmEcdhEs, ContentEncryptionA256Gcm, &recipientPrivateKey.PublicKey)
	if err != nil {
		t.Fatalf("new encrypter: %v", err)
	}

	serialization, err := encrypter.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	return serialization
}

func TestNewEncrypterFailures(t *testing.T) {
	t.Parallel()

	recipientPrivateKey := mustGenerateKey(t, elliptic.P256())

	testCases := []struct {
		name              string
		keyAlgorithm      KeyAlgorithm
		contentEncryption ContentEncryption
		publicKey         *ecdsa.PublicKey
		expectedError     error
	}{
		{
			name:              "unsupported key algorithm",
			keyAlgorithm:      "RSA-OAEP-256",
			contentEncryption: ContentEncryptionA256Gcm,
			publicKey:         &recipientPrivateKey.PublicKey,
			expectedError:     ErrUnsupportedKeyAlgorithm,
		},
		{
			name:              "unsupported content encryption",
			keyAlgorithm:      KeyAlgorithmEcdhEs,
			contentEncryption: "A128CBC-HS256",
			publicKey:         &recipientPrivateKey.PublicKey,
			expectedError:     ErrUnsupportedContentEncryption,
		},
		{
			name:              "nil public key",
			keyAlgorithm:      KeyAlgorithmEcdhEs,
			contentEncryption: ContentEncryptionA256Gcm,
			publicKey:         nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewEncrypter(testCase.keyAlgorithm, testCase.contentEncryption, testCase.publicKey)
			if err == nil {
				t.Fatal("expected an error")
			}
			if testCase.expectedError != nil && !errors.Is(err, testCase.expectedError) {
				t.Errorf("error = %v, want %v", err, testCase.expectedError)
			}
		})
	}
}

func TestParseCompactFailures(t *testing.T) {
	t.Parallel()

	recipientPrivateKey := mustGenerateKey(t, elliptic.P256())
	serialization := mustEncrypt(t, recipientPrivateKey, []byte("plaintext"))
	parts := strings.Split(serialization, ".")

	makeHeader := func(header string) string {
		return strings.Join(
			[]string{base64.RawURLEncoding.EncodeToString([]byte(header)), parts[1], parts[2], parts[3], parts[4]},
			".",
		)
	}

	testCases := []struct {
		name          string
		serialization string
		expectedError error
	}{
		{
			name:          "empty serialization",
			serialization: "",
			expectedError: altshiftErrors.ErrParseError,
		},
		{
			name:          "wrong number of parts",
			serialization: "a.b.c",
			expectedError: altshiftErrors.ErrParseError,
		},
		{
			name:          "invalid protected header base64",
			serialization: "!." + strings.Join(parts[1:], "."),
			expectedError: altshiftErrors.ErrParseError,
		},
		{
			name:          "invalid protected header json",
			serialization: makeHeader("{"),
			expectedError: altshiftErrors.ErrParseError,
		},
		{
			name:          "disallowed key algorithm",
			serialization: makeHeader(`{"alg":"RSA-OAEP-256","enc":"A256GCM"}`),
			expectedError: ErrUnsupportedKeyAlgorithm,
		},
		{
			name:          "disallowed content encryption",
			serialization: makeHeader(`{"alg":"ECDH-ES","enc":"A128GCM"}`),
			expectedError: ErrUnsupportedContentEncryption,
		},
		{
			name:          "compression",
			serialization: makeHeader(`{"alg":"ECDH-ES","enc":"A256GCM","zip":"DEF","epk":{"kty":"EC","crv":"P-256","x":"AA","y":"AA"}}`),
			expectedError: ErrUnsupportedCompression,
		},
		{
			name:          "missing ephemeral public key",
			serialization: makeHeader(`{"alg":"ECDH-ES","enc":"A256GCM"}`),
			expectedError: altshiftErrors.ErrValidationError,
		},
		{
			name:          "unexpected encrypted key",
			serialization: strings.Join([]string{parts[0], "AAAA", parts[2], parts[3], parts[4]}, "."),
			expectedError: ErrUnexpectedEncryptedKey,
		},
		{
			name:          "invalid initialization vector length",
			serialization: strings.Join([]string{parts[0], parts[1], "AAAA", parts[3], parts[4]}, "."),
			expectedError: altshiftErrors.ErrParseError,
		},
		{
			name:          "invalid tag length",
			serialization: strings.Join([]string{parts[0], parts[1], parts[2], parts[3], "AAAA"}, "."),
			expectedError: altshiftErrors.ErrParseError,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseCompact(
				testCase.serialization,
				[]KeyAlgorithm{KeyAlgorithmEcdhEs},
				[]ContentEncryption{ContentEncryptionA256Gcm},
			)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, testCase.expectedError) {
				t.Errorf("error = %v, want %v", err, testCase.expectedError)
			}
		})
	}
}

func TestDecryptFailures(t *testing.T) {
	t.Parallel()

	recipientPrivateKey := mustGenerateKey(t, elliptic.P256())
	serialization := mustEncrypt(t, recipientPrivateKey, []byte("plaintext"))

	tamper := func(t *testing.T, serialization string, partIndex int) string {
		t.Helper()

		parts := strings.Split(serialization, ".")
		data, err := base64.RawURLEncoding.DecodeString(parts[partIndex])
		if err != nil {
			t.Fatalf("base64 decode part %d: %v", partIndex, err)
		}
		if len(data) == 0 {
			t.Fatalf("empty part %d", partIndex)
		}
		data[len(data)-1] ^= 1
		parts[partIndex] = base64.RawURLEncoding.EncodeToString(data)

		return strings.Join(parts, ".")
	}

	testCases := []struct {
		name          string
		serialization string
		privateKey    any
		expectedError error
	}{
		{
			name:          "wrong private key",
			serialization: serialization,
			privateKey:    mustGenerateKey(t, elliptic.P256()),
			expectedError: altshiftErrors.ErrVerificationError,
		},
		{
			name:          "curve mismatch",
			serialization: serialization,
			privateKey:    mustGenerateKey(t, elliptic.P384()),
			expectedError: altshiftErrors.ErrVerificationError,
		},
		{
			name:          "tampered ciphertext",
			serialization: tamper(t, serialization, 3),
			privateKey:    recipientPrivateKey,
			expectedError: altshiftErrors.ErrVerificationError,
		},
		{
			name:          "tampered tag",
			serialization: tamper(t, serialization, 4),
			privateKey:    recipientPrivateKey,
			expectedError: altshiftErrors.ErrVerificationError,
		},
		{
			name:          "nil private key",
			serialization: serialization,
			privateKey:    nil,
		},
		{
			name:          "unsupported private key type",
			serialization: serialization,
			privateKey:    "not a key",
			expectedError: ErrUnsupportedKeyType,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			encryption, err := ParseCompact(
				testCase.serialization,
				[]KeyAlgorithm{KeyAlgorithmEcdhEs},
				[]ContentEncryption{ContentEncryptionA256Gcm},
			)
			if err != nil {
				t.Fatalf("parse compact: %v", err)
			}

			_, err = encryption.Decrypt(testCase.privateKey)
			if err == nil {
				t.Fatal("expected an error")
			}
			if testCase.expectedError != nil && !errors.Is(err, testCase.expectedError) {
				t.Errorf("error = %v, want %v", err, testCase.expectedError)
			}
		})
	}
}

// TestDecryptGoJoseFixture decrypts a JWE produced by
// github.com/go-jose/go-jose/v4 v4.1.3, frozen from a differential
// verification run, to lock in wire-format interoperability.
func TestDecryptGoJoseFixture(t *testing.T) {
	t.Parallel()

	privateKeyData, err := base64.RawURLEncoding.DecodeString("9U8B0t9W4RsiURKmBGpoqzFZFxoYd_VQ71SLq19VPCM")
	if err != nil {
		t.Fatalf("base64 decode private key: %v", err)
	}

	privateKey, err := ecdh.P256().NewPrivateKey(privateKeyData)
	if err != nil {
		t.Fatalf("ecdh new private key: %v", err)
	}

	serialization := "eyJhbGciOiJFQ0RILUVTIiwiY3R5IjoiYXBwbGljYXRpb24vanNvbiIsImVuYyI6IkEyNTZHQ00iLCJlcGsiOnsia3R5IjoiRUMiLCJjcnYiOiJQLTI1NiIsIngiOiJ2ZUN2aV95V3hQMEhjeEh6SWMwdWJNMF9XVXkyUE02N0NlT0V2aFRWWi0wIiwieSI6IjlSdlNaU3p1SVFjbkRmUlpiM2NmbWlRS1VEYWxuaDN3c2dsdUtVOURrOWsifSwia2lkIjoiZml4dHVyZS1rZXkifQ..Ab2Sk3RimpJ4Sk-6.mAAL0gQ14UXySAp4IKmPEw4JdrK7.qdUOxN43W-rFaRun-MfpoQ"

	encryption, err := ParseCompact(
		serialization,
		[]KeyAlgorithm{KeyAlgorithmEcdhEs},
		[]ContentEncryption{ContentEncryptionA256Gcm},
	)
	if err != nil {
		t.Fatalf("parse compact: %v", err)
	}

	header := encryption.Header
	if header.KeyId != "fixture-key" {
		t.Errorf("header key id = %q, want %q", header.KeyId, "fixture-key")
	}
	if header.ContentType != "application/json" {
		t.Errorf("header content type = %q, want %q", header.ContentType, "application/json")
	}

	plaintext, err := encryption.Decrypt(privateKey)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	expected := `{"fixture":"go-jose"}`
	if string(plaintext) != expected {
		t.Errorf("plaintext = %q, want %q", plaintext, expected)
	}
}

// TestDecryptTamperedHeader checks that modifying the protected header
// (the additional authenticated data) makes decryption fail.
func TestDecryptTamperedHeader(t *testing.T) {
	t.Parallel()

	recipientPrivateKey := mustGenerateKey(t, elliptic.P256())

	encrypter, err := NewEncrypter(KeyAlgorithmEcdhEs, ContentEncryptionA256Gcm, &recipientPrivateKey.PublicKey)
	if err != nil {
		t.Fatalf("new encrypter: %v", err)
	}
	encrypter.KeyId = "key-1"

	serialization, err := encrypter.Encrypt([]byte("plaintext"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	parts := strings.Split(serialization, ".")
	headerData, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("base64 decode protected header: %v", err)
	}
	parts[0] = base64.RawURLEncoding.EncodeToString(
		bytes.Replace(headerData, []byte(`"key-1"`), []byte(`"key-2"`), 1),
	)

	encryption, err := ParseCompact(
		strings.Join(parts, "."),
		[]KeyAlgorithm{KeyAlgorithmEcdhEs},
		[]ContentEncryption{ContentEncryptionA256Gcm},
	)
	if err != nil {
		t.Fatalf("parse compact: %v", err)
	}

	if _, err := encryption.Decrypt(recipientPrivateKey); !errors.Is(err, altshiftErrors.ErrVerificationError) {
		t.Errorf("error = %v, want %v", err, altshiftErrors.ErrVerificationError)
	}
}
