// Package cdp speaks the Chrome DevTools Protocol to a browser's DevTools
// WebSocket endpoint: one JSON object per message, commands answered by id,
// events in between.
//
// The endpoint is the one --remote-debugging-port opens, given a port number.
// The other transport Chrome has, --remote-debugging-pipe, would need no
// WebSocket, but a browser started with it -- or with port 0 -- declares itself
// automated to every page (navigator.webdriver is true), and a challenge page
// reads that first.
package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

var ErrClosed = errors.New("cdp connection closed")

// ProtocolError is an error the browser answered a command with.
type ProtocolError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *ProtocolError) Error() string {
	return fmt.Sprintf("cdp error %d: %s", e.Code, e.Message)
}

// sessionNotFound is the code Chrome answers a command with when the session it
// was sent on is gone: the page closed or crashed.
const sessionNotFound = -32001

// IsTransient reports whether the error is the browser declining a command for
// now -- a document being replaced has no nodes for a moment -- rather than the
// page or the connection being gone, which waiting does not mend.
func IsTransient(err error) bool {
	protocolError, ok := errors.AsType[*ProtocolError](err)
	return ok && protocolError.Code != sessionNotFound
}

// Caller sends a command and decodes its result; *Conn is one, and a test's
// fake browser another.
type Caller interface {
	Call(ctx context.Context, sessionId string, method string, params any, result any) error
}

// Transport carries whole messages; *WebSocket is one.
type Transport interface {
	ReadMessage() ([]byte, error)
	WriteMessage(data []byte) error
	Close() error
}

type message struct {
	Id        int64           `json:"id,omitzero"`
	Method    string          `json:"method,omitzero"`
	SessionId string          `json:"sessionId,omitzero"`
	Params    any             `json:"params,omitzero"`
	Result    json.RawMessage `json:"result,omitzero"`
	Error     *ProtocolError  `json:"error,omitzero"`
}

// Conn is one connection to a browser. Commands may be sent from several
// goroutines; events are read and dropped, since nothing here waits on one.
type Conn struct {
	transport Transport
	nextId    atomic.Int64

	mutex   sync.Mutex
	pending map[int64]chan *message
	err     error
	done    chan struct{}
}

// NewConn starts reading the browser's messages and returns the connection.
// Reading ends, and every pending and later command fails, when the transport
// does.
func NewConn(transport Transport) (*Conn, error) {
	if transport == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("transport"))
	}

	conn := &Conn{
		transport: transport,
		pending:   make(map[int64]chan *message),
		done:      make(chan struct{}),
	}
	go conn.read()

	return conn, nil
}

func (c *Conn) read() {
	for {
		data, err := c.transport.ReadMessage()
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = ErrClosed
			}
			c.fail(err)
			return
		}

		var received message
		if err := json.Unmarshal(data, &received); err != nil {
			c.fail(fmt.Errorf("json unmarshal: %w", err))
			return
		}
		if received.Id == 0 {
			continue
		}

		c.mutex.Lock()
		responseChannel, ok := c.pending[received.Id]
		delete(c.pending, received.Id)
		c.mutex.Unlock()

		if ok {
			responseChannel <- &received
		}
	}
}

// fail ends the connection with the error, once.
func (c *Conn) fail(err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.err != nil {
		return
	}
	c.err = err
	close(c.done)
}

// Call sends a command and decodes its result into result, which may be nil.
// An empty session id addresses the browser itself; a page is addressed
// through the session Target.attachToTarget returned for it.
func (c *Conn) Call(ctx context.Context, sessionId string, method string, params any, result any) error {
	id := c.nextId.Add(1)
	responseChannel := make(chan *message, 1)

	c.mutex.Lock()
	if c.err != nil {
		err := c.err
		c.mutex.Unlock()
		return fmt.Errorf("%s: %w", method, err)
	}
	c.pending[id] = responseChannel
	c.mutex.Unlock()

	data, err := json.Marshal(&message{Id: id, Method: method, SessionId: sessionId, Params: params})
	if err != nil {
		c.forget(id)
		return altshiftErrors.NewWithTrace(fmt.Errorf("json marshal: %w", err), method)
	}

	if err := c.transport.WriteMessage(data); err != nil {
		c.forget(id)
		return fmt.Errorf("%s: write message: %w", method, err)
	}

	select {
	case response := <-responseChannel:
		if response.Error != nil {
			return fmt.Errorf("%s: %w", method, response.Error)
		}
		if result == nil || len(response.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(response.Result, result); err != nil {
			return altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal: %w", err), method)
		}
		return nil
	case <-c.done:
		c.forget(id)
		c.mutex.Lock()
		err := c.err
		c.mutex.Unlock()
		return fmt.Errorf("%s: %w", method, err)
	case <-ctx.Done():
		c.forget(id)
		return fmt.Errorf("%s: %w", method, ctx.Err())
	}
}

func (c *Conn) forget(id int64) {
	c.mutex.Lock()
	delete(c.pending, id)
	c.mutex.Unlock()
}

// Close closes the transport, which ends the connection.
func (c *Conn) Close() error {
	return c.transport.Close()
}

// Done is closed when the connection has ended.
func (c *Conn) Done() <-chan struct{} {
	return c.done
}
