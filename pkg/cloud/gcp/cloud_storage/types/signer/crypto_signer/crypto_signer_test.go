package crypto_signer

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"errors"
	"io"
	"testing"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

// errSignFailed is returned by errSigner.Sign to exercise the error path.
var errSignFailed = errors.New("sign failed")

// errSigner is a crypto.Signer whose Sign always fails, used to exercise the
// error path of Signer.Sign without any real cryptography.
type errSigner struct{}

func (errSigner) Public() crypto.PublicKey { return nil }

func (errSigner) Sign(_ io.Reader, _ []byte, _ crypto.SignerOpts) ([]byte, error) {
	return nil, errSignFailed
}

func mustRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa generate key: %v", err)
	}
	return key
}

func TestNew(t *testing.T) {
	t.Parallel()

	key := mustRSAKey(t)

	testCases := []struct {
		name      string
		signer    crypto.Signer
		email     string
		wantErr   bool
		wantNil   bool // want *nil_error.Error with Field
		wantEmpty bool // want *empty_error.Error with Field
		errField  string
	}{
		{name: "nil signer", signer: nil, email: "robot@x.iam.gserviceaccount.com", wantErr: true, wantNil: true, errField: "signer"},
		{name: "empty email", signer: key, email: "", wantErr: true, wantEmpty: true, errField: "email"},
		{name: "valid", signer: key, email: "robot@x.iam.gserviceaccount.com"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			signer, err := New(testCase.signer, testCase.email)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if signer != nil {
					t.Errorf("expected nil signer on error, got %v", signer)
				}
				if testCase.wantNil {
					ne, ok := errors.AsType[*nil_error.Error](err)
					if !ok {
						t.Fatalf("err type = %T (%v), want *nil_error.Error", err, err)
					}
					if ne.Field != testCase.errField {
						t.Errorf("Field = %q, want %q", ne.Field, testCase.errField)
					}
				}
				if testCase.wantEmpty {
					ee, ok := errors.AsType[*empty_error.Error](err)
					if !ok {
						t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
					}
					if ee.Field != testCase.errField {
						t.Errorf("Field = %q, want %q", ee.Field, testCase.errField)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if signer == nil {
				t.Fatal("expected non-nil signer")
			}
			if signer.email != testCase.email {
				t.Errorf("email field = %q, want %q", signer.email, testCase.email)
			}
			if signer.signer != testCase.signer {
				t.Errorf("signer field = %v, want %v", signer.signer, testCase.signer)
			}
		})
	}
}

func TestEmail(t *testing.T) {
	t.Parallel()

	const email = "robot@x.iam.gserviceaccount.com"
	signer, err := New(mustRSAKey(t), email)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := signer.Email(); got != email {
		t.Errorf("Email() = %q, want %q", got, email)
	}
}

func TestSign(t *testing.T) {
	t.Parallel()

	key := mustRSAKey(t)
	signer, err := New(key, "robot@x.iam.gserviceaccount.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	payload := []byte("string-to-sign")
	signature, err := signer.Sign(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(signature) == 0 {
		t.Fatal("expected non-empty signature")
	}

	// Sign signs the SHA-256 digest of the payload with PKCS#1 v1.5.
	digest := sha256.Sum256(payload)
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
		t.Fatalf("signature verification failed: %v", err)
	}
}

func TestSign_SignerError(t *testing.T) {
	t.Parallel()

	signer, err := New(errSigner{}, "robot@x.iam.gserviceaccount.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := signer.Sign(context.Background(), []byte("payload")); err == nil {
		t.Fatal("expected error from failing signer")
	}
}
