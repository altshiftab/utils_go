// Package chrome starts a headed Chrome and connects to it over the DevTools
// protocol, the way a page behind a bot check needs it driven.
//
// The DevTools port is chosen here and given to Chrome by number: a Chrome told
// port 0, or driven over --remote-debugging-pipe, declares itself automated to
// every page (navigator.webdriver is true), and Cloudflare's Turnstile then
// never passes. Headless is refused outright, so Chrome needs a display, Xvfb
// in a container.
package chrome

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/altshiftab/utils_go/pkg/cdp"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

const (
	// shutdownDuration is how long a closing Chrome is given before it is killed.
	shutdownDuration = 3 * time.Second
	// endpointPollInterval is how often a starting Chrome is asked whether it
	// is listening yet.
	endpointPollInterval = 100 * time.Millisecond
)

// ErrChromeExited is Chrome gone before it listened.
var ErrChromeExited = errors.New("chrome exited")

// DefaultFlags are what Chrome is started with besides the profile and the
// port. The sandbox needs namespaces a container does not give, and /dev/shm in
// a container is too small for Chrome's shared memory.
var DefaultFlags = []string{
	"--no-first-run",
	"--no-default-browser-check",
	"--no-sandbox",
	"--disable-dev-shm-usage",
	"--password-store=basic",
	"--window-size=1280,720",
	// Nothing to Google while it runs: no update checks, no field trials.
	"--disable-background-networking",
}

// Browser is a running Chrome and the connection to it.
type Browser struct {
	Conn *cdp.Conn

	command *exec.Cmd
	exited  chan struct{}
}

// Page is a tab of the browser and the session it is addressed through.
type Page struct {
	TargetId  string
	SessionId string
}

// Launch starts Chrome on the profile directory with DefaultFlags and the
// flags given, and connects to it. Chrome is killed, with every process it
// started, when the context ends; Close ends it sooner, and more gently.
func Launch(ctx context.Context, chromePath string, profileDirectory string, flags ...string) (*Browser, error) {
	if chromePath == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("chrome path"))
	}
	if profileDirectory == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("profile directory"))
	}

	port, err := FreePort(ctx)
	if err != nil {
		return nil, fmt.Errorf("free port: %w", err)
	}

	arguments := append(slices.Clone(DefaultFlags), flags...)
	arguments = append(
		arguments,
		"--user-data-dir="+profileDirectory,
		"--remote-debugging-port="+strconv.Itoa(port),
		"about:blank",
	)

	// A group of its own, so that its renderers and GPU process go with it
	// when it is killed rather than outliving it.
	//nolint:gosec // G204: the binary is the operator's configured Chrome; the arguments are the caller's.
	command := exec.CommandContext(ctx, chromePath, arguments...)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	if err := command.Start(); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("exec command start: %w", err), chromePath)
	}

	browser := &Browser{command: command, exited: make(chan struct{})}
	go func() {
		_ = command.Wait()
		close(browser.exited)
	}()

	endpoint, err := DevToolsEndpoint(ctx, port, browser.exited)
	if err != nil {
		browser.Close(ctx)
		return nil, fmt.Errorf("devtools endpoint: %w", err)
	}

	socket, err := cdp.DialWebSocket(ctx, endpoint)
	if err != nil {
		browser.Close(ctx)
		return nil, fmt.Errorf("cdp dial websocket: %w", err)
	}

	conn, err := cdp.NewConn(socket)
	if err != nil {
		_ = socket.Close()
		browser.Close(ctx)
		return nil, fmt.Errorf("cdp new conn: %w", err)
	}
	browser.Conn = conn

	return browser, nil
}

// Close asks Chrome to close and kills it, group and all, when it does not in
// time. It runs to the end even when the context has ended -- a Chrome whose
// context ended has been killed already, and the asking then fails at once.
func (b *Browser) Close(ctx context.Context) {
	if b == nil || b.command == nil || b.command.Process == nil {
		return
	}

	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownDuration)
	defer cancel()

	if b.Conn != nil {
		_ = b.Conn.Call(closeCtx, "", "Browser.close", nil, nil)
		_ = b.Conn.Close()
	}

	pid := b.command.Process.Pid
	select {
	case <-b.exited:
	case <-closeCtx.Done():
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		<-b.exited
	}
	// Whatever of the group is left once the browser itself has gone.
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

// NewPage opens a blank tab and attaches to it.
func (b *Browser) NewPage(ctx context.Context) (*Page, error) {
	if b == nil || b.Conn == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("browser connection"))
	}
	return NewPage(ctx, b.Conn)
}

// NewPage opens a blank tab in the browser behind the caller and attaches to
// it, flattened, so that its session id addresses it on the same connection.
func NewPage(ctx context.Context, caller cdp.Caller) (*Page, error) {
	if caller == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("caller"))
	}

	var created struct {
		TargetId string `json:"targetId"`
	}
	if err := caller.Call(ctx, "", "Target.createTarget", map[string]any{"url": "about:blank"}, &created); err != nil {
		return nil, fmt.Errorf("create target: %w", err)
	}
	if created.TargetId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("target id"))
	}

	var attached struct {
		SessionId string `json:"sessionId"`
	}
	if err := caller.Call(ctx, "", "Target.attachToTarget", map[string]any{"targetId": created.TargetId, "flatten": true}, &attached); err != nil {
		return nil, fmt.Errorf("attach to target: %w", err)
	}
	if attached.SessionId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("session id"))
	}

	return &Page{TargetId: created.TargetId, SessionId: attached.SessionId}, nil
}

// FreePort returns a loopback port nothing is listening on. Another process
// could take it before Chrome does; nothing is done about that race.
func FreePort(ctx context.Context) (int, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, altshiftErrors.NewWithTrace(fmt.Errorf("net listen: %w", err))
	}
	defer func() {
		_ = listener.Close()
	}()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok || address == nil {
		return 0, altshiftErrors.NewWithTrace(nil_error.New("tcp address"))
	}
	return address.Port, nil
}

// DevToolsEndpoint waits for Chrome to listen on its port and returns the
// browser's own endpoint there, from the version it reports. With a fixed port
// Chrome writes no DevToolsActivePort file to read it from instead.
func DevToolsEndpoint(ctx context.Context, port int, exited <-chan struct{}) (string, error) {
	versionUrl := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + "/json/version"
	client := &http.Client{Timeout: endpointPollInterval * 5}

	for {
		if endpoint := browserEndpoint(ctx, client, versionUrl); endpoint != "" {
			return endpoint, nil
		}

		select {
		case <-ctx.Done():
			return "", altshiftErrors.NewWithTrace(fmt.Errorf("wait: %w", ctx.Err()), versionUrl)
		case <-exited:
			return "", altshiftErrors.NewWithTrace(ErrChromeExited, versionUrl)
		case <-time.After(endpointPollInterval):
		}
	}
}

// browserEndpoint asks Chrome for its endpoint, answering nothing while it is
// not yet listening.
func browserEndpoint(ctx context.Context, client *http.Client, versionUrl string) string {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, versionUrl, nil)
	if err != nil {
		return ""
	}

	response, err := client.Do(request)
	if err != nil || response == nil {
		return ""
	}
	defer func() {
		_ = response.Body.Close()
	}()

	var version struct {
		WebSocketDebuggerUrl string `json:"webSocketDebuggerUrl"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&version) != nil {
		return ""
	}
	if !strings.HasPrefix(version.WebSocketDebuggerUrl, "ws://") {
		return ""
	}
	return version.WebSocketDebuggerUrl
}
