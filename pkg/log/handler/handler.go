package handler

import (
	"context"
	"github.com/altshiftab/utils_go/pkg/log/handler/tree"
	"log/slog"
)

type Handler struct {
	Next  slog.Handler
	stack []string
	root  *tree.Group
}

func (handler *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.Next.Enabled(ctx, level)
}

func (handler *Handler) Handle(ctx context.Context, record slog.Record) error {
	newRecord := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)

	if record.NumAttrs() == 0 {
		newRecord.AddAttrs(handler.root.Render()...)
	} else {
		newRoot := handler.root.Clone()
		record.Attrs(
			func(attr slog.Attr) bool {
				newRoot.Merge(handler.stack, attr)
				return true
			},
		)
		newRecord.AddAttrs(newRoot.Render()...)
	}

	return handler.Next.Handle(ctx, newRecord)
}

func (handler *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newRoot := handler.root.Clone()
	newRoot.Merge(handler.stack, attrs...)

	return &Handler{Next: handler.Next, stack: handler.stack, root: newRoot}
}

func (handler *Handler) WithGroup(name string) slog.Handler {
	newRoot := handler.root.Clone()
	newRoot.Merge(handler.stack, slog.Group(name))

	return &Handler{Next: handler.Next, stack: append(handler.stack, name), root: newRoot}
}

func New(next slog.Handler) slog.Handler {
	return &Handler{Next: next, stack: nil, root: new(tree.Group)}
}
