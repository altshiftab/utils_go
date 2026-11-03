package context

import (
	"context"
	"errors"
	"testing"
)

var errBoom = errors.New("boom")

func TestWithError_StoresAndRetrievesError(t *testing.T) {
	t.Parallel()

	ctx := WithError(context.Background(), errBoom)

	got, ok := ctx.Value(ErrorContextKey).(error)
	if !ok {
		t.Fatal("expected value at ErrorContextKey to be an error")
	}
	if !errors.Is(got, errBoom) {
		t.Fatalf("expected stored error to match original, got %v", got)
	}
}

func TestWithError_NilError(t *testing.T) {
	t.Parallel()

	ctx := WithError(context.Background(), nil)
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}

	val := ctx.Value(ErrorContextKey)
	if val != nil {
		// context.WithValue stores nil typed as error, but Value returns the typed nil.
		if err, ok := val.(error); ok && err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	}
}

func TestWithError_ParentValuesPreserved(t *testing.T) {
	t.Parallel()

	type otherKey struct{}

	parent := context.WithValue(context.Background(), otherKey{}, "parent-value")
	ctx := WithError(parent, errBoom)

	if got, _ := ctx.Value(otherKey{}).(string); got != "parent-value" {
		t.Fatalf("expected parent value preserved, got %q", got)
	}
}
