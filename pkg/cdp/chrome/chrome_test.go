package chrome

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftTestingCmp "github.com/altshiftab/utils_go/pkg/testing/cmp"
)

func TestDevToolsEndpoint(t *testing.T) {
	t.Parallel()

	const endpoint = "ws://127.0.0.1:9444/devtools/browser/0c8f"

	testCases := []struct {
		name     string
		listenAt time.Duration
		body     string
		exit     bool
		expected string
		wantIs   error
	}{
		{name: "listening", body: `{"webSocketDebuggerUrl": "` + endpoint + `"}`, expected: endpoint},
		{name: "listening late", listenAt: 300 * time.Millisecond, body: `{"webSocketDebuggerUrl": "` + endpoint + `"}`, expected: endpoint},
		{name: "answering without an endpoint", body: `{}`, wantIs: context.DeadlineExceeded},
		{name: "answering with another scheme", body: `{"webSocketDebuggerUrl": "http://127.0.0.1:9444/"}`, wantIs: context.DeadlineExceeded},
		{name: "chrome gone first", exit: true, wantIs: ErrChromeExited},
		{name: "never listening", wantIs: context.DeadlineExceeded},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			port, err := FreePort(t.Context())
			if err != nil {
				t.Fatalf("free port: %v", err)
			}
			exited := make(chan struct{})

			go func() {
				if testCase.body != "" {
					time.Sleep(testCase.listenAt)
					listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
					if err != nil {
						t.Errorf("listen: %v", err)
						return
					}
					server := &http.Server{
						Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							if r.URL.Path != "/json/version" {
								http.NotFound(w, r)
								return
							}
							_, _ = w.Write([]byte(testCase.body))
						}),
						ReadHeaderTimeout: time.Second,
					}
					t.Cleanup(func() { _ = server.Close() })
					go func() { _ = server.Serve(listener) }()
				}
				if testCase.exit {
					time.Sleep(100 * time.Millisecond)
					close(exited)
				}
			}()

			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()

			got, err := DevToolsEndpoint(ctx, port, exited)
			if testCase.wantIs != nil {
				if !errors.Is(err, testCase.wantIs) {
					t.Fatalf("expected %v, got %v", testCase.wantIs, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("devtools endpoint: %v", err)
			}
			if got != testCase.expected {
				t.Errorf("unexpected endpoint: %q", got)
			}
		})
	}
}

func TestFreePort(t *testing.T) {
	t.Parallel()

	port, err := FreePort(t.Context())
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Fatalf("unexpected port: %d", port)
	}

	// Free means a listener can have it.
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("listen on the free port: %v", err)
	}
	_ = listener.Close()
}

// fakeTargets answers Target.createTarget and Target.attachToTarget.
type fakeTargets struct {
	targetId   string
	sessionId  string
	attachedTo string
	flatten    bool
}

func (f *fakeTargets) Call(_ context.Context, _ string, method string, params any, result any) error {
	var response any
	switch method {
	case "Target.createTarget":
		response = map[string]any{"targetId": f.targetId}
	case "Target.attachToTarget":
		values, _ := params.(map[string]any)
		f.attachedTo, _ = values["targetId"].(string)
		f.flatten, _ = values["flatten"].(bool)
		response = map[string]any{"sessionId": f.sessionId}
	}

	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}

func TestNewPage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		targets  *fakeTargets
		expected *Page
		wantErr  error
	}{
		{
			name:     "attached",
			targets:  &fakeTargets{targetId: "target-1", sessionId: "session-1"},
			expected: &Page{TargetId: "target-1", SessionId: "session-1"},
		},
		{name: "no target", targets: &fakeTargets{sessionId: "session-1"}, wantErr: empty_error.New("target id")},
		{name: "no session", targets: &fakeTargets{targetId: "target-1"}, wantErr: empty_error.New("session id")},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			page, err := NewPage(t.Context(), testCase.targets)
			if testCase.wantErr != nil {
				altshiftTestingCmp.CompareErr(t, err, testCase.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("new page: %v", err)
			}
			if diff := altshiftTestingCmp.Diff(testCase.expected, page); diff != "" {
				t.Errorf("page mismatch (-expected +got):\n%s", diff)
			}
			if testCase.targets.attachedTo != testCase.expected.TargetId || !testCase.targets.flatten {
				t.Errorf("attached to %q, flatten %v", testCase.targets.attachedTo, testCase.targets.flatten)
			}
		})
	}

	_, err := NewPage(t.Context(), nil)
	altshiftTestingCmp.CompareErr(t, err, nil_error.New("caller"))

	_, err = (&Browser{}).NewPage(t.Context())
	altshiftTestingCmp.CompareErr(t, err, nil_error.New("browser connection"))
}

func TestLaunch(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		chromePath       string
		profileDirectory string
		wantErr          error
	}{
		{name: "no chrome", profileDirectory: t.TempDir(), wantErr: empty_error.New("chrome path")},
		{name: "no profile", chromePath: "google-chrome-stable", wantErr: empty_error.New("profile directory")},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := Launch(t.Context(), testCase.chromePath, testCase.profileDirectory)
			altshiftTestingCmp.CompareErr(t, err, testCase.wantErr)
		})
	}
}

// A binary that exits at once is reported as gone, not waited on.
func TestLaunchExited(t *testing.T) {
	t.Parallel()

	exits, err := exec.LookPath("true")
	if err != nil {
		t.Skipf("no true to stand in for a chrome that exits: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	if _, err := Launch(ctx, exits, t.TempDir()); !errors.Is(err, ErrChromeExited) {
		t.Fatalf("expected chrome to have exited, got %v", err)
	}
}

// Closing a browser that never started does nothing.
func TestCloseUnstarted(t *testing.T) {
	t.Parallel()

	(&Browser{}).Close(t.Context())
	var browser *Browser
	browser.Close(t.Context())
}
