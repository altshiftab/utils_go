// Package journal_handler adapts slog to the systemd journal, recording each
// record as one journal entry whose priority follows the record's level.
package journal_handler

import (
	"context"
	"fmt"
	"io"
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
	// fields are journal fields added to every entry, such as SYSLOG_IDENTIFIER.
	fields map[string]string
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
		fields:          handler.fields,
		send:            handler.send,
	}
}

func (handler *Handler) WithGroup(name string) slog.Handler {
	return &Handler{
		next:            handler.next.WithGroup(name),
		writeLock:       handler.writeLock,
		currentPriority: handler.currentPriority,
		fields:          handler.fields,
		send:            handler.send,
	}
}

// Write sends the encoded record to the journal. It is called by the wrapped
// encoder, from inside Handle, so the priority it reads is the one Handle set.
func (handler *Handler) Write(data []byte) (int, error) {
	message := string(data)

	if err := handler.send(message, *handler.currentPriority, handler.fields); err != nil {
		return 0, altshiftErrors.New(fmt.Errorf("journal send: %w", err), message)
	}

	return len(data), nil
}

// New returns a handler recording each slog record as one journal entry, encoded
// by the handler newEncoder builds. The encoder is built around the returned
// handler, because the handler is the io.Writer the encoder writes through: that
// detour is what lets a record's level become the entry's priority.
//
// fields are journal fields added to every entry. SYSLOG_IDENTIFIER is worth
// setting: without it journald falls back to the process name, which the kernel
// truncates to 15 characters, so a service with a longer name cannot be selected
// with "journalctl -t" at all. fields may be nil.
func New(newEncoder func(writer io.Writer) slog.Handler, fields map[string]string) *Handler {
	if newEncoder == nil {
		newEncoder = func(writer io.Writer) slog.Handler { return slog.NewJSONHandler(writer, nil) }
	}

	handler := &Handler{
		writeLock:       &sync.Mutex{},
		currentPriority: new(journal.Priority),
		fields:          fields,
		send:            journal.Send,
	}
	handler.next = newEncoder(handler)

	return handler
}
