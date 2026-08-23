package errors

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"strconv"
	"strings"
)

var (
	ErrSyntaxError       = errors.New("syntax error")
	ErrSemanticError     = errors.New("semantic error")
	ErrParseError        = errors.New("parse error")
	ErrVerificationError = errors.New("verification error")
	ErrValidationError   = errors.New("validation error")
	ErrConversionNotOk   = errors.New("conversion not ok")
	ErrBadSplit          = errors.New("bad split")
	ErrNotInContext      = errors.New("not in context")
	ErrZeroValue         = errors.New("zero value")
	ErrNotInMap          = errors.New("not in map")
	ErrMapZeroValue      = errors.New("map zero value")
	ErrUnexpectedType    = errors.New("unexpected type")
	ErrUnauthorized      = errors.New("unauthorized")
)

func CollectWrappedErrors(err error) []error {
	results := []error{}

	queue := []error{err}
	isRoot := true

	for len(queue) > 0 {
		poppedErr := queue[0]
		queue = queue[1:]

		if poppedErr == nil {
			continue
		}

		if isRoot {
			isRoot = false
		} else {
			results = append(results, poppedErr)
		}

		switch typedErr := poppedErr.(type) { //nolint:errorlint // Manual unwrap traversal; every level is visited explicitly.
		case interface{ Unwrap() error }:
			unwrappedErr := typedErr.Unwrap()
			if unwrappedErr == nil {
				continue
			}

			queue = append(queue, unwrappedErr)
		case interface{ Unwrap() []error }:
			for _, unwrappedErr := range typedErr.Unwrap() {
				if unwrappedErr == nil {
					continue
				}

				queue = append(queue, unwrappedErr)
			}
		}
	}

	return results
}

// maxStackDepth bounds the frames a captured stack retains, so that a stack deep enough to be a
// runaway recursion is recorded to a fixed size rather than to its own.
const maxStackDepth = 512

// capturePcs records the calling goroutine's stack as program counters, skipping the given number
// of frames above the caller's own: skip 0 makes the caller the topmost frame, skip 1 its caller.
//
// The counters are what a stack costs at the point an error is made. Rendering them into the text a
// stack trace is read as is left to formatFrames, which runs where the trace is asked for -- at the
// point it is logged, if it is logged at all.
func capturePcs(skip int) []uintptr {
	pcs := make([]uintptr, 64)

	for {
		// Two frames above the requested one are this function's and runtime.Callers' own.
		n := runtime.Callers(skip+2, pcs)
		if n < len(pcs) || len(pcs) >= maxStackDepth {
			return pcs[:n]
		}

		// The stack filled what it was given and so may have more frames than were asked for.
		pcs = make([]uintptr, min(2*len(pcs), maxStackDepth))
	}
}

// formatFrames renders program counters as the text a stack trace is read as: a line naming each
// frame's function, and one giving the file and line it stands at.
func formatFrames(pcs []uintptr) string {
	if len(pcs) == 0 {
		return ""
	}

	var frames []runtime.Frame

	callersFrames := runtime.CallersFrames(pcs)
	for {
		frame, more := callersFrames.Next()
		if frame.Function != "" {
			frames = append(frames, frame)
		}

		if !more {
			break
		}
	}

	// A goroutine stands on the runtime's own frames -- runtime.main and runtime.goexit -- which
	// say nothing about where the error was made, and which runtime.Stack leaves out as well.
	for len(frames) > 0 && strings.HasPrefix(frames[len(frames)-1].Function, "runtime.") {
		frames = frames[:len(frames)-1]
	}

	var builder strings.Builder
	for _, frame := range frames {
		builder.WriteString(frame.Function)
		builder.WriteString("()\n\t")
		builder.WriteString(frame.File)
		builder.WriteByte(':')
		builder.WriteString(strconv.Itoa(frame.Line))
		builder.WriteByte('\n')
	}

	return strings.TrimSpace(builder.String())
}

// CaptureStackTrace returns the calling goroutine's stack as text, the caller being the topmost
// frame.
func CaptureStackTrace() string {
	return formatFrames(capturePcs(1))
}

type CodeErrorI interface {
	Error() string
	GetCode() string
}

type IdErrorI interface {
	Error() string
	GetId() string
}

type StackTraceErrorI interface {
	Error() string
	GetStackTrace() string
}

type CauseErrorI interface {
	Error() string
	GetCause() error
	Unwrap() error
}

type InputErrorI interface {
	Error() string
	GetInput() any
}

type ContextErrorI interface {
	Error() string
	GetContext() *context.Context
}

type Error struct {
	Message    string
	Cause      error
	Input      any
	Code       string
	Id         string
	StackTrace string
}

func (err *Error) Error() string {
	return err.Message
}

func (err *Error) GetCause() error {
	return err.Cause
}

func (err *Error) GetInput() any {
	return err.Input
}

func (err *Error) GetCode() string {
	return err.Code
}

func (err *Error) GetId() string {
	return err.Id
}

func (err *Error) GetStackTrace() string {
	return err.StackTrace
}

func (err *Error) Unwrap() error {
	return err.Cause
}

func (err *Error) Is(target error) bool {
	_, ok := target.(*Error)
	return ok
}

type ExtendedError struct {
	error
	Input      any
	Code       string
	Id         string
	StackTrace string
	Context    *context.Context

	// pcs is the stack NewWithTrace captured, held as program counters until GetStackTrace is
	// asked for the text they render as. StackTrace, set by hand, is answered with ahead of it.
	pcs []uintptr
}

func (err *ExtendedError) Error() string {
	if err.error == nil {
		return ""
	}
	return err.error.Error()
}

func (err *ExtendedError) GetInput() any {
	if input := err.Input; input != nil {
		return input
	}

	includedErr := err.error
	if includedErr == nil {
		return nil
	}

	if inputError, ok := includedErr.(InputErrorI); ok { //nolint:errorlint // Deliberate: only the immediate error is consulted.
		return inputError.GetInput()
	}

	return nil
}

func (err *ExtendedError) GetCode() string {
	if code := err.Code; code != "" {
		return err.Code
	}

	includedErr := err.error
	if includedErr == nil {
		return ""
	}

	if codeError, ok := includedErr.(CodeErrorI); ok { //nolint:errorlint // Deliberate: only the immediate error is consulted.
		return codeError.GetCode()
	}

	return ""
}

func (err *ExtendedError) GetId() string {
	if id := err.Id; id != "" {
		return err.Id
	}

	includedErr := err.error
	if includedErr == nil {
		return ""
	}

	if idError, ok := includedErr.(IdErrorI); ok { //nolint:errorlint // Deliberate: only the immediate error is consulted.
		return idError.GetId()
	}

	return ""
}

func (err *ExtendedError) GetStackTrace() string {
	if stackTrace := err.StackTrace; stackTrace != "" {
		return stackTrace
	}

	if len(err.pcs) != 0 {
		return formatFrames(err.pcs)
	}

	includedErr := err.error
	if includedErr == nil {
		return ""
	}

	if stackTraceError, ok := includedErr.(StackTraceErrorI); ok { //nolint:errorlint // Deliberate: only the immediate error is consulted.
		return stackTraceError.GetStackTrace()
	}

	return ""
}

func (err *ExtendedError) GetContext() *context.Context {
	if contextPtr := err.Context; contextPtr != nil {
		return contextPtr
	}

	includedErr := err.error
	if includedErr == nil {
		return nil
	}

	if contextError, ok := errors.AsType[ContextErrorI](includedErr); ok {
		return contextError.GetContext()
	}

	return nil
}

func (err *ExtendedError) Unwrap() []error {
	switch typedErr := err.error.(type) { //nolint:errorlint // Unwrap must expose the direct error's own Unwrap, not search the chain.
	case interface{ Unwrap() error }:
		return []error{typedErr.Unwrap()}
	case interface{ Unwrap() []error }:
		return typedErr.Unwrap()
	}

	return nil
}

func (err *ExtendedError) Is(target error) bool {
	return errors.Is(err.error, target)
}

func (err *ExtendedError) As(target any) bool {
	if err.error == nil {
		return false
	}

	return errors.As(err.error, target)
}

func New(e any, input ...any) *ExtendedError {
	var err error

	// Expecting `e` to be an `error` or a string. If not, make it a string.
	switch typedE := e.(type) {
	case error:
		err = typedE
	case string:
		err = errors.New(typedE) //nolint:err113 // Constructor whose purpose is making errors from arbitrary values.
	case nil:
		break
	default:
		err = fmt.Errorf("%v", typedE) //nolint:err113 // Constructor whose purpose is making errors from arbitrary values.
	}

	var errInput any = input
	if len(input) == 0 {
		errInput = nil
	}
	if len(input) == 1 {
		errInput = input[0]
	}

	return &ExtendedError{error: err, Input: errInput}
}

func NewCtx(ctx context.Context, e any, input ...any) *ExtendedError {
	extendedErr := New(e, input...)
	extendedErr.Context = &ctx

	return extendedErr
}

func NewWithTrace(e any, input ...any) *ExtendedError {
	extendedErr := New(e, input...)
	extendedErr.pcs = capturePcs(1)

	return extendedErr
}

func NewWithTraceCtx(ctx context.Context, e any, input ...any) *ExtendedError {
	extendedErr := NewWithTrace(e, input...)
	extendedErr.Context = &ctx

	return extendedErr
}

func IsAny(err error, targets ...error) bool {
	for _, target := range targets {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func IsAll(err error, targets ...error) bool {
	for _, target := range targets {
		if !errors.Is(err, target) {
			return false
		}
	}
	return true
}

func IsClosedError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	return errors.Is(err, io.EOF) ||
		errors.Is(err, net.ErrClosed) ||
		strings.HasSuffix(errMsg, "write: broken pipe") ||
		strings.HasSuffix(errMsg, "use of closed network connection")
}
