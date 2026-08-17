package tx_authorizer

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqltesting "github.com/altshiftab/utils_go/pkg/database/sql/testing"
)

var (
	_ TxAuthorizer = TxAuthorizerFunction(nil)
	_ TxAuthorizer = New(func(context.Context, string, *sql.Tx) (bool, error) { return false, nil })
)

var errAuthorize = errors.New("authorize failed")

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

func TestTxAuthorizerFunctionAuthorized(t *testing.T) {
	t.Parallel()

	tx := newTx(t)

	type ctxKey struct{}
	ctx := context.WithValue(t.Context(), ctxKey{}, "value")

	ctxOk := false
	var gotId string
	var gotTx *sql.Tx
	fn := TxAuthorizerFunction(func(c context.Context, id string, x *sql.Tx) (bool, error) {
		ctxOk = c.Value(ctxKey{}) == "value"
		gotId = id
		gotTx = x
		return true, nil
	})

	authorized, err := fn.Authorized(ctx, "subject", tx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !authorized {
		t.Fatal("expected authorized to be true")
	}
	if !ctxOk {
		t.Fatal("expected the context to be threaded through unchanged")
	}
	if gotId != "subject" {
		t.Fatalf("expected id %q, got %q", "subject", gotId)
	}
	if gotTx != tx {
		t.Fatal("expected the transaction to be threaded through unchanged")
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		result         bool
		err            error
		wantAuthorized bool
		wantErr        error
	}{
		{name: "authorized", result: true, err: nil, wantAuthorized: true, wantErr: nil},
		{name: "not authorized", result: false, err: nil, wantAuthorized: false, wantErr: nil},
		{name: "error", result: false, err: errAuthorize, wantAuthorized: false, wantErr: errAuthorize},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			authorizer := New(func(context.Context, string, *sql.Tx) (bool, error) {
				return testCase.result, testCase.err
			})
			if authorizer == nil {
				t.Fatal("expected non-nil authorizer")
			}

			got, err := authorizer.Authorized(t.Context(), "subject", nil)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("expected %v, got %v", testCase.wantErr, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != testCase.wantAuthorized {
				t.Fatalf("expected authorized %t, got %t", testCase.wantAuthorized, got)
			}
		})
	}
}
