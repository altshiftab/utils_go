package authorized_tx_caller

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqltesting "github.com/altshiftab/utils_go/pkg/database/sql/testing"
	"github.com/altshiftab/utils_go/pkg/database/sql/types/tx_authorizer"
	"github.com/altshiftab/utils_go/pkg/database/sql/types/tx_caller"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

var _ tx_caller.TxCaller[int] = (*AuthorizedTxCaller[int])(nil)

var (
	errCaller     = errors.New("caller failed")
	errAuthorizer = errors.New("authorizer failed")
)

func constCaller(value int) tx_caller.TxCaller[int] {
	return tx_caller.New(func(context.Context, *sql.Tx) (int, error) { return value, nil })
}

func failingCaller(err error) tx_caller.TxCaller[int] {
	return tx_caller.New(func(context.Context, *sql.Tx) (int, error) { return 0, err })
}

func constAuthorizer(authorized bool, err error) tx_authorizer.TxAuthorizer {
	return tx_authorizer.New(func(context.Context, string, *sql.Tx) (bool, error) {
		return authorized, err
	})
}

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

func TestNewConstructor(t *testing.T) {
	t.Parallel()

	caller := constCaller(1)
	authorizer := constAuthorizer(true, nil)

	c := New("id-1", caller, authorizer)
	if c == nil {
		t.Fatal("expected non-nil authorized caller")
	}
	if c.Id != "id-1" {
		t.Fatalf("expected Id %q, got %q", "id-1", c.Id)
	}
	if c.TxCaller == nil {
		t.Fatal("expected TxCaller to be populated")
	}
	if c.TxAuthorizer == nil {
		t.Fatal("expected TxAuthorizer to be populated")
	}
}

func TestCall(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		caller     tx_caller.TxCaller[int]
		authorizer tx_authorizer.TxAuthorizer
		wantValue  int
		assertErr  func(t *testing.T, err error)
	}{
		{
			name:       "no authorizer runs caller",
			caller:     constCaller(7),
			authorizer: nil,
			wantValue:  7,
			assertErr:  nil,
		},
		{
			name:       "authorized runs caller",
			caller:     constCaller(5),
			authorizer: constAuthorizer(true, nil),
			wantValue:  5,
			assertErr:  nil,
		},
		{
			name:       "nil caller",
			caller:     nil,
			authorizer: nil,
			wantValue:  0,
			assertErr: func(t *testing.T, err error) {
				ne, ok := errors.AsType[*nil_error.Error](err)
				if !ok {
					t.Fatalf("expected *nil_error.Error, got %v", err)
				}
				if ne.Field != "tx caller" {
					t.Fatalf("expected field %q, got %q", "tx caller", ne.Field)
				}
			},
		},
		{
			name:       "unauthorized",
			caller:     constCaller(5),
			authorizer: constAuthorizer(false, nil),
			wantValue:  0,
			assertErr: func(t *testing.T, err error) {
				if !errors.Is(err, altshiftErrors.ErrUnauthorized) {
					t.Fatalf("expected ErrUnauthorized, got %v", err)
				}
			},
		},
		{
			name:       "authorizer error",
			caller:     constCaller(5),
			authorizer: constAuthorizer(false, errAuthorizer),
			wantValue:  0,
			assertErr: func(t *testing.T, err error) {
				if !errors.Is(err, errAuthorizer) {
					t.Fatalf("expected errAuthorizer, got %v", err)
				}
			},
		},
		{
			name:       "caller error",
			caller:     failingCaller(errCaller),
			authorizer: constAuthorizer(true, nil),
			wantValue:  0,
			assertErr: func(t *testing.T, err error) {
				if !errors.Is(err, errCaller) {
					t.Fatalf("expected errCaller, got %v", err)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			c := New("subject", testCase.caller, testCase.authorizer)
			got, err := c.Call(t.Context(), nil)

			if testCase.assertErr != nil {
				if err == nil {
					t.Fatalf("expected error, got nil (value %d)", got)
				}
				testCase.assertErr(t, err)
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != testCase.wantValue {
				t.Fatalf("expected value %d, got %d", testCase.wantValue, got)
			}
		})
	}
}

func TestCallUnauthorizedDoesNotInvokeCaller(t *testing.T) {
	t.Parallel()

	invoked := false
	caller := tx_caller.New(func(context.Context, *sql.Tx) (int, error) {
		invoked = true
		return 1, nil
	})
	authorizer := constAuthorizer(false, nil)

	_, err := New("subject", caller, authorizer).Call(t.Context(), nil)
	if !errors.Is(err, altshiftErrors.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
	if invoked {
		t.Fatal("expected the caller not to run when unauthorized")
	}
}

func TestCallThreadsContextIdAndTx(t *testing.T) {
	t.Parallel()

	tx := newTx(t)

	type ctxKey struct{}
	ctx := context.WithValue(t.Context(), ctxKey{}, "value")

	var (
		authCtxOk, authTxOk     bool
		callerCtxOk, callerTxOk bool
		gotId                   string
	)

	authorizer := tx_authorizer.New(func(c context.Context, id string, x *sql.Tx) (bool, error) {
		authCtxOk = c.Value(ctxKey{}) == "value"
		authTxOk = x == tx
		gotId = id
		return true, nil
	})
	caller := tx_caller.New(func(c context.Context, x *sql.Tx) (int, error) {
		callerCtxOk = c.Value(ctxKey{}) == "value"
		callerTxOk = x == tx
		return 9, nil
	})

	got, err := New("subject-9", caller, authorizer).Call(ctx, tx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 9 {
		t.Fatalf("expected 9, got %d", got)
	}
	if gotId != "subject-9" {
		t.Fatalf("expected id %q, got %q", "subject-9", gotId)
	}
	if !authCtxOk || !authTxOk {
		t.Fatal("expected the authorizer to receive the same context and transaction")
	}
	if !callerCtxOk || !callerTxOk {
		t.Fatal("expected the caller to receive the same context and transaction")
	}
}
