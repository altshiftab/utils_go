package errors

import (
	"fmt"
	"strings"
	"testing"
)

// uncomparableError contains a slice field, making it uncomparable with ==.
type uncomparableError struct {
	Details []string
}

func (e uncomparableError) Error() string {
	return fmt.Sprintf("uncomparable: %v", e.Details)
}

func (e uncomparableError) Unwrap() error {
	return fmt.Errorf("inner error")
}

func TestCollectWrappedErrors_UncomparableType(t *testing.T) {
	t.Parallel()

	err := uncomparableError{Details: []string{"a", "b"}}

	// This must not panic.
	results := CollectWrappedErrors(err)

	if len(results) != 1 {
		t.Fatalf("expected 1 wrapped error, got %d", len(results))
	}
	if results[0].Error() != "inner error" {
		t.Fatalf("expected 'inner error', got %q", results[0].Error())
	}
}

func TestCollectWrappedErrors_ComparableType(t *testing.T) {
	t.Parallel()

	inner := fmt.Errorf("inner")
	err := fmt.Errorf("outer: %w", inner)

	results := CollectWrappedErrors(err)

	if len(results) != 1 {
		t.Fatalf("expected 1 wrapped error, got %d", len(results))
	}
	if results[0] != inner {
		t.Fatalf("expected inner error, got %v", results[0])
	}
}

func TestCollectWrappedErrors_NilError(t *testing.T) {
	t.Parallel()

	results := CollectWrappedErrors(nil)

	if len(results) != 0 {
		t.Fatalf("expected 0 wrapped errors, got %d", len(results))
	}
}

// Ensure structurally identical wrapped errors are NOT skipped.
func TestCollectWrappedErrors_StructurallyIdenticalChild(t *testing.T) {
	t.Parallel()

	// Use an uncomparable error without Unwrap as the child.
	child := uncomparableLeafError{Details: []string{"a", "b"}}
	parent := wrappingError{msg: "parent", wrapped: child}

	results := CollectWrappedErrors(parent)

	// child is structurally identical to what root would look like,
	// but it's a different instance — it must still be collected.
	if len(results) != 1 {
		t.Fatalf("expected 1 wrapped error, got %d", len(results))
	}
}

// Verify that reflect.DeepEqual would incorrectly skip this case.
func TestCollectWrappedErrors_DeepEqualWouldSkip(t *testing.T) {
	t.Parallel()

	// Parent and child are structurally identical uncomparable errors.
	child := uncomparableLeafError{Details: []string{"x"}}
	parent := uncomparableLeafError{Details: []string{"x"}}
	parent.wrapped = child

	results := CollectWrappedErrors(parent)

	// reflect.DeepEqual would consider child == parent and skip it.
	// Our fix must still collect the child.
	if len(results) != 1 {
		t.Fatalf("expected 1 wrapped error, got %d", len(results))
	}
}

type wrappingError struct {
	msg     string
	wrapped error
}

func (e wrappingError) Error() string { return e.msg }
func (e wrappingError) Unwrap() error { return e.wrapped }

// uncomparableLeafError is uncomparable (has a slice) and optionally wraps another error.
type uncomparableLeafError struct {
	Details []string
	wrapped error
}

func (e uncomparableLeafError) Error() string {
	return fmt.Sprintf("leaf: %v", e.Details)
}

func (e uncomparableLeafError) Unwrap() error { return e.wrapped }

// traceHelper stands one frame above whatever calls it, so that a test can say which function a
// captured stack is expected to start at.
func traceHelper() *ExtendedError {
	return NewWithTrace("boom")
}

func TestNewWithTrace_StackTrace(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		make  func() *ExtendedError
		check func(*testing.T, string)
	}{
		{
			name: "starts at the caller",
			make: func() *ExtendedError { return NewWithTrace("boom") },
			check: func(t *testing.T, stackTrace string) {
				// NewWithTrace's own frame, and the runtime's beneath it, are left out; the
				// function that asked for the error is what the trace starts at.
				firstLine, _, _ := strings.Cut(stackTrace, "\n")
				if !strings.HasSuffix(firstLine, ".TestNewWithTrace_StackTrace.func1()") {
					t.Fatalf("expected the caller as the topmost frame, got %q", firstLine)
				}
				if strings.Contains(stackTrace, "errors.NewWithTrace(") {
					t.Fatalf("expected NewWithTrace's own frame to be left out, got %q", stackTrace)
				}
			},
		},
		{
			name: "reports the frames beneath it",
			make: traceHelper,
			check: func(t *testing.T, stackTrace string) {
				if !strings.Contains(stackTrace, ".traceHelper()") {
					t.Fatalf("expected traceHelper's frame, got %q", stackTrace)
				}
				if !strings.Contains(stackTrace, ".TestNewWithTrace_StackTrace") {
					t.Fatalf("expected the calling test's frame, got %q", stackTrace)
				}
			},
		},
		{
			name: "names the file and line of each frame",
			make: func() *ExtendedError { return NewWithTrace("boom") },
			check: func(t *testing.T, stackTrace string) {
				if !strings.Contains(stackTrace, "errors_test.go:") {
					t.Fatalf("expected a file and line, got %q", stackTrace)
				}
			},
		},
		{
			name: "leaves out the runtime's own frames",
			make: func() *ExtendedError { return NewWithTrace("boom") },
			check: func(t *testing.T, stackTrace string) {
				// Every other line names a function; the ones between give its file and line.
				var functionLines []string
				for line := range strings.SplitSeq(stackTrace, "\n") {
					if !strings.HasPrefix(line, "\t") {
						functionLines = append(functionLines, line)
					}
				}

				if len(functionLines) == 0 {
					t.Fatalf("expected a frame, got %q", stackTrace)
				}

				if lastLine := functionLines[len(functionLines)-1]; strings.HasPrefix(lastLine, "runtime.") {
					t.Fatalf("expected no trailing runtime frame, got %q", lastLine)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			stackTrace := testCase.make().GetStackTrace()
			if stackTrace == "" {
				t.Fatal("expected a stack trace")
			}

			testCase.check(t, stackTrace)
		})
	}
}

func TestExtendedError_GetStackTrace(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		err      *ExtendedError
		expected string
	}{
		{
			name: "an error made without a trace has none",
			err:  New("boom"),
		},
		{
			name:     "a trace set by hand is answered with as it was set",
			err:      &ExtendedError{error: ErrSyntaxError, StackTrace: "set by hand"},
			expected: "set by hand",
		},
		{
			name: "a trace set by hand is preferred to a captured one",
			err: func() *ExtendedError {
				err := NewWithTrace("boom")
				err.StackTrace = "set by hand"
				return err
			}(),
			expected: "set by hand",
		},
		{
			name:     "the wrapped error's trace is answered with where there is none of its own",
			err:      New(&Error{Message: "boom", StackTrace: "wrapped"}),
			expected: "wrapped",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if stackTrace := testCase.err.GetStackTrace(); stackTrace != testCase.expected {
				t.Fatalf("expected %q, got %q", testCase.expected, stackTrace)
			}
		})
	}
}

func TestCapturePcs_DeepStack(t *testing.T) {
	t.Parallel()

	// A stack deeper than the frames capturePcs starts out with is recorded beyond them, up to the
	// bound a runaway recursion is held to.
	var recurse func(int) []uintptr
	recurse = func(n int) []uintptr {
		if n == 0 {
			return capturePcs(0)
		}
		return recurse(n - 1)
	}

	testCases := []struct {
		name  string
		depth int
		check func(*testing.T, []uintptr)
	}{
		{
			name:  "within the frames captured to begin with",
			depth: 8,
			check: func(t *testing.T, pcs []uintptr) {
				if len(pcs) < 8 {
					t.Fatalf("expected at least 8 frames, got %d", len(pcs))
				}
			},
		},
		{
			name:  "beyond them",
			depth: 200,
			check: func(t *testing.T, pcs []uintptr) {
				if len(pcs) <= 64 {
					t.Fatalf("expected more than the 64 frames captured to begin with, got %d", len(pcs))
				}
			},
		},
		{
			name:  "beyond the bound",
			depth: 800,
			check: func(t *testing.T, pcs []uintptr) {
				if len(pcs) != maxStackDepth {
					t.Fatalf("expected the stack to be held to %d frames, got %d", maxStackDepth, len(pcs))
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			testCase.check(t, recurse(testCase.depth))
		})
	}
}

func TestFormatFrames_NoPcs(t *testing.T) {
	t.Parallel()

	if stackTrace := formatFrames(nil); stackTrace != "" {
		t.Fatalf("expected no stack trace, got %q", stackTrace)
	}
}
