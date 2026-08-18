package journal

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

var errTest = errors.New("test error")

func TestPriorityValuesMatchSyslog(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		priority Priority
		expected int
	}{
		{name: "emergency", priority: PriorityEmergency, expected: 0},
		{name: "alert", priority: PriorityAlert, expected: 1},
		{name: "critical", priority: PriorityCritical, expected: 2},
		{name: "error", priority: PriorityError, expected: 3},
		{name: "warning", priority: PriorityWarning, expected: 4},
		{name: "notice", priority: PriorityNotice, expected: 5},
		{name: "info", priority: PriorityInfo, expected: 6},
		{name: "debug", priority: PriorityDebug, expected: 7},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if int(testCase.priority) != testCase.expected {
				t.Errorf("expected %d, got %d", testCase.expected, int(testCase.priority))
			}
		})
	}
}

func TestValidateFieldName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		fieldName string
		expectErr bool
	}{
		{name: "simple name", fieldName: "MESSAGE", expectErr: false},
		{name: "name with an underscore inside", fieldName: "SYSLOG_IDENTIFIER", expectErr: false},
		{name: "name with digits", fieldName: "FIELD2", expectErr: false},
		{name: "digits only", fieldName: "2", expectErr: false},
		{name: "empty name", fieldName: "", expectErr: true},
		{name: "leading underscore is reserved for journald", fieldName: "_PID", expectErr: true},
		{name: "lowercase is rejected", fieldName: "message", expectErr: true},
		{name: "hyphen is rejected", fieldName: "MY-FIELD", expectErr: true},
		{name: "equals sign is rejected", fieldName: "MY=FIELD", expectErr: true},
		{name: "newline is rejected", fieldName: "MY\nFIELD", expectErr: true},
		{name: "space is rejected", fieldName: "MY FIELD", expectErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := validateFieldName(testCase.fieldName)

			if testCase.expectErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if !errors.Is(err, altshiftErrors.ErrValidationError) {
					t.Errorf("expected a validation error, got %v", err)
				}
				return
			}

			if err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestAppendField(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		fieldName string
		value     string
		expected  string
		expectErr bool
	}{
		{
			name:      "a value without a newline is written inline",
			fieldName: "MESSAGE",
			value:     "hello",
			expected:  "MESSAGE=hello\n",
		},
		{
			name:      "an empty value is still written inline",
			fieldName: "MESSAGE",
			value:     "",
			expected:  "MESSAGE=\n",
		},
		{
			name:      "a value containing an equals sign needs no escaping",
			fieldName: "MESSAGE",
			value:     "a=b",
			expected:  "MESSAGE=a=b\n",
		},
		{
			name:      "an invalid field name is rejected",
			fieldName: "_PID",
			value:     "1",
			expectErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			buffer := new(bytes.Buffer)
			err := appendField(buffer, testCase.fieldName, testCase.value)

			if testCase.expectErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if got := buffer.String(); got != testCase.expected {
				t.Errorf("expected %q, got %q", testCase.expected, got)
			}
		})
	}
}

func TestAppendFieldUsesTheLengthPrefixedFormForNewlines(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		value string
	}{
		{name: "two lines", value: "line one\nline two"},
		{name: "trailing newline", value: "line one\n"},
		{name: "leading newline", value: "\nline one"},
		{name: "only a newline", value: "\n"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			buffer := new(bytes.Buffer)
			if err := appendField(buffer, "MULTILINE", testCase.value); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			// name "\n" | uint64 little-endian length | value "\n"
			prefix := "MULTILINE\n"
			raw := buffer.Bytes()

			if !bytes.HasPrefix(raw, []byte(prefix)) {
				t.Fatalf("expected the field name on its own line, got %q", raw)
			}

			rest := raw[len(prefix):]
			if len(rest) < 8 {
				t.Fatalf("expected a length prefix, got %q", rest)
			}

			length := binary.LittleEndian.Uint64(rest[:8])
			if length != uint64(len(testCase.value)) {
				t.Errorf("expected length %d, got %d", len(testCase.value), length)
			}

			payload := rest[8:]
			expectedPayload := testCase.value + "\n"
			if string(payload) != expectedPayload {
				t.Errorf("expected payload %q, got %q", expectedPayload, payload)
			}
		})
	}
}

func TestIsSocketSpaceError(t *testing.T) {
	t.Parallel()

	wrap := func(err error) error {
		return &net.OpError{Op: "write", Net: "unixgram", Err: os.NewSyscallError("sendmsg", err)}
	}

	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil", err: nil, expected: false},
		{name: "message too large", err: wrap(syscall.EMSGSIZE), expected: true},
		{name: "no buffer space", err: wrap(syscall.ENOBUFS), expected: true},
		{name: "connection refused is not a space error", err: wrap(syscall.ECONNREFUSED), expected: false},
		{name: "a plain error is not a space error", err: errTest, expected: false},
		{name: "an op error without a syscall error", err: &net.OpError{Op: "write", Err: errTest}, expected: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := isSocketSpaceError(testCase.err); got != testCase.expected {
				t.Errorf("expected %v, got %v", testCase.expected, got)
			}
		})
	}
}

func TestSendRejectsAnInvalidFieldName(t *testing.T) {
	t.Parallel()

	if !Enabled() {
		t.Skip("no local journal to write to")
	}

	if err := Send("message", PriorityInfo, map[string]string{"_RESERVED": "value"}); err == nil {
		t.Error("expected a reserved field name to be rejected")
	}
}

func TestSendWritesEntries(t *testing.T) {
	t.Parallel()

	if !Enabled() {
		t.Skip("no local journal to write to")
	}

	testCases := []struct {
		name     string
		message  string
		priority Priority
		value    string
	}{
		{name: "short entry", message: "short", priority: PriorityInfo, value: "value"},
		{name: "entry with a multiline field", message: "multiline", priority: PriorityWarning, value: "one\ntwo"},
		// Larger than the socket buffer, so the descriptor-passing path runs.
		{name: "entry too large for the socket", message: "large", priority: PriorityError, value: strings.Repeat("x", 512*1024)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := Send(
				fmt.Sprintf("utils_go journal test: %s", testCase.message),
				testCase.priority,
				map[string]string{"SYSLOG_IDENTIFIER": "utils_go_journal_test", "TEST_FIELD": testCase.value},
			)
			if err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}
