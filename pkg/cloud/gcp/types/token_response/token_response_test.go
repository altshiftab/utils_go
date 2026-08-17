package token_response

import (
	"errors"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

func TestResponseToken(t *testing.T) {
	t.Parallel()

	t.Run("with positive expires_in", func(t *testing.T) {
		t.Parallel()

		before := time.Now()
		response := &Response{AccessToken: "abc", ExpiresIn: 3600, TokenType: "Bearer"}
		tok := response.Token()
		after := time.Now()

		if tok.AccessToken != "abc" {
			t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "abc")
		}
		if tok.TokenType != "Bearer" {
			t.Errorf("TokenType = %q, want %q", tok.TokenType, "Bearer")
		}
		if tok.Expiry.IsZero() {
			t.Fatal("Expiry is zero, want set")
		}

		earliest := before.Add(3600 * time.Second)
		latest := after.Add(3600 * time.Second)
		if tok.Expiry.Before(earliest) || tok.Expiry.After(latest) {
			t.Errorf("Expiry = %v, want within [%v, %v]", tok.Expiry, earliest, latest)
		}
	})

	t.Run("zero expires_in leaves expiry zero", func(t *testing.T) {
		t.Parallel()

		response := &Response{AccessToken: "abc", ExpiresIn: 0}
		tok := response.Token()
		if !tok.Expiry.IsZero() {
			t.Errorf("Expiry = %v, want zero", tok.Expiry)
		}
	})

	t.Run("negative expires_in leaves expiry zero", func(t *testing.T) {
		t.Parallel()

		response := &Response{AccessToken: "abc", ExpiresIn: -1}
		tok := response.Token()
		if !tok.Expiry.IsZero() {
			t.Errorf("Expiry = %v, want zero", tok.Expiry)
		}
	})
}

func TestParse(t *testing.T) {
	t.Parallel()

	t.Run("valid response", func(t *testing.T) {
		t.Parallel()

		response, err := Parse([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"Bearer"}`))
		if err != nil {
			t.Fatalf("Parse: unexpected error: %v", err)
		}
		if response.AccessToken != "tok" {
			t.Errorf("AccessToken = %q, want %q", response.AccessToken, "tok")
		}
		if response.ExpiresIn != 3600 {
			t.Errorf("ExpiresIn = %d, want %d", response.ExpiresIn, 3600)
		}
		if response.TokenType != "Bearer" {
			t.Errorf("TokenType = %q, want %q", response.TokenType, "Bearer")
		}
	})

	t.Run("missing access token yields empty error", func(t *testing.T) {
		t.Parallel()

		response, err := Parse([]byte(`{"expires_in":3600,"token_type":"Bearer"}`))
		if err == nil {
			t.Fatal("Parse: expected error, got nil")
		}
		if response != nil {
			t.Errorf("Parse: expected nil response, got %#v", response)
		}
		emptyErr, ok := errors.AsType[*empty_error.Error](err)
		if !ok {
			t.Fatalf("expected *empty_error.Error, got %T", err)
		}
		if emptyErr.Field != "access token" {
			t.Errorf("empty error Field = %q, want %q", emptyErr.Field, "access token")
		}
	})

	t.Run("invalid json yields error", func(t *testing.T) {
		t.Parallel()

		response, err := Parse([]byte(`{not valid json`))
		if err == nil {
			t.Fatal("Parse: expected error, got nil")
		}
		if response != nil {
			t.Errorf("Parse: expected nil response, got %#v", response)
		}
	})
}
