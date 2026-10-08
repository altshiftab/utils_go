package turnstile

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/cdp"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftTestingCmp "github.com/altshiftab/utils_go/pkg/testing/cmp"
)

var errBrowserGone = errors.New("browser gone")

var testWidget = &box{X: 100, Y: 300, Width: 300, Height: 65}

// fakeBrowser answers the commands Solve sends the way Chrome does for a page
// behind Turnstile: the widget's frame inside a shadow root, passed once the
// checkbox has been pressed -- or at once, for a browser Turnstile lets
// through, or never.
type fakeBrowser struct {
	passAtOnce bool
	neverPass  bool
	// noDocument answers DOM.getDocument with a protocol error, as a page
	// between documents does.
	noDocument bool
	// gone answers every DOM command as Chrome does for a closed page.
	gone bool

	mutex    sync.Mutex
	presses  [][2]float64
	sessions []string
}

// passed is whether the challenge has let the browser through.
func (f *fakeBrowser) passed() bool {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return !f.neverPass && (f.passAtOnce || len(f.presses) > 0)
}

func (f *fakeBrowser) done(context.Context) (bool, error) {
	return f.passed(), nil
}

func (f *fakeBrowser) Call(_ context.Context, sessionId string, method string, params any, result any) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.sessions = append(f.sessions, sessionId)

	var response any
	switch method {
	case "DOM.getDocument":
		if f.gone {
			return &cdp.ProtocolError{Code: -32001, Message: "Session with given id not found."}
		}
		if f.noDocument {
			return &cdp.ProtocolError{Code: -32000, Message: "no document"}
		}
		response = map[string]any{"root": map[string]any{
			"nodeName": "#document",
			"children": []any{map[string]any{
				"nodeName": "HTML",
				"children": []any{
					map[string]any{"nodeName": "IFRAME", "backendNodeId": 7, "attributes": []string{"src", "https://www.youtube.com/embed/x"}},
					map[string]any{"nodeName": "DIV", "shadowRoots": []any{map[string]any{
						"nodeName": "#document-fragment",
						"children": []any{map[string]any{
							"nodeName":      "IFRAME",
							"backendNodeId": 42,
							"attributes":    []string{"style", "border: none", "src", "https://challenges.cloudflare.com/cdn-cgi/challenge-platform/turnstile"},
						}},
					}}},
				},
			}},
		}}
	case "DOM.getBoxModel":
		if node, _ := params.(map[string]any)["backendNodeId"].(int64); node != 42 {
			return &cdp.ProtocolError{Code: -32000, Message: "no box"}
		}
		response = map[string]any{"model": map[string]any{
			"content": []float64{testWidget.X, testWidget.Y, testWidget.X + testWidget.Width, testWidget.Y},
			"width":   testWidget.Width,
			"height":  testWidget.Height,
		}}
	case "Input.dispatchMouseEvent":
		event, _ := params.(map[string]any)
		if event["type"] == "mousePressed" {
			x, _ := event["x"].(float64)
			y, _ := event["y"].(float64)
			f.presses = append(f.presses, [2]float64{x, y})
		}
	}

	if result == nil || response == nil {
		return nil
	}
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}

func TestSolve(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		browser     *fakeBrowser
		timeout     time.Duration
		wantPresses int
		wantErr     error
		// wantGone is the solving ending at once on the page being gone.
		wantGone bool
	}{
		{name: "let through", browser: &fakeBrowser{passAtOnce: true}, timeout: 10 * time.Second},
		{name: "asked to click", browser: &fakeBrowser{}, timeout: 10 * time.Second, wantPresses: 1},
		{name: "never passed", browser: &fakeBrowser{neverPass: true}, timeout: 3 * time.Second, wantErr: ErrUnsolved},
		{name: "no document yet", browser: &fakeBrowser{neverPass: true, noDocument: true}, timeout: 2 * time.Second, wantErr: ErrUnsolved},
		{name: "the page gone", browser: &fakeBrowser{neverPass: true, gone: true}, timeout: 10 * time.Second, wantGone: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testCase.timeout)
			defer cancel()

			browser := testCase.browser
			err := Solve(ctx, browser, "session-1", browser.done)
			if testCase.wantGone {
				// Not waited out: the page being gone ends the solving at once.
				if _, ok := errors.AsType[*cdp.ProtocolError](err); !ok || ctx.Err() != nil {
					t.Fatalf("expected a protocol error at once, got %v", err)
				}
			} else if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("expected %v, got %v", testCase.wantErr, err)
				}
			} else if err != nil {
				t.Fatalf("solve: %v", err)
			}

			browser.mutex.Lock()
			defer browser.mutex.Unlock()

			for _, session := range browser.sessions {
				if session != "session-1" {
					t.Errorf("a command was sent outside the page's session: %q", session)
				}
			}
			if testCase.wantErr == nil && !testCase.wantGone && len(browser.presses) != testCase.wantPresses {
				t.Fatalf("unexpected presses: %v", browser.presses)
			}
			for _, press := range browser.presses {
				// On the checkbox, near the widget's left edge and half way down.
				if press[0] < testWidget.X+20 || press[0] > testWidget.X+40 ||
					press[1] < testWidget.Y+testWidget.Height/2-4 || press[1] > testWidget.Y+testWidget.Height/2+4 {
					t.Errorf("pressed outside the checkbox: %v", press)
				}
			}
		})
	}
}

func TestSolveDoneFails(t *testing.T) {
	t.Parallel()

	done := func(context.Context) (bool, error) { return false, errBrowserGone }
	if err := Solve(t.Context(), &fakeBrowser{}, "session-1", done); !errors.Is(err, errBrowserGone) {
		t.Fatalf("expected the done error, got %v", err)
	}
}

func TestSolveNil(t *testing.T) {
	t.Parallel()

	done := func(context.Context) (bool, error) { return true, nil }

	err := Solve(t.Context(), nil, "session-1", done)
	altshiftTestingCmp.CompareErr(t, err, nil_error.New("caller"))

	err = Solve(t.Context(), &fakeBrowser{}, "session-1", nil)
	altshiftTestingCmp.CompareErr(t, err, nil_error.New("done"))
}

func TestFindChallengeFrame(t *testing.T) {
	t.Parallel()

	challengeFrame := &domNode{NodeName: "IFRAME", BackendNodeId: 3, Attributes: []string{"src", "https://challenges.cloudflare.com/x"}}

	testCases := []struct {
		name     string
		root     *domNode
		expected *domNode
	}{
		{name: "nothing", root: &domNode{NodeName: "#document"}},
		{name: "nil", root: nil},
		{
			name: "another frame only",
			root: &domNode{Children: []*domNode{{NodeName: "IFRAME", Attributes: []string{"src", "https://challenges.cloudflare.com.evil.com/"}}}},
		},
		{
			name:     "in a nested document",
			root:     &domNode{Children: []*domNode{{NodeName: "IFRAME", ContentDocument: &domNode{Children: []*domNode{challengeFrame}}}}},
			expected: challengeFrame,
		},
		{
			name:     "in a shadow root",
			root:     &domNode{Children: []*domNode{{NodeName: "DIV", ShadowRoots: []*domNode{{Children: []*domNode{challengeFrame}}}}}},
			expected: challengeFrame,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if frame := findChallengeFrame(testCase.root); frame != testCase.expected {
				t.Errorf("unexpected frame: %+v", frame)
			}
		})
	}
}

func TestCheckboxPoint(t *testing.T) {
	t.Parallel()

	for range 100 {
		x, y := checkboxPoint(testWidget)
		if x < testWidget.X+26 || x > testWidget.X+32 {
			t.Fatalf("x off the checkbox: %v", x)
		}
		if middle := testWidget.Y + testWidget.Height/2; y < middle-2 || y > middle+2 {
			t.Fatalf("y off the checkbox: %v", y)
		}
	}
}
