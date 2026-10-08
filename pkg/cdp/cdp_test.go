package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftTestingCmp "github.com/altshiftab/utils_go/pkg/testing/cmp"
)

// fakeTransport answers commands the way Chrome does, an event before every
// response to show events are passed over: "Echo" returns its params with the
// session it was sent on, "Fail" a protocol error, and "Hang" nothing at all.
type fakeTransport struct {
	incoming chan []byte
	closed   chan struct{}
	once     sync.Once
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{incoming: make(chan []byte, 16), closed: make(chan struct{})}
}

func (f *fakeTransport) ReadMessage() ([]byte, error) {
	select {
	case data := <-f.incoming:
		return data, nil
	case <-f.closed:
		return nil, io.EOF
	}
}

func (f *fakeTransport) WriteMessage(data []byte) error {
	var command struct {
		Id        int64           `json:"id"`
		Method    string          `json:"method"`
		SessionId string          `json:"sessionId"`
		Params    json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(data, &command); err != nil {
		return err
	}

	var response any
	switch command.Method {
	case "Echo":
		response = map[string]any{
			"id":     command.Id,
			"result": map[string]any{"params": command.Params, "session": command.SessionId},
		}
	case "Fail":
		response = map[string]any{"id": command.Id, "error": map[string]any{"code": -32000, "message": "no such thing"}}
	case "Hang":
		return nil
	default:
		response = map[string]any{"id": command.Id, "result": map[string]any{}}
	}

	event, _ := json.Marshal(map[string]any{"method": "Page.frameNavigated", "params": map[string]any{}})
	encoded, _ := json.Marshal(response)
	f.incoming <- event
	f.incoming <- encoded
	return nil
}

func (f *fakeTransport) Close() error {
	f.once.Do(func() { close(f.closed) })
	return nil
}

func newTestConn(t *testing.T) (*Conn, *fakeTransport) {
	t.Helper()

	transport := newFakeTransport()
	conn, err := NewConn(transport)
	if err != nil {
		t.Fatalf("new conn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return conn, transport
}

func TestCall(t *testing.T) {
	t.Parallel()

	conn, _ := newTestConn(t)

	type echoed struct {
		Params  map[string]string `json:"params"`
		Session string            `json:"session"`
	}

	testCases := []struct {
		name      string
		method    string
		sessionId string
		params    any
		expected  *echoed
		wantErr   bool
	}{
		{
			name:     "browser command",
			method:   "Echo",
			params:   map[string]string{"url": "https://forums.mvgroup.org/"},
			expected: &echoed{Params: map[string]string{"url": "https://forums.mvgroup.org/"}},
		},
		{
			name:      "session command",
			method:    "Echo",
			sessionId: "session-1",
			params:    map[string]string{"a": "b"},
			expected:  &echoed{Params: map[string]string{"a": "b"}, Session: "session-1"},
		},
		{name: "protocol error", method: "Fail", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := &echoed{}
			err := conn.Call(t.Context(), testCase.sessionId, testCase.method, testCase.params, result)
			if testCase.wantErr {
				if _, ok := errors.AsType[*ProtocolError](err); !ok {
					t.Fatalf("expected a protocol error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			if diff := altshiftTestingCmp.Diff(testCase.expected, result); diff != "" {
				t.Errorf("result mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestCallNilResult(t *testing.T) {
	t.Parallel()

	conn, _ := newTestConn(t)
	if err := conn.Call(t.Context(), "", "Page.enable", nil, nil); err != nil {
		t.Fatalf("call: %v", err)
	}
}

func TestCallCancelled(t *testing.T) {
	t.Parallel()

	conn, _ := newTestConn(t)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	if err := conn.Call(ctx, "", "Hang", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the deadline, got %v", err)
	}

	// The connection outlives a command given up on.
	if err := conn.Call(t.Context(), "", "Page.enable", nil, nil); err != nil {
		t.Fatalf("call after a cancelled one: %v", err)
	}
}

func TestCallClosed(t *testing.T) {
	t.Parallel()

	conn, transport := newTestConn(t)

	hung := make(chan error, 1)
	go func() {
		hung <- conn.Call(t.Context(), "", "Hang", nil, nil)
	}()

	// The browser going away ends what was waiting and refuses what comes after.
	time.Sleep(20 * time.Millisecond)
	_ = transport.Close()

	select {
	case err := <-hung:
		if !errors.Is(err, ErrClosed) {
			t.Errorf("expected the connection closed, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("a pending command was not ended")
	}

	<-conn.Done()
	if err := conn.Call(t.Context(), "", "Page.enable", nil, nil); !errors.Is(err, ErrClosed) {
		t.Errorf("expected the connection closed, got %v", err)
	}
}

func TestNewConn(t *testing.T) {
	t.Parallel()

	_, err := NewConn(nil)
	altshiftTestingCmp.CompareErr(t, err, nil_error.New("transport"))
}

func TestIsTransient(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "no node for now", err: &ProtocolError{Code: -32000, Message: "Could not find node with given id"}, expected: true},
		{name: "wrapped", err: fmt.Errorf("DOM.getDocument: %w", &ProtocolError{Code: -32000, Message: "x"}), expected: true},
		{name: "the session gone", err: &ProtocolError{Code: sessionNotFound, Message: "Session with given id not found."}},
		{name: "the connection gone", err: ErrClosed},
		{name: "nil", err: nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := IsTransient(testCase.err); got != testCase.expected {
				t.Errorf("unexpected: %v", got)
			}
		})
	}
}
