package cdp

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // G505: RFC 6455 defines the handshake with SHA-1; it is not used for security.
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// A WebSocket client of the size the DevTools endpoint needs: text messages
// both ways, fragmented or not, pings answered, and nothing else -- no
// extensions, no subprotocols, no TLS, since the endpoint is Chrome's own on
// loopback. The standard library has no client, and this is less than a
// dependency would bring.

const (
	websocketGuid = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xa

	// maxMessageBytes bounds one message: a whole page's DOM is the largest
	// thing asked for, and it is far smaller.
	maxMessageBytes = 64 * 1024 * 1024
)

var (
	ErrHandshake       = errors.New("websocket handshake failed")
	ErrProtocol        = errors.New("websocket protocol error")
	ErrMessageTooLarge = errors.New("websocket message too large")
)

// WebSocket is one client connection.
type WebSocket struct {
	conn   net.Conn
	reader *bufio.Reader

	writeMutex sync.Mutex
}

// DialWebSocket opens a WebSocket to a ws:// URL.
func DialWebSocket(ctx context.Context, rawUrl string) (*WebSocket, error) {
	socketUrl, err := url.Parse(rawUrl)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("url parse: %w", err), rawUrl)
	}
	if socketUrl.Scheme != "ws" {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: unsupported scheme %q", ErrHandshake, socketUrl.Scheme), rawUrl)
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", socketUrl.Host)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("net dialer dial context: %w", err), socketUrl.Host)
	}

	// The handshake is bounded by the context; the connection after it is not.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	socket, err := handshake(conn, socketUrl)
	if !stop() {
		_ = conn.Close()
		return nil, fmt.Errorf("handshake: %w", ctx.Err())
	}
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("handshake: %w", err)
	}

	return socket, nil
}

func handshake(conn net.Conn, socketUrl *url.URL) (*WebSocket, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("rand read: %w", err))
	}
	key := base64.StdEncoding.EncodeToString(nonce)

	// Only the path and query go on the request line; ws:// is not a scheme
	// Write knows, and the request is the plain HTTP it is anyway.
	request := &http.Request{
		Method:     http.MethodGet,
		URL:        &url.URL{Path: socketUrl.Path, RawQuery: socketUrl.RawQuery},
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Host:       socketUrl.Host,
		Header: http.Header{
			"Upgrade":               {"websocket"},
			"Connection":            {"Upgrade"},
			"Sec-WebSocket-Key":     {key},
			"Sec-WebSocket-Version": {"13"},
		},
	}
	if err := request.Write(conn); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("http request write: %w", err))
	}

	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("http read response: %w", err))
	}
	_ = response.Body.Close()

	//nolint:gosec // G401: see the import.
	accept := sha1.Sum([]byte(key + websocketGuid))
	if response.StatusCode != http.StatusSwitchingProtocols ||
		!strings.EqualFold(response.Header.Get("Upgrade"), "websocket") ||
		response.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(accept[:]) {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s", ErrHandshake, response.Status))
	}

	return &WebSocket{conn: conn, reader: reader}, nil
}

// writeFrame sends one masked frame, as a client must.
func (w *WebSocket) writeFrame(opcode byte, payload []byte) error {
	header := make([]byte, 0, 14)
	header = append(header, 0x80|opcode)

	switch length := len(payload); {
	case length < 126:
		header = append(header, 0x80|byte(length))
	case length <= 0xffff:
		header = append(header, 0x80|126)
		header = binary.BigEndian.AppendUint16(header, uint16(length))
	default:
		header = append(header, 0x80|127)
		header = binary.BigEndian.AppendUint64(header, uint64(length))
	}

	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("rand read: %w", err))
	}
	header = append(header, mask...)

	masked := make([]byte, len(payload))
	for index, value := range payload {
		masked[index] = value ^ mask[index%4]
	}

	w.writeMutex.Lock()
	defer w.writeMutex.Unlock()

	if _, err := w.conn.Write(append(header, masked...)); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("conn write: %w", err))
	}
	return nil
}

// WriteMessage sends one text message.
func (w *WebSocket) WriteMessage(data []byte) error {
	return w.writeFrame(opText, data)
}

// readFrame reads one frame, unmasking it if the server masked it.
func (w *WebSocket) readFrame() (bool, byte, []byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(w.reader, header[:]); err != nil {
		return false, 0, nil, err
	}

	final := header[0]&0x80 != 0
	opcode := header[0] & 0x0f
	if header[0]&0x70 != 0 {
		return false, 0, nil, fmt.Errorf("%w: reserved bits set", ErrProtocol)
	}

	length := uint64(header[1] & 0x7f)
	switch length {
	case 126:
		var extended [2]byte
		if _, err := io.ReadFull(w.reader, extended[:]); err != nil {
			return false, 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(extended[:]))
	case 127:
		var extended [8]byte
		if _, err := io.ReadFull(w.reader, extended[:]); err != nil {
			return false, 0, nil, err
		}
		length = binary.BigEndian.Uint64(extended[:])
	}
	if length > maxMessageBytes {
		return false, 0, nil, fmt.Errorf("%w: %d bytes", ErrMessageTooLarge, length)
	}

	var mask [4]byte
	masked := header[1]&0x80 != 0
	if masked {
		if _, err := io.ReadFull(w.reader, mask[:]); err != nil {
			return false, 0, nil, err
		}
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(w.reader, payload); err != nil {
		return false, 0, nil, err
	}
	if masked {
		for index := range payload {
			payload[index] ^= mask[index%4]
		}
	}

	return final, opcode, payload, nil
}

// ReadMessage returns the next text or binary message, answering pings on the
// way. A close from the server ends the connection with io.EOF.
func (w *WebSocket) ReadMessage() ([]byte, error) {
	var message []byte
	inMessage := false

	for {
		final, opcode, payload, err := w.readFrame()
		if err != nil {
			return nil, err
		}

		switch opcode {
		case opPing:
			if err := w.writeFrame(opPong, payload); err != nil {
				return nil, fmt.Errorf("write pong: %w", err)
			}
			continue
		case opPong:
			continue
		case opClose:
			_ = w.writeFrame(opClose, nil)
			return nil, io.EOF
		case opText, opBinary:
			if inMessage {
				return nil, fmt.Errorf("%w: a new message inside a fragmented one", ErrProtocol)
			}
			message = payload
			inMessage = true
		case opContinuation:
			if !inMessage {
				return nil, fmt.Errorf("%w: a continuation outside a message", ErrProtocol)
			}
			if uint64(len(message))+uint64(len(payload)) > maxMessageBytes {
				return nil, fmt.Errorf("%w: over %d bytes", ErrMessageTooLarge, maxMessageBytes)
			}
			message = append(message, payload...)
		default:
			return nil, fmt.Errorf("%w: opcode %d", ErrProtocol, opcode)
		}

		if final {
			return message, nil
		}
	}
}

// Close ends the connection without the closing handshake; the browser it is
// to is about to be closed anyway.
func (w *WebSocket) Close() error {
	return w.conn.Close()
}
