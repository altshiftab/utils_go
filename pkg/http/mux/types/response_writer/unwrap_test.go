package response_writer

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

// controllableWriter implements http.ResponseWriter plus the optional methods
// http.ResponseController looks for, recording which ones it was asked for.
type controllableWriter struct {
	header http.Header

	flushed          bool
	readDeadlineSet  bool
	writeDeadlineSet bool
}

func (w *controllableWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *controllableWriter) Write(data []byte) (int, error) { return len(data), nil }

func (w *controllableWriter) WriteHeader(int) {}

func (w *controllableWriter) Flush() { w.flushed = true }

func (w *controllableWriter) SetReadDeadline(time.Time) error {
	w.readDeadlineSet = true
	return nil
}

func (w *controllableWriter) SetWriteDeadline(time.Time) error {
	w.writeDeadlineSet = true
	return nil
}

// TestUnwrapReachesTheWrappedWriter pins the reason Unwrap exists: without it
// http.ResponseController stops at this struct, because the embedded field is
// an interface and promotes only Header, Write and WriteHeader. Every call then
// returns http.ErrNotSupported however capable the wrapped writer is, and
// nothing says so unless the caller checks the error.
func TestUnwrapReachesTheWrappedWriter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		call func(*http.ResponseController) error
		// reached reports whether the wrapped writer saw the call.
		reached func(*controllableWriter) bool
	}{
		{
			name:    "flush",
			call:    func(controller *http.ResponseController) error { return controller.Flush() },
			reached: func(w *controllableWriter) bool { return w.flushed },
		},
		{
			name: "set read deadline",
			call: func(controller *http.ResponseController) error {
				return controller.SetReadDeadline(time.Time{})
			},
			reached: func(w *controllableWriter) bool { return w.readDeadlineSet },
		},
		{
			name: "set write deadline",
			call: func(controller *http.ResponseController) error {
				return controller.SetWriteDeadline(time.Time{})
			},
			reached: func(w *controllableWriter) bool { return w.writeDeadlineSet },
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			wrapped := &controllableWriter{}
			responseWriter := &ResponseWriter{ResponseWriter: wrapped}

			if err := testCase.call(http.NewResponseController(responseWriter)); err != nil {
				if errors.Is(err, http.ErrNotSupported) {
					t.Fatalf(
						"%s: the controller could not reach the wrapped writer (Unwrap missing?): %v",
						testCase.name,
						err,
					)
				}
				t.Fatalf("%s: unexpected error: %v", testCase.name, err)
			}

			if !testCase.reached(wrapped) {
				t.Errorf("%s: the call returned nil but never reached the wrapped writer", testCase.name)
			}
		})
	}
}

// TestUnwrapDoesNotInventCapabilities checks the other half: unwrapping must
// expose what the wrapped writer actually supports and nothing more, so a
// handler still learns that a writer without these methods cannot do them.
func TestUnwrapDoesNotInventCapabilities(t *testing.T) {
	t.Parallel()

	responseWriter := &ResponseWriter{ResponseWriter: &headerOnlyWriter{}}

	err := http.NewResponseController(responseWriter).SetReadDeadline(time.Time{})
	if !errors.Is(err, http.ErrNotSupported) {
		t.Errorf("expected http.ErrNotSupported for a writer without deadlines, got %v", err)
	}
}
