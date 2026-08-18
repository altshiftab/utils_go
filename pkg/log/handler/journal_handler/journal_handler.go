// Package journal_handler adapts slog to the systemd journal, recording each
// record as one journal entry whose priority follows the record's level.
package journal_handler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/log/journal"
)

var levelToPriority = map[slog.Level]journal.Priority{
	slog.LevelDebug: journal.PriorityDebug,
	slog.LevelInfo:  journal.PriorityInfo,
	slog.LevelWarn:  journal.PriorityWarning,
	slog.LevelError: journal.PriorityError,
}

// Handler sits on both sides of an slog handler: it is the slog.Handler a logger
// writes records to, and the io.Writer the wrapped encoder writes bytes to. The
// detour exists because a record's level has to become a journal priority, which
// is known when the record arrives but not when its encoded bytes come back. The
// level is therefore recorded on the way in and read on the way out, with a lock
// held across the pair so concurrent records cannot take each other's priority.
type Handler struct {
	next            slog.Handler
	writeLock       *sync.Mutex
	currentPriority *journal.Priority
	// send is journal.Send unless a test replaces it.
	send func(message string, priority journal.Priority, variables map[string]string) error
}

func (handler *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.next.Enabled(ctx, level)
}

func (handler *Handler) Handle(ctx context.Context, record slog.Record) error {
	handler.writeLock.Lock()
	defer handler.writeLock.Unlock()

	priority, ok := levelToPriority[record.Level]
	if !ok {
		priority = journal.PriorityInfo
	}
	*handler.currentPriority = priority

	if err := handler.next.Handle(ctx, record); err != nil {
		return altshiftErrors.New(fmt.Errorf("next handle: %w", err))
	}

	return nil
}

func (handler *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{
		next:            handler.next.WithAttrs(attrs),
		writeLock:       handler.writeLock,
		currentPriority: handler.currentPriority,
		send:            handler.send,
	}
}

func (handler *Handler) WithGroup(name string) slog.Handler {
	return &Handler{
		next:            handler.next.WithGroup(name),
		writeLock:       handler.writeLock,
		currentPriority: handler.currentPriority,
		send:            handler.send,
	}
}

// Write sends the encoded record to the journal. It is called by the wrapped
// encoder, from inside Handle, so the priority it reads is the one Handle set.
func (handler *Handler) Write(data []byte) (int, error) {
	message := string(data)

	if err := handler.send(message, *handler.currentPriority, nil); err != nil {
		return 0, altshiftErrors.New(fmt.Errorf("journal send: %w", err), message)
	}

	return len(data), nil
}

// NewJsonHandler returns a handler recording each record as a JSON object in the
// journal. handlerOptions may be nil.
func NewJsonHandler(handlerOptions *slog.HandlerOptions) *Handler {
	handler := &Handler{writeLock: &sync.Mutex{}, currentPriority: new(journal.Priority), send: journal.Send}
	handler.next = slog.NewJSONHandler(handler, handlerOptions)

	return handler
}
