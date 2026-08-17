package cose

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"math/big"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cbor"
)

// The COSE_Key embedded in the attested credential data of a real WebAuthn registration, and the
// same key as PKIX DER from the client's getPublicKey().
const (
	webauthnCoseKeyBase64 = "pQECAyYgASFYIAGXagpJU2jomFD5gH8KIjogs1e9T3U6DtBuy0viSliTIlggvEQ6WY7jMZuHcA7QX9WG3dYzK6syuC0oPvD-7VQeEeE"
	webauthnPkixDerBase64 = "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEAZdqCklTaOiYUPmAfwoiOiCzV71PdToO0G7LS-JKWJO8RDpZjuMxm4dwDtBf1Ybd1jMrqzK4LSg-8P7tVB4R4Q"
)

func TestParsePublicKeyWebauthnFixture(t *testing.T) {
	t.Parallel()

	coseKeyData, err := base64.RawURLEncoding.DecodeString(webauthnCoseKeyBase64)
	if err != nil {
		t.Fatalf("base64 decode cose key: %v", err)
	}

	expectedDer, err := base64.RawURLEncoding.DecodeString(webauthnPkixDerBase64)
	if err != nil {
		t.Fatalf("base64 decode pkix der: %v", err)
	}

	publicKey, err := ParsePublicKey(coseKeyData)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}

	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatalf("marshal pkix public key: %v", err)
	}

	if !bytes.Equal(der, expectedDer) {
		t.Errorf("pkix der mismatch: %x != %x", der, expectedDer)
	}

	value, err := cbor.Decode(coseKeyData)
	if err != nil {
		t.Fatalf("cbor decode: %v", err)
	}
	keyMap, ok := value.(map[any]any)
	if !ok {
		t.Fatalf("cose key is not a map: %T", value)
	}

	if algorithm, ok := KeyAlgorithm(keyMap); !ok || algorithm != Algorithm(-7) {
		t.Errorf("key algorithm: got %d (present: %t), want -7", algorithm, ok)
	}
}

func makeEc2KeyMap(t *testing.T, publicKey *ecdsa.PublicKey) map[any]any {
	t.Helper()

	ecdhKey, err := publicKey.ECDH()
	if err != nil {
		t.Fatalf("ecdh: %v", err)
	}

	// Uncompressed point: 0x04 || X || Y
	raw := ecdhKey.Bytes()
	coordinateSize := (len(raw) - 1) / 2
	return map[any]any{
		keyParameterKty: keyTypeEc2,
		keyParameterCrv: curveIdFromElliptic(t, publicKey.Curve),
		keyParameterX:   raw[1 : 1+coordinateSize],
		keyParameterY:   raw[1+coordinateSize:],
	}
}

func curveIdFromElliptic(t *testing.T, curve elliptic.Curve) int64 {
	t.Helper()

	for id, registeredCurve := range ellipticCurveRegistry {
		if registeredCurve == curve {
			return id
		}
	}

	t.Fatalf("unregistered curve: %v", curve)
	return 0
}

func TestPublicKeyRoundTrips(t *testing.T) {
	t.Parallel()

	t.Run("ec2", func(t *testing.T) {
		t.Parallel()

		for _, curve := range []elliptic.Curve{elliptic.P256(), elliptic.P384(), elliptic.P521()} {
			privateKey, err := ecdsa.GenerateKey(curve, rand.Reader)
			if err != nil {
				t.Fatalf("ecdsa generate key: %v", err)
			}

			convertedKey, err := PublicKey(makeEc2KeyMap(t, &privateKey.PublicKey))
			if err != nil {
				t.Fatalf("public key (%s): %v", curve.Params().Name, err)
			}

			if !privateKey.PublicKey.Equal(convertedKey) {
				t.Errorf("key mismatch (%s)", curve.Params().Name)
			}
		}
	})

	t.Run("okp ed25519", func(t *testing.T) {
		t.Parallel()

		publicKey, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("ed25519 generate key: %v", err)
		}

		convertedKey, err := PublicKey(map[any]any{
			keyParameterKty: keyTypeOkp,
			keyParameterCrv: CurveEd25519,
			keyParameterX:   []byte(publicKey),
		})
		if err != nil {
			t.Fatalf("public key: %v", err)
		}

		if !publicKey.Equal(convertedKey) {
			t.Errorf("key mismatch")
		}
	})

	t.Run("rsa", func(t *testing.T) {
		t.Parallel()

		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("rsa generate key: %v", err)
		}

		publicKey := &privateKey.PublicKey
		convertedKey, err := PublicKey(map[any]any{
			keyParameterKty:  keyTypeRsa,
			keyParameterRsaN: publicKey.N.Bytes(),
			keyParameterRsaE: big.NewInt(int64(publicKey.E)).Bytes(),
		})
		if err != nil {
			t.Fatalf("public key: %v", err)
		}

		if !publicKey.Equal(convertedKey) {
			t.Errorf("key mismatch")
		}
	})
}

func TestPublicKeyRejects(t *testing.T) {
	t.Parallel()

	validX := make([]byte, 32)
	validX[31] = 1

	testCases := []struct {
		name        string
		keyMap      map[any]any
		expectedErr error
	}{
		{
			name:        "missing kty",
			keyMap:      map[any]any{},
			expectedErr: ErrMalformedKey,
		},
		{
			name:        "unsupported kty",
			keyMap:      map[any]any{keyParameterKty: int64(4)},
			expectedErr: ErrUnsupportedAlgorithm,
		},
		{
			name:        "ec2 missing crv",
			keyMap:      map[any]any{keyParameterKty: keyTypeEc2},
			expectedErr: ErrMalformedKey,
		},
		{
			name: "ec2 unsupported curve",
			keyMap: map[any]any{
				keyParameterKty: keyTypeEc2,
				keyParameterCrv: int64(99),
			},
			expectedErr: ErrUnsupportedAlgorithm,
		},
		{
			name: "ec2 missing coordinate",
			keyMap: map[any]any{
				keyParameterKty: keyTypeEc2,
				keyParameterCrv: CurveP256,
				keyParameterX:   validX,
			},
			expectedErr: ErrMalformedKey,
		},
		{
			name: "ec2 oversized coordinate",
			keyMap: map[any]any{
				keyParameterKty: keyTypeEc2,
				keyParameterCrv: CurveP256,
				keyParameterX:   make([]byte, 33),
				keyParameterY:   validX,
			},
			expectedErr: ErrMalformedKey,
		},
		{
			name: "ec2 point not on curve",
			keyMap: map[any]any{
				keyParameterKty: keyTypeEc2,
				keyParameterCrv: CurveP256,
				keyParameterX:   validX,
				keyParameterY:   validX,
			},
			expectedErr: ErrMalformedKey,
		},
		{
			name: "okp unsupported curve",
			keyMap: map[any]any{
				keyParameterKty: keyTypeOkp,
				keyParameterCrv: CurveP256,
				keyParameterX:   make([]byte, 32),
			},
			expectedErr: ErrUnsupportedAlgorithm,
		},
		{
			name: "okp wrong key length",
			keyMap: map[any]any{
				keyParameterKty: keyTypeOkp,
				keyParameterCrv: CurveEd25519,
				keyParameterX:   make([]byte, 31),
			},
			expectedErr: ErrMalformedKey,
		},
		{
			name: "rsa missing exponent",
			keyMap: map[any]any{
				keyParameterKty:  keyTypeRsa,
				keyParameterRsaN: validX,
			},
			expectedErr: ErrMalformedKey,
		},
		{
			name: "rsa unsupported exponent",
			keyMap: map[any]any{
				keyParameterKty:  keyTypeRsa,
				keyParameterRsaN: validX,
				keyParameterRsaE: []byte{1},
			},
			expectedErr: ErrMalformedKey,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := PublicKey(testCase.keyMap); !errors.Is(err, testCase.expectedErr) {
				t.Errorf("expected %v, got %v", testCase.expectedErr, err)
			}
		})
	}
}

func TestParsePublicKeyRejectsNonMap(t *testing.T) {
	t.Parallel()

	data, err := cbor.Encode(int64(1))
	if err != nil {
		t.Fatalf("cbor encode: %v", err)
	}

	if _, err := ParsePublicKey(data); !errors.Is(err, ErrMalformedKey) {
		t.Errorf("expected malformed key error, got %v", err)
	}
}
