package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

var errDialRefused = errors.New("dial refused")

// echoServer stands in for the instance: it writes back whatever it is sent.
func echoServer(t *testing.T) string {
	t.Helper()

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()

	return listener.Addr().String()
}

func dialTo(address string) dialFunc {
	return func(ctx context.Context) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", address)
	}
}

func TestServe(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		dial     dialFunc
		wantEcho bool
		wantLog  string
	}{
		{name: "forwards both ways", dial: dialTo(echoServer(t)), wantEcho: true},
		{
			name:    "a failed dial closes the client and is reported",
			dial:    func(context.Context) (net.Conn, error) { return nil, errDialRefused },
			wantLog: "dial refused",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			var stderr lockedBuffer
			done := make(chan struct{})
			go func() {
				serve(context.Background(), listener, testCase.dial, &stderr)
				close(done)
			}()

			client, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", listener.Addr().String())
			if err != nil {
				t.Fatalf("dial proxy: %v", err)
			}
			_ = client.SetDeadline(time.Now().Add(5 * time.Second))
			_, _ = client.Write([]byte("ping"))
			reply := make([]byte, 4)
			_, readErr := io.ReadFull(client, reply)
			_ = client.Close()

			_ = listener.Close()
			<-done

			if testCase.wantEcho && (readErr != nil || string(reply) != "ping") {
				t.Errorf("reply %q, %v; want ping", reply, readErr)
			}
			if !testCase.wantEcho && readErr == nil {
				t.Error("the client got a reply through a dial that failed")
			}
			if !strings.Contains(stderr.String(), testCase.wantLog) {
				t.Errorf("stderr %q, want it to contain %q", stderr.String(), testCase.wantLog)
			}
		})
	}
}

func TestProxy_Command(t *testing.T) {
	t.Parallel()

	target := echoServer(t)
	testCases := []struct {
		name     string
		command  []string
		wantCode int
		wantOut  string
	}{
		{
			name:    "the command sees the proxy and the username",
			command: []string{"sh", "-c", `test "$PGHOST" = 127.0.0.1 && test -n "$PGPORT" && test "$PGSSLMODE" = disable && printf %s "$PGUSER"`},
			wantOut: "v@example.com",
		},
		{name: "the command's status is the exit code", command: []string{"sh", "-c", "exit 3"}, wantCode: 3},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr lockedBuffer
			args := &arguments{instance: "proj:region:instance", address: "127.0.0.1", port: 0, command: testCase.command}
			code, err := proxy(context.Background(), args, dialTo(target), "v@example.com", &stdout, &stderr)
			if err != nil {
				t.Fatalf("proxy: %v (stderr %q)", err, stderr.String())
			}
			if code != testCase.wantCode {
				t.Errorf("exit code %d, want %d", code, testCase.wantCode)
			}
			if stdout.String() != testCase.wantOut {
				t.Errorf("stdout %q, want %q", stdout.String(), testCase.wantOut)
			}
		})
	}
}

func TestRun_Usage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		argv     []string
		wantCode int
	}{
		{name: "help", argv: []string{"--help"}, wantCode: exitClean},
		{name: "no instance", argv: nil, wantCode: exitUsage},
		{name: "unknown address type", argv: []string{"--ip-type", "PSC", "p:r:i"}, wantCode: exitUsage},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			code, err := run(context.Background(), testCase.argv, &stdout, &stderr)
			if err != nil || code != testCase.wantCode {
				t.Errorf("run(%v) = %d, %v; want %d", testCase.argv, code, err, testCase.wantCode)
			}
		})
	}
}

// lockedBuffer is a bytes.Buffer the proxy's goroutines may write to while the test reads it.
type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
