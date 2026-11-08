package authenticator

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftJwkErrors "github.com/altshiftab/utils_go/pkg/json/jose/jwk/errors"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key_handler"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key_handler/key_handler_config"
)

func makeTokenString(t *testing.T, header string, payload string) string {
	t.Helper()

	encode := base64.RawURLEncoding.EncodeToString
	return encode([]byte(header)) + "." + encode([]byte(payload)) + "." + encode([]byte("signature"))
}

func makeKeyHandler(t *testing.T, jwkUrlString string, options ...key_handler_config.Option) *key_handler.Handler {
	t.Helper()

	jwkUrl, err := url.Parse(jwkUrlString)
	if err != nil {
		t.Fatalf("url parse: %v", err)
	}

	handler, err := key_handler.New(jwkUrl, options...)
	if err != nil {
		t.Fatalf("key handler new: %v", err)
	}

	return handler
}

func TestAuthenticatorWithKeyHandlerAuthenticateMissingKid(t *testing.T) {
	t.Parallel()

	authenticator, err := NewWithKeyHandler(makeKeyHandler(t, "http://localhost/jwks"))
	if err != nil {
		t.Fatalf("new with key handler: %v", err)
	}

	tokenString := makeTokenString(t, `{"alg":"EdDSA","typ":"JWT"}`, `{"sub":"test"}`)

	_, err = authenticator.Authenticate(context.Background(), tokenString)
	if err == nil {
		t.Fatal("expected an error")
	}

	if !errors.Is(err, altshiftErrors.ErrValidationError) {
		t.Errorf("expected the error to match ErrValidationError: %v", err)
	}

	if !errors.Is(err, altshiftErrors.ErrNotInMap) {
		t.Errorf("expected the error to match ErrNotInMap: %v", err)
	}
}

func TestAuthenticatorWithKeyHandlerAuthenticateUnknownKid(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			responseWriter.Header().Set("Content-Type", "application/json")
			responseWriter.Header().Set("Cache-Control", "max-age=3600")
			_, _ = responseWriter.Write([]byte(`{"keys": []}`))
		}),
	)
	defer server.Close()

	authenticator, err := NewWithKeyHandler(makeKeyHandler(t, server.URL, key_handler_config.WithFetchOptions(fetch_config.WithHttpClient(server.Client()))))
	if err != nil {
		t.Fatalf("new with key handler: %v", err)
	}

	tokenString := makeTokenString(t, `{"alg":"HS256","kid":"x","typ":"JWT"}`, `{"sub":"test"}`)

	_, err = authenticator.Authenticate(context.Background(), tokenString)
	if err == nil {
		t.Fatal("expected an error")
	}

	if !errors.Is(err, altshiftErrors.ErrVerificationError) {
		t.Errorf("expected the error to match ErrVerificationError: %v", err)
	}

	if !errors.Is(err, altshiftJwkErrors.ErrUnknownKeyId) {
		t.Errorf("expected the error to match ErrUnknownKeyId: %v", err)
	}
}
