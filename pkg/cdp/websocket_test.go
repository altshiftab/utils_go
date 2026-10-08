package cdp

import (
	"bufio"
	"bytes"
	"crypto/sha1" //nolint:gosec // G505: the RFC 6455 handshake.
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

var errUnmasked = errors.New("an unmasked client frame")

// readClientFrame reads a frame the way a server must: refusing one the
// client did not mask.
func readClientFrame(reader *bufio.Reader) (byte, []byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return 0, nil, err
	}
	if header[1]&0x80 == 0 {
		return 0, nil, errUnmasked
	}

	length := uint64(header[1] & 0x7f)
	switch length {
	case 126:
		var extended [2]byte
		if _, err := io.ReadFull(reader, extended[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(extended[:]))
	case 127:
		var extended [8]byte
		if _, err := io.ReadFull(reader, extended[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(extended[:])
	}

	var mask [4]byte
	if _, err := io.ReadFull(reader, mask[:]); err != nil {
		return 0, nil, err
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return 0, nil, err
	}
	for index := range payload {
		payload[index] ^= mask[index%4]
	}

	return header[0] & 0x0f, payload, nil
}

// serverFrame is an unmasked frame, as a server sends.
func serverFrame(final bool, opcode byte, payload []byte) []byte {
	first := opcode
	if final {
		first |= 0x80
	}

	frame := []byte{first}
	switch length := len(payload); {
	case length < 126:
		frame = append(frame, byte(length))
	case length <= 0xffff:
		frame = append(frame, 126)
		frame = binary.BigEndian.AppendUint16(frame, uint16(length))
	default:
		frame = append(frame, 127)
		frame = binary.BigEndian.AppendUint64(frame, uint64(length))
	}
	return append(frame, payload...)
}

// websocketServer accepts the handshake -- with a wrong accept value when told
// to -- and then runs the script against the connection.
func websocketServer(t *testing.T, wrongAccept bool, script func(reader *bufio.Reader, conn net.Conn)) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" || r.Header.Get("Sec-WebSocket-Version") != "13" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		key := r.Header.Get("Sec-WebSocket-Key")
		if wrongAccept {
			key = "something else"
		}
		//nolint:gosec // G401: the RFC 6455 handshake.
		accept := sha1.Sum([]byte(key + websocketGuid))

		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("not hijackable")
			return
		}
		conn, buffered, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()

		_, _ = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(accept[:]) + "\r\n\r\n")
		_ = buffered.Flush()

		script(buffered.Reader, conn)
	}))
	t.Cleanup(server.Close)

	return server
}

func socketUrl(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/browser/abc"
}

func TestWebSocket(t *testing.T) {
	t.Parallel()

	large := bytes.Repeat([]byte("x"), 70000)

	server := websocketServer(t, false, func(reader *bufio.Reader, conn net.Conn) {
		// Echo the first message, after a ping that must be answered.
		opcode, payload, err := readClientFrame(reader)
		if err != nil || opcode != opText {
			t.Errorf("first message: %d %v", opcode, err)
			return
		}
		_, _ = conn.Write(serverFrame(true, opPing, []byte("are you there")))
		if opcode, pong, err := readClientFrame(reader); err != nil || opcode != opPong || string(pong) != "are you there" {
			t.Errorf("pong: %d %q %v", opcode, pong, err)
			return
		}
		_, _ = conn.Write(serverFrame(true, opText, payload))

		// A message in three fragments, the middle one after a pong to ignore.
		_, _ = conn.Write(serverFrame(false, opText, []byte(`{"id":`)))
		_, _ = conn.Write(serverFrame(true, opPong, nil))
		_, _ = conn.Write(serverFrame(false, opContinuation, []byte(`1,`)))
		_, _ = conn.Write(serverFrame(true, opContinuation, []byte(`"result":{}}`)))

		// One long enough for the 64-bit length, then a long one back.
		_, _ = conn.Write(serverFrame(true, opText, large))
		if opcode, payload, err := readClientFrame(reader); err != nil || opcode != opText || len(payload) != len(large) {
			t.Errorf("large message from the client: %d %d %v", opcode, len(payload), err)
			return
		}

		_, _ = conn.Write(serverFrame(true, opClose, nil))
		_, _, _ = readClientFrame(reader)
	})

	socket, err := DialWebSocket(t.Context(), socketUrl(server))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = socket.Close() }()

	if err := socket.WriteMessage([]byte(`{"id":1,"method":"Browser.getVersion"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}

	steps := []struct {
		name     string
		expected []byte
		wantErr  error
	}{
		{name: "echo", expected: []byte(`{"id":1,"method":"Browser.getVersion"}`)},
		{name: "fragments", expected: []byte(`{"id":1,"result":{}}`)},
		{name: "large", expected: large},
	}
	for _, step := range steps {
		message, err := socket.ReadMessage()
		if err != nil {
			t.Fatalf("%s: read: %v", step.name, err)
		}
		if !bytes.Equal(message, step.expected) {
			t.Errorf("%s: unexpected message of %d bytes", step.name, len(message))
		}
		if step.name == "large" {
			if err := socket.WriteMessage(large); err != nil {
				t.Fatalf("write large: %v", err)
			}
		}
	}

	if _, err := socket.ReadMessage(); !errors.Is(err, io.EOF) {
		t.Errorf("expected the close to end reading, got %v", err)
	}
}

func TestDialWebSocket(t *testing.T) {
	t.Parallel()

	wrongAccept := websocketServer(t, true, func(*bufio.Reader, net.Conn) {})
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(refusing.Close)

	testCases := []struct {
		name    string
		url     string
		wantErr error
	}{
		{name: "a wrong accept", url: socketUrl(wrongAccept), wantErr: ErrHandshake},
		{name: "no upgrade", url: socketUrl(refusing), wantErr: ErrHandshake},
		{name: "not ws", url: "wss://127.0.0.1:1/", wantErr: ErrHandshake},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			socket, err := DialWebSocket(t.Context(), testCase.url)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("expected %v, got %v", testCase.wantErr, err)
			}
			if socket != nil {
				t.Error("expected no socket")
			}
		})
	}
}

// A conn over the real socket, end to end against the test server.
func TestConnOverWebSocket(t *testing.T) {
	t.Parallel()

	server := websocketServer(t, false, func(reader *bufio.Reader, conn net.Conn) {
		_, payload, err := readClientFrame(reader)
		if err != nil {
			return
		}
		var command struct {
			Id int64 `json:"id"`
		}
		_ = json.Unmarshal(payload, &command)
		_, _ = conn.Write(serverFrame(true, opText, []byte(`{"method":"Target.targetCreated","params":{}}`)))
		_, _ = conn.Write(serverFrame(true, opText, []byte(`{"id":`+strconv.FormatInt(command.Id, 10)+`,"result":{"userAgent":"Chrome/155"}}`)))
		_, _, _ = readClientFrame(reader)
	})

	socket, err := DialWebSocket(t.Context(), socketUrl(server))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn, err := NewConn(socket)
	if err != nil {
		t.Fatalf("new conn: %v", err)
	}
	defer func() { _ = conn.Close() }()

	var version struct {
		UserAgent string `json:"userAgent"`
	}
	if err := conn.Call(t.Context(), "", "Browser.getVersion", nil, &version); err != nil {
		t.Fatalf("call: %v", err)
	}
	if version.UserAgent != "Chrome/155" {
		t.Errorf("unexpected user agent: %q", version.UserAgent)
	}
}

// What a server may send that the client must refuse rather than act on.
func TestWebSocketMalformed(t *testing.T) {
	t.Parallel()

	tooLong := []byte{0x80 | opText, 127}
	tooLong = binary.BigEndian.AppendUint64(tooLong, maxMessageBytes+1)

	testCases := []struct {
		name    string
		frames  [][]byte
		wantErr error
	}{
		{name: "too large a frame", frames: [][]byte{tooLong}, wantErr: ErrMessageTooLarge},
		{name: "reserved bits", frames: [][]byte{{0x80 | 0x40 | opText, 0}}, wantErr: ErrProtocol},
		{name: "a continuation outside a message", frames: [][]byte{serverFrame(true, opContinuation, []byte("x"))}, wantErr: ErrProtocol},
		{
			name:    "a message inside a fragmented one",
			frames:  [][]byte{serverFrame(false, opText, []byte("a")), serverFrame(true, opText, []byte("b"))},
			wantErr: ErrProtocol,
		},
		{name: "an unknown opcode", frames: [][]byte{serverFrame(true, 0x3, nil)}, wantErr: ErrProtocol},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := websocketServer(t, false, func(reader *bufio.Reader, conn net.Conn) {
				for _, frame := range testCase.frames {
					_, _ = conn.Write(frame)
				}
				_, _, _ = readClientFrame(reader)
			})

			socket, err := DialWebSocket(t.Context(), socketUrl(server))
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer func() { _ = socket.Close() }()

			if _, err := socket.ReadMessage(); !errors.Is(err, testCase.wantErr) {
				t.Errorf("expected %v, got %v", testCase.wantErr, err)
			}
		})
	}
}

// Fragments each within bounds that together are not.
func TestWebSocketFragmentsTooLarge(t *testing.T) {
	t.Parallel()

	chunk := bytes.Repeat([]byte("x"), 16*1024*1024)
	server := websocketServer(t, false, func(reader *bufio.Reader, conn net.Conn) {
		_, _ = conn.Write(serverFrame(false, opText, chunk))
		for range maxMessageBytes / len(chunk) {
			if _, err := conn.Write(serverFrame(false, opContinuation, chunk)); err != nil {
				return
			}
		}
		_, _, _ = readClientFrame(reader)
	})

	socket, err := DialWebSocket(t.Context(), socketUrl(server))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = socket.Close() }()

	if _, err := socket.ReadMessage(); !errors.Is(err, ErrMessageTooLarge) {
		t.Errorf("expected the message refused, got %v", err)
	}
}
