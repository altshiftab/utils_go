package sql

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	context2 "github.com/altshiftab/utils_go/pkg/context"
	"github.com/altshiftab/utils_go/pkg/database/sql/types/tx_caller"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/interfaces/parser"
	"github.com/altshiftab/utils_go/pkg/utils"
)

func WithTx[T any](
	ctx context.Context,
	database *sql.DB,
	txCaller tx_caller.TxCaller[T],
) (T, error) {
	var zero T

	if err := ctx.Err(); err != nil {
		return zero, fmt.Errorf("context err: %w", err)
	}

	if database == nil {
		return zero, motmedelErrors.NewWithTrace(nil_error.New("sql database"))
	}

	if utils.IsNil(txCaller) {
		return zero, motmedelErrors.NewWithTrace(nil_error.New("tx caller"))
	}

	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return zero, motmedelErrors.NewWithTrace(fmt.Errorf("begin transaction: %w", err))
	}
	if transaction == nil {
		return zero, motmedelErrors.NewWithTrace(nil_error.New("tx"))
	}

	out, err := txCaller.Call(ctx, transaction)
	if err != nil {
		var rollbackErr error
		if rollbackErr = transaction.Rollback(); rollbackErr != nil {
			slog.ErrorContext(
				context2.WithError(
					ctx,
					motmedelErrors.NewWithTrace(fmt.Errorf("tx rollback: %w", rollbackErr), transaction),
				),
				"An error occurred when rolling back a transaction.",
			)
		}
		return zero, err
	}

	if err := transaction.Commit(); err != nil {
		return zero, motmedelErrors.NewWithTrace(fmt.Errorf("tx commit: %w", err), transaction)
	}

	return out, nil
}

func QueryReturningById[T any](
	ctx context.Context,
	id string,
	query string,
	database *sql.DB,
	rowParser parser.ParserCtx[T, *sql.Row],
) (T, error) {
	var zero T

	if err := ctx.Err(); err != nil {
		return zero, fmt.Errorf("context err: %w", err)
	}

	if query == "" {
		return zero, motmedelErrors.NewWithTrace(empty_error.New("query"))
	}

	if database == nil {
		return zero, motmedelErrors.NewWithTrace(nil_error.New("sql database"))
	}

	if utils.IsNil(rowParser) {
		return zero, motmedelErrors.NewWithTrace(nil_error.New("parser"))
	}

	if id == "" {
		return zero, nil
	}

	row := database.QueryRowContext(ctx, query, id)
	out, err := rowParser.Parse(ctx, row)
	if err != nil {
		return zero, err
	}

	return out, nil
}
