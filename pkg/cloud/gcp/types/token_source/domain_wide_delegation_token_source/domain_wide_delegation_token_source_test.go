package domain_wide_delegation_token_source

import (
	"context"
	"errors"
	"testing"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token_source"
)

// errNoToken is returned by errTokenSource.Token to short-circuit the OAuth
// transport before any network dial.
var errNoToken = errors.New("no token")

// errTokenSource is a token_source.TokenSource whose Token always fails. Used as
// the signer source so that the OAuth transport short-circuits in RoundTrip
// before any network dial occurs.
type errTokenSource struct{}

func (errTokenSource) Token() (*token.Token, error) {
	return nil, errNoToken
}

func innerSource(t *testing.T, ts token_source.TokenSource) *TokenSource {
	t.Helper()
	reusable, ok := ts.(*token_source.ReusableTokenSource)
	if !ok {
		t.Fatalf("token source type = %T, want *token_source.ReusableTokenSource", ts)
	}
	inner, ok := reusable.TokenSource.(*TokenSource)
	if !ok {
		t.Fatalf("inner token source type = %T, want *TokenSource", reusable.TokenSource)
	}
	return inner
}

func TestNew_Validation(t *testing.T) {
	t.Parallel()

	signer := errTokenSource{}

	t.Run("nil signer source", func(t *testing.T) {
		t.Parallel()

		ts, err := New(context.Background(), nil, "sa@x.iam.gserviceaccount.com", "user@example.com", nil, "")
		if err == nil {
			t.Fatal("expected error for nil signer source")
		}
		if ts != nil {
			t.Errorf("expected nil token source, got %v", ts)
		}
		ne, ok := errors.AsType[*nil_error.Error](err)
		if !ok {
			t.Fatalf("err type = %T (%v), want *nil_error.Error", err, err)
		}
		if ne.Field != "signer source" {
			t.Errorf("Field = %q, want %q", ne.Field, "signer source")
		}
	})

	t.Run("empty service account email", func(t *testing.T) {
		t.Parallel()

		ts, err := New(context.Background(), signer, "", "user@example.com", nil, "")
		if err == nil {
			t.Fatal("expected error for empty service account email")
		}
		if ts != nil {
			t.Errorf("expected nil token source, got %v", ts)
		}
		ee, ok := errors.AsType[*empty_error.Error](err)
		if !ok {
			t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
		}
		if ee.Field != "service account email" {
			t.Errorf("Field = %q, want %q", ee.Field, "service account email")
		}
	})

	t.Run("empty subject", func(t *testing.T) {
		t.Parallel()

		ts, err := New(context.Background(), signer, "sa@x.iam.gserviceaccount.com", "", nil, "")
		if err == nil {
			t.Fatal("expected error for empty subject")
		}
		if ts != nil {
			t.Errorf("expected nil token source, got %v", ts)
		}
		ee, ok := errors.AsType[*empty_error.Error](err)
		if !ok {
			t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
		}
		if ee.Field != "subject" {
			t.Errorf("Field = %q, want %q", ee.Field, "subject")
		}
	})
}

func TestNew_Fields(t *testing.T) {
	t.Parallel()

	signer := errTokenSource{}
	scopes := []string{"https://www.googleapis.com/auth/cloud-platform"}

	t.Run("default token url", func(t *testing.T) {
		t.Parallel()

		ts, err := New(context.Background(), signer, "sa@x.iam.gserviceaccount.com", "user@example.com", scopes, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		inner := innerSource(t, ts)
		if inner.tokenURL != defaultTokenURL {
			t.Errorf("tokenURL = %q, want default %q", inner.tokenURL, defaultTokenURL)
		}
		if inner.saEmail != "sa@x.iam.gserviceaccount.com" {
			t.Errorf("saEmail = %q, want %q", inner.saEmail, "sa@x.iam.gserviceaccount.com")
		}
		if inner.subject != "user@example.com" {
			t.Errorf("subject = %q, want %q", inner.subject, "user@example.com")
		}
		if len(inner.scopes) != len(scopes) {
			t.Errorf("scopes len = %d, want %d", len(inner.scopes), len(scopes))
		}
		if inner.signerSource == nil {
			t.Error("signerSource not set")
		}
		if inner.iamCredentialsClient == nil {
			t.Error("iamCredentialsClient not set")
		}
	})

	t.Run("custom token url", func(t *testing.T) {
		t.Parallel()

		const custom = "https://oauth2.example/token"
		ts, err := New(context.Background(), signer, "sa@x.iam.gserviceaccount.com", "user@example.com", scopes, custom)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		inner := innerSource(t, ts)
		if inner.tokenURL != custom {
			t.Errorf("tokenURL = %q, want %q", inner.tokenURL, custom)
		}
	})
}

func TestToken_CancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ts, err := New(ctx, errTokenSource{}, "sa@x.iam.gserviceaccount.com", "user@example.com", nil, "https://oauth2.example/token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := ts.Token(); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestToken_SignerError(t *testing.T) {
	t.Parallel()

	// The signer source fails inside the OAuth transport's RoundTrip, so the
	// signJwt request errors before any network dial. This exercises claim
	// assembly and the sign call path offline.
	ts, err := New(context.Background(), errTokenSource{}, "sa@x.iam.gserviceaccount.com", "user@example.com", nil, "https://oauth2.example/token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := ts.Token(); err == nil {
		t.Fatal("expected error from failing signer source")
	}
}
