package service_account_token_source

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json/v2"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/credentials_file"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/token_response"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func mustRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa generate key: %v", err)
	}
	return key
}

func pkcs1PEM(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	der := x509.MarshalPKCS1PrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}))
}

func pkcs8PEM(t *testing.T, key crypto.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestParsePrivateKey(t *testing.T) {
	t.Parallel()

	rsaKey := mustRSAKey(t)

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa generate key: %v", err)
	}

	unsupportedPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{0x01, 0x02}}))
	badPKCS1PEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte{0x01, 0x02, 0x03}}))

	testCases := []struct {
		name    string
		pemData string
		wantErr bool
	}{
		{name: "pkcs1", pemData: pkcs1PEM(t, rsaKey)},
		{name: "pkcs8 rsa", pemData: pkcs8PEM(t, rsaKey)},
		{name: "no pem block", pemData: "not a pem block", wantErr: true},
		{name: "unsupported block type", pemData: unsupportedPEM, wantErr: true},
		{name: "pkcs8 non-rsa", pemData: pkcs8PEM(t, ecKey), wantErr: true},
		{name: "bad pkcs1 bytes", pemData: badPKCS1PEM, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			key, err := parsePrivateKey(testCase.pemData)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if key != nil {
					t.Errorf("expected nil key on error, got %v", key)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if key == nil {
				t.Fatal("expected non-nil key")
			}
			if key.N.Cmp(rsaKey.N) != 0 {
				t.Error("parsed key modulus does not match original")
			}
		})
	}
}

func TestParsePrivateKey_UnsupportedType(t *testing.T) {
	t.Parallel()

	unsupportedPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{0x01, 0x02}}))
	_, err := parsePrivateKey(unsupportedPEM)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, altshiftErrors.ErrUnexpectedType) {
		t.Errorf("expected ErrUnexpectedType, got %v", err)
	}
}

func testCredentialsFile(t *testing.T, key *rsa.PrivateKey) *credentials_file.File {
	t.Helper()
	return &credentials_file.File{
		ClientEmail:  "robot@x.iam.gserviceaccount.com",
		PrivateKeyID: "key-id-1",
		PrivateKey:   pkcs8PEM(t, key),
	}
}

func TestNewFromCredentialsFileWithSubject(t *testing.T) {
	t.Parallel()

	key := mustRSAKey(t)

	t.Run("empty token url", func(t *testing.T) {
		t.Parallel()

		ts, err := NewFromCredentialsFileWithSubject(context.Background(), "", testCredentialsFile(t, key), nil, "")
		if err == nil {
			t.Fatal("expected error for empty token url")
		}
		if ts != nil {
			t.Errorf("expected nil token source, got %v", ts)
		}
		if !strings.Contains(err.Error(), "token url") {
			t.Errorf("error = %q, want it to mention %q", err.Error(), "token url")
		}
	})

	t.Run("nil credentials file", func(t *testing.T) {
		t.Parallel()

		ts, err := NewFromCredentialsFileWithSubject(context.Background(), "https://oauth2.example/token", nil, nil, "")
		if err == nil {
			t.Fatal("expected error for nil credentials file")
		}
		if ts != nil {
			t.Errorf("expected nil token source, got %v", ts)
		}
		ne, ok := errors.AsType[*nil_error.Error](err)
		if !ok {
			t.Fatalf("err type = %T (%v), want *nil_error.Error", err, err)
		}
		if ne.Field != "credentials file" {
			t.Errorf("Field = %q, want %q", ne.Field, "credentials file")
		}
	})

	t.Run("invalid private key", func(t *testing.T) {
		t.Parallel()

		credFile := &credentials_file.File{
			ClientEmail: "robot@x.iam.gserviceaccount.com",
			PrivateKey:  "garbage",
		}
		ts, err := NewFromCredentialsFileWithSubject(context.Background(), "https://oauth2.example/token", credFile, nil, "")
		if err == nil {
			t.Fatal("expected error for invalid private key")
		}
		if ts != nil {
			t.Errorf("expected nil token source, got %v", ts)
		}
	})

	t.Run("fields set", func(t *testing.T) {
		t.Parallel()

		credFile := testCredentialsFile(t, key)
		scopes := []string{"https://www.googleapis.com/auth/cloud-platform"}
		options := []fetch_config.Option{fetch_config.WithMethod(http.MethodPost)}

		ts, err := NewFromCredentialsFileWithSubject(
			context.Background(), "https://oauth2.example/token", credFile, scopes, "user@example.com", options...,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ts.clientEmail != credFile.ClientEmail {
			t.Errorf("clientEmail = %q, want %q", ts.clientEmail, credFile.ClientEmail)
		}
		if ts.privateKeyID != credFile.PrivateKeyID {
			t.Errorf("privateKeyID = %q, want %q", ts.privateKeyID, credFile.PrivateKeyID)
		}
		if ts.tokenURI != "https://oauth2.example/token" {
			t.Errorf("tokenURI = %q, want %q", ts.tokenURI, "https://oauth2.example/token")
		}
		if ts.subject != "user@example.com" {
			t.Errorf("subject = %q, want %q", ts.subject, "user@example.com")
		}
		if len(ts.scopes) != len(scopes) {
			t.Errorf("scopes len = %d, want %d", len(ts.scopes), len(scopes))
		}
		if len(ts.options) != len(options) {
			t.Errorf("options len = %d, want %d", len(ts.options), len(options))
		}
		if ts.CredentialsFile() != credFile {
			t.Error("CredentialsFile() did not return the provided file")
		}
		if ts.ClientEmail() != credFile.ClientEmail {
			t.Errorf("ClientEmail() = %q, want %q", ts.ClientEmail(), credFile.ClientEmail)
		}
		if ts.Signer() != ts.privateKey {
			t.Error("Signer() did not return the parsed private key")
		}
	})
}

func TestNewFromCredentialsFile_NoSubject(t *testing.T) {
	t.Parallel()

	key := mustRSAKey(t)
	ts, err := NewFromCredentialsFile(context.Background(), "https://oauth2.example/token", testCredentialsFile(t, key), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ts.subject != "" {
		t.Errorf("subject = %q, want empty", ts.subject)
	}
}

func TestToken(t *testing.T) {
	t.Parallel()

	key := mustRSAKey(t)
	credFile := testCredentialsFile(t, key)
	scopes := []string{"https://www.googleapis.com/auth/cloud-platform", "https://www.googleapis.com/auth/devstorage.read_only"}

	testCases := []struct {
		name    string
		subject string
	}{
		{name: "no subject", subject: ""},
		{name: "with subject", subject: "user@example.com"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var serverURL string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method = %s, want POST", r.Method)
				}
				if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
					t.Errorf("Content-Type = %q, want %q", got, "application/x-www-form-urlencoded")
				}
				if err := r.ParseForm(); err != nil {
					t.Errorf("parse form: %v", err)
					return
				}
				if got := r.PostForm.Get("grant_type"); got != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
					t.Errorf("grant_type = %q, want jwt-bearer", got)
				}

				assertion := r.PostForm.Get("assertion")
				parts := strings.Split(assertion, ".")
				if len(parts) != 3 {
					t.Errorf("assertion has %d parts, want 3", len(parts))
					return
				}

				headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
				if err != nil {
					t.Errorf("decode header: %v", err)
					return
				}
				var header map[string]string
				if err := json.Unmarshal(headerBytes, &header); err != nil {
					t.Errorf("unmarshal header: %v", err)
					return
				}
				if header["alg"] != "RS256" {
					t.Errorf("alg = %q, want RS256", header["alg"])
				}
				if header["typ"] != "JWT" {
					t.Errorf("typ = %q, want JWT", header["typ"])
				}
				if header["kid"] != credFile.PrivateKeyID {
					t.Errorf("kid = %q, want %q", header["kid"], credFile.PrivateKeyID)
				}

				claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
				if err != nil {
					t.Errorf("decode claims: %v", err)
					return
				}
				var claims map[string]any
				if err := json.Unmarshal(claimsBytes, &claims); err != nil {
					t.Errorf("unmarshal claims: %v", err)
					return
				}
				if claims["iss"] != credFile.ClientEmail {
					t.Errorf("iss = %v, want %q", claims["iss"], credFile.ClientEmail)
				}
				if claims["aud"] != serverURL {
					t.Errorf("aud = %v, want %q", claims["aud"], serverURL)
				}
				if claims["scope"] != strings.Join(scopes, " ") {
					t.Errorf("scope = %v, want %q", claims["scope"], strings.Join(scopes, " "))
				}
				if testCase.subject == "" {
					if _, ok := claims["sub"]; ok {
						t.Errorf("sub present but no subject configured")
					}
				} else if claims["sub"] != testCase.subject {
					t.Errorf("sub = %v, want %q", claims["sub"], testCase.subject)
				}

				sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
				if err != nil {
					t.Errorf("decode signature: %v", err)
					return
				}
				digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
				if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sigBytes); err != nil {
					t.Errorf("signature verification failed: %v", err)
				}

				w.Header().Set("Content-Type", "application/json")
				if err := json.MarshalWrite(w, &token_response.Response{ //nolint:gosec // G117: fake OAuth token in test fixture
					AccessToken: "access-token",
					TokenType:   "Bearer",
					ExpiresIn:   3600,
				}); err != nil {
					t.Errorf("encode: %v", err)
				}
			}))
			t.Cleanup(server.Close)
			serverURL = server.URL

			ts, err := NewFromCredentialsFileWithSubject(context.Background(), server.URL, credFile, scopes, testCase.subject)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			tok, err := ts.Token()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tok.AccessToken != "access-token" {
				t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "access-token")
			}
			if tok.Type() != "Bearer" {
				t.Errorf("Type() = %q, want %q", tok.Type(), "Bearer")
			}
		})
	}
}

func TestToken_CancelledContext(t *testing.T) {
	t.Parallel()

	key := mustRSAKey(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ts, err := NewFromCredentialsFile(ctx, "https://oauth2.example/token", testCredentialsFile(t, key), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := ts.Token(); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
