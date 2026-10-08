// Package turnstile passes Cloudflare's Turnstile the way a person does: when
// its widget asks whether the browser is human, it clicks the box.
//
// Measured against MVGroup and Letterboxd (2026-10): a headed Chrome with a
// fresh profile is either let through or shown the checkbox, and a mouse click
// sent through the DevTools protocol passes the latter in a few seconds. Only
// the DOM and Input domains are used -- nothing is evaluated in the page while
// the widget is deciding, and no script domain is enabled for it to notice.
package turnstile

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"strings"
	"time"

	"github.com/altshiftab/utils_go/pkg/cdp"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

const (
	challengeHost   = "challenges.cloudflare.com"
	pollInterval    = 500 * time.Millisecond
	reclickInterval = 8 * time.Second
	// settleDuration is how long the widget is left to load before it is
	// clicked, as a person would take to find it.
	settleDuration = 1500 * time.Millisecond
)

// ErrUnsolved is a challenge still standing when the time ran out.
var ErrUnsolved = errors.New("challenge not solved")

// Done reports whether the page is past what the widget was guarding: the
// challenge page gone, or a form's submit button enabled.
type Done func(ctx context.Context) (bool, error)

// box is a rectangle on the page, in CSS pixels.
type box struct {
	X, Y, Width, Height float64
}

// Solve watches the page behind the session until done reports true, clicking
// Turnstile's checkbox whenever it is showing, has settled and has not just
// been clicked. The context bounds the wait; when it ends first the error is
// ErrUnsolved.
func Solve(ctx context.Context, caller cdp.Caller, sessionId string, done Done) error {
	if caller == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("caller"))
	}
	if done == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("done"))
	}

	//nolint:gosec // G404: jitter, not a secret.
	settle := settleDuration + time.Duration(rand.Int64N(int64(time.Second)))

	var seenAt, clickedAt time.Time
	for {
		finished, err := done(ctx)
		if err != nil {
			return fmt.Errorf("done: %w", err)
		}
		if finished {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", ErrUnsolved, ctx.Err())
		case <-time.After(pollInterval):
		}

		if !clickedAt.IsZero() && time.Since(clickedAt) < reclickInterval {
			continue
		}

		widget, err := widgetBox(ctx, caller, sessionId)
		if err != nil {
			return fmt.Errorf("widget box: %w", err)
		}
		if widget == nil {
			seenAt = time.Time{}
			continue
		}
		if seenAt.IsZero() {
			seenAt = time.Now()
		}
		if time.Since(seenAt) < settle {
			continue
		}

		x, y := checkboxPoint(widget)
		if err := cdp.Click(ctx, caller, sessionId, x, y); err != nil {
			return fmt.Errorf("click: %w", err)
		}
		clickedAt = time.Now()
		slog.InfoContext(ctx, "Turnstile's checkbox was clicked.")
	}
}

// domNode is the part of a DOM node the search needs. The widget's frame sits
// in a closed shadow root, which the protocol reaches into when asked to
// pierce.
type domNode struct {
	NodeName        string     `json:"nodeName"`
	BackendNodeId   int64      `json:"backendNodeId"`
	Attributes      []string   `json:"attributes"`
	Children        []*domNode `json:"children"`
	ShadowRoots     []*domNode `json:"shadowRoots"`
	ContentDocument *domNode   `json:"contentDocument"`
}

func (n *domNode) attribute(name string) string {
	for index := 0; index+1 < len(n.Attributes); index += 2 {
		if n.Attributes[index] == name {
			return n.Attributes[index+1]
		}
	}
	return ""
}

// findChallengeFrame returns the frame Turnstile's widget is drawn in.
func findChallengeFrame(root *domNode) *domNode {
	stack := []*domNode{root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == nil {
			continue
		}

		if strings.EqualFold(node.NodeName, "iframe") {
			if frameUrl, err := url.Parse(node.attribute("src")); err == nil && frameUrl.Hostname() == challengeHost {
				return node
			}
		}

		stack = append(stack, node.Children...)
		stack = append(stack, node.ShadowRoots...)
		stack = append(stack, node.ContentDocument)
	}
	return nil
}

// widgetBox returns where Turnstile's widget is, when it is showing.
func widgetBox(ctx context.Context, caller cdp.Caller, sessionId string) (*box, error) {
	var document struct {
		Root *domNode `json:"root"`
	}
	if err := caller.Call(ctx, sessionId, "DOM.getDocument", map[string]any{"depth": -1, "pierce": true}, &document); err != nil {
		// A page between documents has none for a moment.
		if cdp.IsTransient(err) {
			return nil, nil
		}
		return nil, err
	}

	frame := findChallengeFrame(document.Root)
	if frame == nil {
		return nil, nil
	}

	var model struct {
		Model *struct {
			Content []float64 `json:"content"`
			Width   float64   `json:"width"`
			Height  float64   `json:"height"`
		} `json:"model"`
	}
	if err := caller.Call(ctx, sessionId, "DOM.getBoxModel", map[string]any{"backendNodeId": frame.BackendNodeId}, &model); err != nil {
		// A frame being replaced, or one not yet laid out, has no box.
		if cdp.IsTransient(err) {
			return nil, nil
		}
		return nil, err
	}
	if model.Model == nil || len(model.Model.Content) < 2 || model.Model.Width <= 0 || model.Model.Height <= 0 {
		return nil, nil
	}

	return &box{X: model.Model.Content[0], Y: model.Model.Content[1], Width: model.Model.Width, Height: model.Model.Height}, nil
}

// checkboxPoint is where the widget's checkbox is: inset from its left edge,
// half way down, with the jitter of a hand.
func checkboxPoint(widget *box) (float64, float64) {
	//nolint:gosec // G404: jitter, not a secret.
	return widget.X + 26 + rand.Float64()*6, widget.Y + widget.Height/2 + rand.Float64()*4 - 2
}
