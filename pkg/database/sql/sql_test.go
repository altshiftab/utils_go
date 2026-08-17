package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	sqltesting "github.com/altshiftab/utils_go/pkg/database/sql/testing"
	"github.com/altshiftab/utils_go/pkg/database/sql/types/authorized_tx_caller"
	"github.com/altshiftab/utils_go/pkg/database/sql/types/tx_authorizer"
	"github.com/altshiftab/utils_go/pkg/database/sql/types/tx_caller"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/interfaces/parser"
)

var (
	errCaller = errors.New("caller failed")
	errBegin  = errors.New("begin failed")
	errCommit = errors.New("commit failed")
	errParse  = errors.New("parse failed")
)

// recHooks records commit and rollback activity of a recording driver and
// allows injecting begin and commit failures.
type recHooks struct {
	mu        sync.Mutex
	beginErr  error
	commitErr error
	commits   int
	rollbacks int
}

func (h *recHooks) recordCommit() {
	h.mu.Lock()
	h.commits++
	h.mu.Unlock()
}

func (h *recHooks) recordRollback() {
	h.mu.Lock()
	h.rollbacks++
	h.mu.Unlock()
}

func (h *recHooks) counts() (int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.commits, h.rollbacks
}

type recDriver struct{ hooks *recHooks }

func (d *recDriver) Open(string) (driver.Conn, error) { return &recConn{hooks: d.hooks}, nil }

type recConn struct{ hooks *recHooks }

func (c *recConn) Prepare(string) (driver.Stmt, error) { return &sqltesting.Stmt{}, nil }
func (c *recConn) Close() error                        { return nil }
func (c *recConn) Begin() (driver.Tx, error) {
	if c.hooks.beginErr != nil {
		return nil, c.hooks.beginErr
	}
	return &recTx{hooks: c.hooks}, nil
}

type recTx struct{ hooks *recHooks }

func (x *recTx) Commit() error {
	x.hooks.recordCommit()
	return x.hooks.commitErr
}

func (x *recTx) Rollback() error {
	x.hooks.recordRollback()
	return nil
}

var (
	_ driver.Driver = (*recDriver)(nil)
	_ driver.Conn   = (*recConn)(nil)
	_ driver.Tx     = (*recTx)(nil)
)

var recDriverCounter atomic.Int64

func newRecordingDb(t *testing.T, hooks *recHooks) *sql.DB {
	t.Helper()

	name := fmt.Sprintf("recording-%d", recDriverCounter.Add(1))
	sql.Register(name, &recDriver{hooks: hooks})

	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatalf("open recording db: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil recording db")
	}
	t.Cleanup(func() { _ = db.Close() })

	return db
}

func TestWithTxValidation(t *testing.T) {
	t.Parallel()

	validCaller := tx_caller.New(func(context.Context, *sql.Tx) (string, error) {
		return "value", nil
	})

	testCases := []struct {
		name      string
		canceled  bool
		db        *sql.DB
		caller    tx_caller.TxCaller[string]
		assertErr func(t *testing.T, err error)
	}{
		{
			name:     "canceled context",
			canceled: true,
			db:       sqltesting.NewDb(),
			caller:   validCaller,
			assertErr: func(t *testing.T, err error) {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("expected context.Canceled, got %v", err)
				}
			},
		},
		{
			name:   "nil database",
			db:     nil,
			caller: validCaller,
			assertErr: func(t *testing.T, err error) {
				ne, ok := errors.AsType[*nil_error.Error](err)
				if !ok {
					t.Fatalf("expected *nil_error.Error, got %v", err)
				}
				if ne.Field != "sql database" {
					t.Fatalf("expected field %q, got %q", "sql database", ne.Field)
				}
			},
		},
		{
			name:   "nil caller",
			db:     sqltesting.NewDb(),
			caller: nil,
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
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			if testCase.canceled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			got, err := WithTx(ctx, testCase.db, testCase.caller)
			if err == nil {
				t.Fatalf("expected error, got nil (value %q)", got)
			}
			if got != "" {
				t.Fatalf("expected zero value, got %q", got)
			}
			testCase.assertErr(t, err)
		})
	}
}

func TestWithTxCommitAndRollback(t *testing.T) {
	t.Parallel()

	t.Run("success commits", func(t *testing.T) {
		t.Parallel()

		hooks := &recHooks{}
		db := newRecordingDb(t, hooks)

		receivedTx := false
		caller := tx_caller.New(func(_ context.Context, tx *sql.Tx) (string, error) {
			receivedTx = tx != nil
			return "committed", nil
		})

		got, err := WithTx(t.Context(), db, caller)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "committed" {
			t.Fatalf("expected %q, got %q", "committed", got)
		}
		if !receivedTx {
			t.Fatal("expected the caller to receive a non-nil transaction")
		}
		if commits, rollbacks := hooks.counts(); commits != 1 || rollbacks != 0 {
			t.Fatalf("expected 1 commit and 0 rollbacks, got %d/%d", commits, rollbacks)
		}
	})

	t.Run("caller error rolls back", func(t *testing.T) {
		t.Parallel()

		hooks := &recHooks{}
		db := newRecordingDb(t, hooks)

		caller := tx_caller.New(func(context.Context, *sql.Tx) (string, error) {
			return "", errCaller
		})

		got, err := WithTx(t.Context(), db, caller)
		if !errors.Is(err, errCaller) {
			t.Fatalf("expected errCaller, got %v", err)
		}
		if got != "" {
			t.Fatalf("expected zero value, got %q", got)
		}
		if commits, rollbacks := hooks.counts(); commits != 0 || rollbacks != 1 {
			t.Fatalf("expected 0 commits and 1 rollback, got %d/%d", commits, rollbacks)
		}
	})

	t.Run("begin error", func(t *testing.T) {
		t.Parallel()

		hooks := &recHooks{beginErr: errBegin}
		db := newRecordingDb(t, hooks)

		caller := tx_caller.New(func(context.Context, *sql.Tx) (string, error) {
			return "unused", nil
		})

		_, err := WithTx(t.Context(), db, caller)
		if !errors.Is(err, errBegin) {
			t.Fatalf("expected errBegin, got %v", err)
		}
		if commits, rollbacks := hooks.counts(); commits != 0 || rollbacks != 0 {
			t.Fatalf("expected no commit or rollback on begin error, got %d/%d", commits, rollbacks)
		}
	})

	t.Run("commit error", func(t *testing.T) {
		t.Parallel()

		hooks := &recHooks{commitErr: errCommit}
		db := newRecordingDb(t, hooks)

		caller := tx_caller.New(func(context.Context, *sql.Tx) (string, error) {
			return "value", nil
		})

		_, err := WithTx(t.Context(), db, caller)
		if !errors.Is(err, errCommit) {
			t.Fatalf("expected errCommit, got %v", err)
		}
		if commits, rollbacks := hooks.counts(); commits != 1 || rollbacks != 0 {
			t.Fatalf("expected 1 commit attempt and 0 rollbacks, got %d/%d", commits, rollbacks)
		}
	})
}

func TestWithTxAuthorizer(t *testing.T) {
	t.Parallel()

	const id = "user-123"

	t.Run("authorized commits", func(t *testing.T) {
		t.Parallel()

		hooks := &recHooks{}
		db := newRecordingDb(t, hooks)

		var gotId string
		authorizer := tx_authorizer.New(func(_ context.Context, authId string, _ *sql.Tx) (bool, error) {
			gotId = authId
			return true, nil
		})
		caller := tx_caller.New(func(context.Context, *sql.Tx) (string, error) {
			return "ok", nil
		})

		got, err := WithTx(t.Context(), db, authorized_tx_caller.New(id, caller, authorizer))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "ok" {
			t.Fatalf("expected %q, got %q", "ok", got)
		}
		if gotId != id {
			t.Fatalf("expected authorizer to receive id %q, got %q", id, gotId)
		}
		if commits, rollbacks := hooks.counts(); commits != 1 || rollbacks != 0 {
			t.Fatalf("expected 1 commit and 0 rollbacks, got %d/%d", commits, rollbacks)
		}
	})

	t.Run("unauthorized rolls back", func(t *testing.T) {
		t.Parallel()

		hooks := &recHooks{}
		db := newRecordingDb(t, hooks)

		callerInvoked := false
		authorizer := tx_authorizer.New(func(context.Context, string, *sql.Tx) (bool, error) {
			return false, nil
		})
		caller := tx_caller.New(func(context.Context, *sql.Tx) (string, error) {
			callerInvoked = true
			return "should-not-run", nil
		})

		got, err := WithTx(t.Context(), db, authorized_tx_caller.New(id, caller, authorizer))
		if !errors.Is(err, altshiftErrors.ErrUnauthorized) {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
		if got != "" {
			t.Fatalf("expected zero value, got %q", got)
		}
		if callerInvoked {
			t.Fatal("expected the caller not to run when unauthorized")
		}
		if commits, rollbacks := hooks.counts(); commits != 0 || rollbacks != 1 {
			t.Fatalf("expected 0 commits and 1 rollback, got %d/%d", commits, rollbacks)
		}
	})
}

func TestQueryReturningById(t *testing.T) {
	t.Parallel()

	t.Run("empty id returns zero without calling parser", func(t *testing.T) {
		t.Parallel()

		db := sqltesting.NewDb()
		if db == nil {
			t.Fatal("expected non-nil db")
		}
		t.Cleanup(func() { _ = db.Close() })

		parserInvoked := false
		rowParser := parser.NewCtx(func(context.Context, *sql.Row) (string, error) {
			parserInvoked = true
			return "unexpected", nil
		})

		got, err := QueryReturningById(t.Context(), "", "SELECT 1", db, rowParser)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Fatalf("expected zero value, got %q", got)
		}
		if parserInvoked {
			t.Fatal("expected the parser not to be called for an empty id")
		}
	})

	t.Run("parser result returned", func(t *testing.T) {
		t.Parallel()

		db := sqltesting.NewDb()
		if db == nil {
			t.Fatal("expected non-nil db")
		}
		t.Cleanup(func() { _ = db.Close() })

		receivedRow := false
		rowParser := parser.NewCtx(func(_ context.Context, row *sql.Row) (string, error) {
			receivedRow = row != nil
			return "parsed", nil
		})

		got, err := QueryReturningById(t.Context(), "id-1", "SELECT 1", db, rowParser)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "parsed" {
			t.Fatalf("expected %q, got %q", "parsed", got)
		}
		if !receivedRow {
			t.Fatal("expected the parser to receive a non-nil row")
		}
	})

	t.Run("parser error propagated", func(t *testing.T) {
		t.Parallel()

		db := sqltesting.NewDb()
		if db == nil {
			t.Fatal("expected non-nil db")
		}
		t.Cleanup(func() { _ = db.Close() })

		rowParser := parser.NewCtx(func(context.Context, *sql.Row) (string, error) {
			return "", errParse
		})

		got, err := QueryReturningById(t.Context(), "id-1", "SELECT 1", db, rowParser)
		if !errors.Is(err, errParse) {
			t.Fatalf("expected errParse, got %v", err)
		}
		if got != "" {
			t.Fatalf("expected zero value, got %q", got)
		}
	})

	t.Run("empty result set yields ErrNoRows on scan", func(t *testing.T) {
		t.Parallel()

		db := sqltesting.NewDb()
		if db == nil {
			t.Fatal("expected non-nil db")
		}
		t.Cleanup(func() { _ = db.Close() })

		rowParser := parser.NewCtx(func(_ context.Context, row *sql.Row) (string, error) {
			var value string
			if err := row.Scan(&value); err != nil {
				return "", err
			}
			return value, nil
		})

		_, err := QueryReturningById(t.Context(), "id-1", "SELECT 1", db, rowParser)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expected sql.ErrNoRows, got %v", err)
		}
	})
}

func TestQueryReturningByIdValidation(t *testing.T) {
	t.Parallel()

	validParser := parser.NewCtx(func(context.Context, *sql.Row) (string, error) {
		return "value", nil
	})

	testCases := []struct {
		name      string
		canceled  bool
		id        string
		query     string
		db        *sql.DB
		rowParser parser.ParserCtx[string, *sql.Row]
		assertErr func(t *testing.T, err error)
	}{
		{
			name:      "canceled context",
			canceled:  true,
			id:        "id-1",
			query:     "SELECT 1",
			db:        sqltesting.NewDb(),
			rowParser: validParser,
			assertErr: func(t *testing.T, err error) {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("expected context.Canceled, got %v", err)
				}
			},
		},
		{
			name:      "empty query",
			id:        "id-1",
			query:     "",
			db:        sqltesting.NewDb(),
			rowParser: validParser,
			assertErr: func(t *testing.T, err error) {
				ee, ok := errors.AsType[*empty_error.Error](err)
				if !ok {
					t.Fatalf("expected *empty_error.Error, got %v", err)
				}
				if ee.Field != "query" {
					t.Fatalf("expected field %q, got %q", "query", ee.Field)
				}
			},
		},
		{
			name:      "nil database",
			id:        "id-1",
			query:     "SELECT 1",
			db:        nil,
			rowParser: validParser,
			assertErr: func(t *testing.T, err error) {
				ne, ok := errors.AsType[*nil_error.Error](err)
				if !ok {
					t.Fatalf("expected *nil_error.Error, got %v", err)
				}
				if ne.Field != "sql database" {
					t.Fatalf("expected field %q, got %q", "sql database", ne.Field)
				}
			},
		},
		{
			name:      "nil parser",
			id:        "id-1",
			query:     "SELECT 1",
			db:        sqltesting.NewDb(),
			rowParser: nil,
			assertErr: func(t *testing.T, err error) {
				ne, ok := errors.AsType[*nil_error.Error](err)
				if !ok {
					t.Fatalf("expected *nil_error.Error, got %v", err)
				}
				if ne.Field != "parser" {
					t.Fatalf("expected field %q, got %q", "parser", ne.Field)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			if testCase.canceled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			got, err := QueryReturningById(ctx, testCase.id, testCase.query, testCase.db, testCase.rowParser)
			if err == nil {
				t.Fatalf("expected error, got nil (value %q)", got)
			}
			if got != "" {
				t.Fatalf("expected zero value, got %q", got)
			}
			testCase.assertErr(t, err)
		})
	}
}
