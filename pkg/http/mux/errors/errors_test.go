package errors

import (
	"errors"
	"testing"
)

func TestSentinelErrorMessages(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  error
		want string
	}{
		{name: "ErrNoResponseWritten", err: ErrNoResponseWritten, want: "no response was written"},
		{name: "ErrNoResponseWriterFlusher", err: ErrNoResponseWriterFlusher, want: "no response writer flusher"},
		{name: "ErrTransferEncodingAlreadySet", err: ErrTransferEncodingAlreadySet, want: "transfer encoding already set"},
		{name: "ErrUnusableMuxSpecification", err: ErrUnusableMuxSpecification, want: "unusable mux specification"},
		{name: "ErrCouldNotObtainHttpContext", err: ErrCouldNotObtainHttpContext, want: "could not obtain http context"},
		{name: "ErrContentEncodingToDataNotOk", err: ErrContentEncodingToDataNotOk, want: "content encoding to data not ok"},
		{name: "ErrUnusableResponseError", err: ErrUnusableResponseError, want: "unusable response error"},
		{
			name: "ErrMultipleResponseErrorErrors",
			err:  ErrMultipleResponseErrorErrors,
			want: "unusable mux specification: multiple response error errors",
		},
		{
			name: "ErrUnexpectedResponseErrorType",
			err:  ErrUnexpectedResponseErrorType,
			want: "unusable response error: unexpected response error type",
		},
		{name: "ErrUnexpectedContentEncoding", err: ErrUnexpectedContentEncoding, want: "unexpected content encoding"},
		{name: "ErrUnsupportedFileExtension", err: ErrUnsupportedFileExtension, want: "unsupported file extension"},
		{name: "ErrMisdirectedRequest", err: ErrMisdirectedRequest, want: "misdirected request"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if testCase.err == nil {
				t.Fatal("error is nil")
			}
			if got := testCase.err.Error(); got != testCase.want {
				t.Errorf("Error() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestWrappedSentinelIdentity(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		err    error
		target error
	}{
		{
			name:   "multiple response error errors wraps unusable mux specification",
			err:    ErrMultipleResponseErrorErrors,
			target: ErrUnusableMuxSpecification,
		},
		{
			name:   "unexpected response error type wraps unusable response error",
			err:    ErrUnexpectedResponseErrorType,
			target: ErrUnusableResponseError,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if !errors.Is(testCase.err, testCase.target) {
				t.Errorf("errors.Is(%v, %v) = false, want true", testCase.err, testCase.target)
			}
		})
	}
}

func TestSentinelErrorsAreDistinct(t *testing.T) {
	t.Parallel()

	// ErrUnexpectedResponseErrorType wraps ErrUnusableResponseError but must not
	// be conflated with the unrelated ErrUnusableMuxSpecification chain.
	if errors.Is(ErrUnexpectedResponseErrorType, ErrUnusableMuxSpecification) {
		t.Error("ErrUnexpectedResponseErrorType should not match ErrUnusableMuxSpecification")
	}
	if errors.Is(ErrMultipleResponseErrorErrors, ErrUnusableResponseError) {
		t.Error("ErrMultipleResponseErrorErrors should not match ErrUnusableResponseError")
	}
}
