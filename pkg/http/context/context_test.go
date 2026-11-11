package context

import (
	"context"
	"testing"

	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

func TestContextKeys(t *testing.T) {
	t.Parallel()

	if RequestIdContextKey == nil {
		t.Error("RequestIdContextKey is nil")
	}
}

func TestWithHttpContextValue(t *testing.T) {
	t.Parallel()

	httpContext := &altshiftHttpTypes.HttpContext{}
	ctx := WithHttpContextValue(context.Background(), httpContext)

	stored, ok := ctx.Value(HttpContextContextKey).(*altshiftHttpTypes.HttpContext)
	if !ok {
		t.Fatalf("value not stored as *HttpContext")
	}
	if stored != httpContext {
		t.Errorf("stored value = %p, want %p", stored, httpContext)
	}
}

func TestWithHttpContextValueNil(t *testing.T) {
	t.Parallel()

	ctx := WithHttpContextValue(context.Background(), nil)

	stored, ok := ctx.Value(HttpContextContextKey).(*altshiftHttpTypes.HttpContext)
	if !ok {
		t.Fatalf("value not stored as *HttpContext")
	}
	if stored != nil {
		t.Errorf("stored value = %p, want nil", stored)
	}
}

func TestWithHttpContext(t *testing.T) {
	t.Parallel()

	ctx := WithHttpContext(context.Background())

	stored, ok := ctx.Value(HttpContextContextKey).(*altshiftHttpTypes.HttpContext)
	if !ok {
		t.Fatalf("value not stored as *HttpContext")
	}
	if stored == nil {
		t.Errorf("stored value is nil, want non-nil empty HttpContext")
	}
}

func TestRequestIdRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(context.Background(), RequestIdContextKey, "request-id")

	stored, ok := ctx.Value(RequestIdContextKey).(string)
	if !ok {
		t.Fatalf("value not stored as string")
	}
	if stored != "request-id" {
		t.Errorf("stored value = %q, want %q", stored, "request-id")
	}
}

func TestWithBodiesOmitted(t *testing.T) {
	t.Parallel()

	if BodiesOmitted(context.Background()) {
		t.Error("an unmarked context reads as marked")
	}
	if !BodiesOmitted(WithBodiesOmitted(context.Background())) {
		t.Error("a marked context reads as unmarked")
	}
	//nolint:staticcheck // SA1012: a nil context is what the guard is for.
	if BodiesOmitted(nil) {
		t.Error("a nil context reads as marked")
	}
}
