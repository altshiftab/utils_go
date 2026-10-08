package cdp

import (
	"context"
	"math/rand/v2"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

// clickSteps is how many moves the pointer takes on its way to a click.
const clickSteps = 12

// Click moves the mouse to the point from somewhere below and to its left and
// presses it there, at the pace of a hand rather than in one event.
func Click(ctx context.Context, caller Caller, sessionId string, x float64, y float64) error {
	if caller == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("caller"))
	}

	fromX, fromY := x-180, y+90
	for step := 1; step <= clickSteps; step++ {
		progress := float64(step) / clickSteps
		params := map[string]any{"type": "mouseMoved", "x": fromX + (x-fromX)*progress, "y": fromY + (y-fromY)*progress}
		if err := caller.Call(ctx, sessionId, "Input.dispatchMouseEvent", params, nil); err != nil {
			return err
		}
		Pause(ctx, 15*time.Millisecond, 30*time.Millisecond)
	}

	press := map[string]any{"type": "mousePressed", "x": x, "y": y, "button": "left", "clickCount": 1}
	if err := caller.Call(ctx, sessionId, "Input.dispatchMouseEvent", press, nil); err != nil {
		return err
	}
	Pause(ctx, 60*time.Millisecond, 80*time.Millisecond)

	release := map[string]any{"type": "mouseReleased", "x": x, "y": y, "button": "left", "clickCount": 1}
	return caller.Call(ctx, sessionId, "Input.dispatchMouseEvent", release, nil)
}

// Pause waits between least and least+spread, or until the context ends.
func Pause(ctx context.Context, least time.Duration, spread time.Duration) {
	duration := least
	if spread > 0 {
		//nolint:gosec // G404: jitter, not a secret.
		duration += time.Duration(rand.Int64N(int64(spread)))
	}

	select {
	case <-ctx.Done():
	case <-time.After(duration):
	}
}
