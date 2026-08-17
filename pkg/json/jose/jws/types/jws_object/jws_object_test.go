package jws_object

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	motmedelCryptoHmac "github.com/altshiftab/utils_go/pkg/crypto/hmac"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// signedJws builds a compact HS256 JWS over the given header/payload JSON with the
// supplied secret, returning the serialization together with the raw parts.
func signedJws(t *testing.T, secret []byte, headerJson, payloadJson string) (string, []byte) {
	t.Helper()

	method, err := motmedelCryptoHmac.New("HS256", secret)
	if err != nil {
		t.Fatalf("hmac new: %v", err)
	}

	headerB64 := base64.RawURLEncoding.EncodeToString([]byte(headerJson))
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(payloadJson))
	signingInput := headerB64 + "." + payloadB64

	signature, err := method.Sign([]byte(signingInput))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	serialization := signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
	return serialization, signature
}

func TestNew(t *testing.T) {
	t.Parallel()

	headerJson := `{"alg":"HS256"}`
	payloadJson := `{"sub":"1234567890"}`
	serialization, signature := signedJws(t, []byte("secret"), headerJson, payloadJson)

	testCases := []struct {
		name          string
		serialization string
		wantErr       bool
	}{
		{
			name:          "valid",
			serialization: serialization,
		},
		{
			name:          "too few parts",
			serialization: "header.payload",
			wantErr:       true,
		},
		{
			name:          "invalid base64 signature",
			serialization: base64.RawURLEncoding.EncodeToString([]byte(headerJson)) + "." + base64.RawURLEncoding.EncodeToString([]byte(payloadJson)) + ".!!!not-base64!!!",
			wantErr:       true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			object, err := New(testCase.serialization)

			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if !errors.Is(err, motmedelErrors.ErrParseError) {
					t.Fatalf("errors.Is(ErrParseError) = false, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if object == nil {
				t.Fatal("expected non-nil object")
			}
			if string(object.Header) != headerJson {
				t.Fatalf("header = %q, want %q", object.Header, headerJson)
			}
			if string(object.Payload) != payloadJson {
				t.Fatalf("payload = %q, want %q", object.Payload, payloadJson)
			}
			if !bytes.Equal(object.Signature, signature) {
				t.Fatalf("signature = %x, want %x", object.Signature, signature)
			}
			if object.Raw != testCase.serialization {
				t.Fatalf("raw = %q, want %q", object.Raw, testCase.serialization)
			}
		})
	}
}

func TestObject_Verify_Valid(t *testing.T) {
	t.Parallel()

	secret := []byte("top-secret")
	serialization, _ := signedJws(t, secret, `{"alg":"HS256"}`, `{"sub":"abc"}`)

	object, err := New(serialization)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	verifier, err := motmedelCryptoHmac.New("HS256", secret)
	if err != nil {
		t.Fatalf("hmac new: %v", err)
	}

	if err := object.Verify(verifier); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestObject_Verify_WrongSecret(t *testing.T) {
	t.Parallel()

	serialization, _ := signedJws(t, []byte("right"), `{"alg":"HS256"}`, `{"sub":"abc"}`)

	object, err := New(serialization)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	verifier, err := motmedelCryptoHmac.New("HS256", []byte("wrong"))
	if err != nil {
		t.Fatalf("hmac new: %v", err)
	}

	err = object.Verify(verifier)
	if err == nil {
		t.Fatal("expected verification error but got nil")
	}
	if !errors.Is(err, motmedelErrors.ErrVerificationError) {
		t.Fatalf("errors.Is(ErrVerificationError) = false, got %v", err)
	}
}

func TestObject_Verify_TamperedPayload(t *testing.T) {
	t.Parallel()

	secret := []byte("secret")
	serialization, _ := signedJws(t, secret, `{"alg":"HS256"}`, `{"sub":"abc"}`)

	object, err := New(serialization)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// Swap the payload part in Raw so the signing input no longer matches.
	parts := strings.Split(object.Raw, ".")
	parts[1] = base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"evil"}`))
	object.Raw = strings.Join(parts, ".")

	verifier, err := motmedelCryptoHmac.New("HS256", secret)
	if err != nil {
		t.Fatalf("hmac new: %v", err)
	}

	if err := object.Verify(verifier); !errors.Is(err, motmedelErrors.ErrVerificationError) {
		t.Fatalf("errors.Is(ErrVerificationError) = false, got %v", err)
	}
}

func TestObject_Verify_BadRaw(t *testing.T) {
	t.Parallel()

	verifier, err := motmedelCryptoHmac.New("HS256", []byte("secret"))
	if err != nil {
		t.Fatalf("hmac new: %v", err)
	}

	object := &Object{Raw: "only.two"}
	err = object.Verify(verifier)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, motmedelErrors.ErrBadSplit) {
		t.Fatalf("errors.Is(ErrBadSplit) = false, got %v", err)
	}
}
