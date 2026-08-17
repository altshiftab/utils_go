package tx_caller

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqltesting "github.com/altshiftab/utils_go/pkg/database/sql/testing"
)

var (
	_ TxCaller[int] = TxCallerFunction[int](nil)
	_ TxCaller[int] = New(func(context.Context, *sql.Tx) (int, error) { return 0, nil })
)

var errCall = errors.New("call failed")

func newTx(t *testing.T) *sql.Tx {
	t.Helper()

	db := sqltesting.NewDb()
	if db == nil {
		t.Fatal("expected non-nil db")
	}
	t.Cleanup(func() { _ = db.Close() })

	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if tx == nil {
		t.Fatal("expected non-nil tx")
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	return tx
}

func TestTxCallerFunctionCall(t *testing.T) {
	t.Parallel()

	tx := newTx(t)

	type ctxKey struct{}
	ctx := context.WithValue(t.Context(), ctxKey{}, "value")

	ctxOk := false
	var gotTx *sql.Tx
	fn := TxCallerFunction[string](func(c context.Context, x *sql.Tx) (string, error) {
		ctxOk = c.Value(ctxKey{}) == "value"
		gotTx = x
		return "result", nil
	})

	got, err := fn.Call(ctx, tx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "result" {
		t.Fatalf("expected %q, got %q", "result", got)
	}
	if !ctxOk {
		t.Fatal("expected the context to be threaded through unchanged")
	}
	if gotTx != tx {
		t.Fatal("expected the transaction to be threaded through unchanged")
	}
}

func TestTxCallerFunctionCallError(t *testing.T) {
	t.Parallel()

	fn := TxCallerFunction[int](func(context.Context, *sql.Tx) (int, error) {
		return 0, errCall
	})

	got, err := fn.Call(t.Context(), nil)
	if !errors.Is(err, errCall) {
		t.Fatalf("expected errCall, got %v", err)
	}
	if got != 0 {
		t.Fatalf("expected zero value, got %d", got)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	caller := New(func(context.Context, *sql.Tx) (int, error) { return 42, nil })
	if caller == nil {
		t.Fatal("expected non-nil caller")
	}

	got, err := caller.Call(t.Context(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}
