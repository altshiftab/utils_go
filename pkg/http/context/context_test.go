package context

import (
	"context"
	"testing"

	motmedelHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

func TestContextKeys(t *testing.T) {
	t.Parallel()

	if RequestIdContextKey == nil {
		t.Error("RequestIdContextKey is nil")
	}
}

func TestWithHttpContextValue(t *testing.T) {
	t.Parallel()

	httpContext := &motmedelHttpTypes.HttpContext{}
	ctx := WithHttpContextValue(context.Background(), httpContext)

	stored, ok := ctx.Value(HttpContextContextKey).(*motmedelHttpTypes.HttpContext)
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

	stored, ok := ctx.Value(HttpContextContextKey).(*motmedelHttpTypes.HttpContext)
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

	stored, ok := ctx.Value(HttpContextContextKey).(*motmedelHttpTypes.HttpContext)
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
