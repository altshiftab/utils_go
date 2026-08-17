package context

import (
	stdContext "context"
	"testing"

	altshiftTlsTypes "github.com/altshiftab/utils_go/pkg/tls/types"
)

func TestWithTlsContextValue(t *testing.T) {
	t.Parallel()

	tlsContext := &altshiftTlsTypes.TlsContext{ClientInitiated: true}
	ctx := WithTlsContextValue(stdContext.Background(), tlsContext)

	got, ok := ctx.Value(TlsContextKey).(*altshiftTlsTypes.TlsContext)
	if !ok {
		t.Fatal("expected value stored under TlsContextKey to be *TlsContext")
	}
	if got != tlsContext {
		t.Fatal("expected the same TlsContext pointer to be returned")
	}
	if !got.ClientInitiated {
		t.Fatal("expected ClientInitiated to be preserved")
	}
}

func TestWithTlsContext(t *testing.T) {
	t.Parallel()

	ctx := WithTlsContext(stdContext.Background())

	got, ok := ctx.Value(TlsContextKey).(*altshiftTlsTypes.TlsContext)
	if !ok {
		t.Fatal("expected value stored under TlsContextKey to be *TlsContext")
	}
	if got == nil {
		t.Fatal("expected a non-nil TlsContext")
	}
}

func TestKeyIsolation(t *testing.T) {
	t.Parallel()

	// A background context has no value stored under the key.
	if v := stdContext.Background().Value(TlsContextKey); v != nil {
		t.Fatalf("expected nil for missing key, got %v", v)
	}
}
