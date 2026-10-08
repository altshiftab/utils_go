package cdp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftTestingCmp "github.com/altshiftab/utils_go/pkg/testing/cmp"
)

var errInputRefused = errors.New("input refused")

// recordingCaller keeps the mouse events sent to it, failing on the one named.
type recordingCaller struct {
	failOn string
	events []map[string]any
}

func (r *recordingCaller) Call(_ context.Context, _ string, _ string, params any, _ any) error {
	event, _ := params.(map[string]any)
	r.events = append(r.events, event)
	if event["type"] == r.failOn {
		return errInputRefused
	}
	return nil
}

func TestClick(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		failOn  string
		wantErr error
	}{
		{name: "pressed"},
		{name: "move refused", failOn: "mouseMoved", wantErr: errInputRefused},
		{name: "press refused", failOn: "mousePressed", wantErr: errInputRefused},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			caller := &recordingCaller{failOn: testCase.failOn}
			err := Click(t.Context(), caller, "session-1", 120.5, 330)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("expected %v, got %v", testCase.wantErr, err)
			}
			if testCase.wantErr != nil {
				return
			}

			if len(caller.events) != clickSteps+2 {
				t.Fatalf("unexpected event count: %d", len(caller.events))
			}
			// The pointer arrives where it presses, and presses then releases there.
			for index, kind := range map[int]string{clickSteps - 1: "mouseMoved", clickSteps: "mousePressed", clickSteps + 1: "mouseReleased"} {
				event := caller.events[index]
				if event["type"] != kind || event["x"] != 120.5 || event["y"] != 330.0 {
					t.Errorf("event %d: %v", index, event)
				}
			}
		})
	}

	err := Click(t.Context(), nil, "session-1", 0, 0)
	altshiftTestingCmp.CompareErr(t, err, nil_error.New("caller"))
}

func TestPause(t *testing.T) {
	t.Parallel()

	started := time.Now()
	Pause(t.Context(), 20*time.Millisecond, 0)
	if waited := time.Since(started); waited < 20*time.Millisecond {
		t.Errorf("paused too briefly: %s", waited)
	}

	// An ended context ends the pause.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	started = time.Now()
	Pause(ctx, time.Minute, time.Minute)
	if waited := time.Since(started); waited > time.Second {
		t.Errorf("paused past the context: %s", waited)
	}
}
