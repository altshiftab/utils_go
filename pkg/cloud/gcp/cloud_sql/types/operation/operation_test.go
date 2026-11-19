package operation

import (
	"testing"
)

func TestDoneAndFailure(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		operation   *Operation
		wantDone    bool
		wantFailure string
	}{
		{name: "nil", operation: nil},
		{name: "running", operation: &Operation{Status: StatusRunning}},
		{name: "done", operation: &Operation{Status: StatusDone}, wantDone: true},
		{
			name: "failed",
			operation: &Operation{Status: StatusDone, Error: &OperationErrors{Errors: []*OperationError{
				{Code: "ERROR_RDBMS", Message: "pg_dump failed"},
				nil,
				{Code: "INTERNAL_ERROR", Message: "again"},
			}}},
			wantDone:    true,
			wantFailure: "ERROR_RDBMS: pg_dump failed; INTERNAL_ERROR: again",
		},
		{name: "an empty error list", operation: &Operation{Status: StatusDone, Error: &OperationErrors{}}, wantDone: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.operation.Done(); got != testCase.wantDone {
				t.Errorf("done %t, want %t", got, testCase.wantDone)
			}
			if got := testCase.operation.Failure(); got != testCase.wantFailure {
				t.Errorf("failure %q, want %q", got, testCase.wantFailure)
			}
		})
	}
}
