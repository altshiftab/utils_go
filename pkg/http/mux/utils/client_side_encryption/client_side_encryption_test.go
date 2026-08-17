package client_side_encryption

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/utils/client_side_encryption/body_parser_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/utils/client_side_encryption/header_parser_config"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwe"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key"
)

func makeJwe(t *testing.T, recipientPublicKey *ecdsa.PublicKey, keyId string) []byte {
	t.Helper()

	encrypter, err := jwe.NewEncrypter(jwe.KeyAlgorithmEcdhEs, jwe.ContentEncryptionA256Gcm, recipientPublicKey)
	if err != nil {
		t.Fatalf("jwe new encrypter: %v", err)
	}
	encrypter.KeyId = keyId

	serialization, err := encrypter.Encrypt([]byte(`{}`))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	return []byte(serialization)
}

func TestNewBodyParserSetsKeyIdentifier(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	bodyParser, err := NewBodyParser(privateKey, body_parser_config.WithKeyIdentifier("expected-kid"))
	if err != nil {
		t.Fatalf("new body parser: %v", err)
	}

	if bodyParser.KeyIdentifier != "expected-kid" {
		t.Errorf("key identifier = %q, want %q", bodyParser.KeyIdentifier, "expected-kid")
	}
}

func TestBodyParserParseKeyIdentifierMismatch(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	bodyParser, err := NewBodyParser(privateKey, body_parser_config.WithKeyIdentifier("expected-kid"))
	if err != nil {
		t.Fatalf("new body parser: %v", err)
	}

	_, responseError := bodyParser.Parse(nil, makeJwe(t, &privateKey.PublicKey, "other-kid"))
	if responseError == nil {
		t.Fatal("expected a response error")
	}

	problemDetail := responseError.ProblemDetail
	if problemDetail == nil {
		t.Fatal("expected a problem detail")
	}

	if problemDetail.Status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", problemDetail.Status, http.StatusBadRequest)
	}
}

func TestBodyParserParseUnusableKeyIsServerError(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// An int is not a supported key type; `jwe.Decrypt` fails with
	// `jwe.ErrUnsupportedKeyType` rather than a verification error.
	bodyParser, err := NewBodyParser(12345)
	if err != nil {
		t.Fatalf("new body parser: %v", err)
	}

	_, responseError := bodyParser.Parse(nil, makeJwe(t, &privateKey.PublicKey, ""))
	if responseError == nil {
		t.Fatal("expected a response error")
	}

	if responseError.ServerError == nil {
		t.Error("expected a server error")
	}

	if responseError.ClientError != nil {
		t.Errorf("unexpected client error: %v", responseError.ClientError)
	}
}

func TestBodyParserParseDecryptFailureIsClientError(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	otherPrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}

	bodyParser, err := NewBodyParser(privateKey)
	if err != nil {
		t.Fatalf("new body parser: %v", err)
	}

	_, responseError := bodyParser.Parse(nil, makeJwe(t, &otherPrivateKey.PublicKey, ""))
	if responseError == nil {
		t.Fatal("expected a response error")
	}

	if responseError.ServerError != nil {
		t.Errorf("unexpected server error: %v", responseError.ServerError)
	}

	if responseError.ClientError == nil {
		t.Error("expected a client error")
	}

	problemDetail := responseError.ProblemDetail
	if problemDetail == nil {
		t.Fatal("expected a problem detail")
	}

	if problemDetail.Status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", problemDetail.Status, http.StatusBadRequest)
	}
}

func TestBodyParserRoundTrip(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	bodyParser, err := NewBodyParser(privateKey)
	if err != nil {
		t.Fatalf("new body parser: %v", err)
	}

	plaintext, responseError := bodyParser.Parse(nil, makeJwe(t, &privateKey.PublicKey, ""))
	if responseError == nil {
		if string(plaintext) != `{}` {
			t.Errorf("plaintext = %q, want %q", plaintext, `{}`)
		}
	} else {
		t.Fatalf("unexpected response error: %+v", responseError)
	}
}

func makeHeaderRequest(t *testing.T, headerValue string) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
	request.Header.Set(header_parser_config.DefaultHeaderName, headerValue)

	return request
}

func TestHeaderParserParse(t *testing.T) {
	t.Parallel()

	clientPrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	clientJwk, err := key.NewFromPublicKey(&clientPrivateKey.PublicKey, "", "client-key", "")
	if err != nil {
		t.Fatalf("key new from public key: %v", err)
	}

	clientJwkData, err := json.Marshal(clientJwk)
	if err != nil {
		t.Fatalf("json marshal (client jwk): %v", err)
	}

	headerParser, err := NewHeaderParser()
	if err != nil {
		t.Fatalf("new header parser: %v", err)
	}

	encrypter, responseError := headerParser.Parse(makeHeaderRequest(t, string(clientJwkData)))
	if responseError != nil {
		t.Fatalf("unexpected response error: %+v", responseError)
	}
	if encrypter == nil {
		t.Fatal("expected a non-nil encrypter")
	}

	if encrypter.KeyId != "client-key" {
		t.Errorf("encrypter key id = %q, want %q", encrypter.KeyId, "client-key")
	}
	if encrypter.ContentType != header_parser_config.DefaultContentType {
		t.Errorf("encrypter content type = %q, want %q", encrypter.ContentType, header_parser_config.DefaultContentType)
	}

	serialization, err := encrypter.Encrypt([]byte(`{"response":true}`))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	encryption, err := jwe.ParseCompact(
		serialization,
		[]jwe.KeyAlgorithm{jwe.KeyAlgorithmEcdhEs},
		[]jwe.ContentEncryption{jwe.ContentEncryptionA256Gcm},
	)
	if err != nil {
		t.Fatalf("parse compact: %v", err)
	}

	plaintext, err := encryption.Decrypt(clientPrivateKey)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(plaintext) != `{"response":true}` {
		t.Errorf("plaintext = %q, want %q", plaintext, `{"response":true}`)
	}
}

func TestHeaderParserParseFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		headerValue string
	}{
		{
			name:        "invalid json",
			headerValue: "{",
		},
		{
			name:        "null jwk",
			headerValue: "null",
		},
		{
			name:        "unsupported kty",
			headerValue: `{"kty":"oct","k":"AAAA"}`,
		},
		{
			name:        "rsa key with ecdh-es",
			headerValue: `{"kty":"RSA","n":"3Zzo","e":"AQAB","alg":"RS256"}`,
		},
		{
			name:        "malformed ec key",
			headerValue: `{"kty":"EC","crv":"P-256","x":"!","y":"!"}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			headerParser, err := NewHeaderParser()
			if err != nil {
				t.Fatalf("new header parser: %v", err)
			}

			_, responseError := headerParser.Parse(makeHeaderRequest(t, testCase.headerValue))
			if responseError == nil {
				t.Fatal("expected a response error")
			}

			problemDetail := responseError.ProblemDetail
			if problemDetail == nil {
				t.Fatal("expected a problem detail")
			}
			if problemDetail.Status != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", problemDetail.Status, http.StatusBadRequest)
			}
		})
	}
}
