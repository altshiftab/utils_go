package numeric_date

import (
	"encoding/json/v2"
	"errors"
	"testing"
	"time"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func TestNewFromSeconds(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		seconds  float64
		expected time.Time
	}{
		{
			name:     "zero",
			seconds:  0,
			expected: time.Unix(0, 0),
		},
		{
			name:     "positive integer",
			seconds:  1609459200, // 2021-01-01 00:00:00 UTC
			expected: time.Unix(1609459200, 0),
		},
		{
			name:     "positive with fraction",
			seconds:  1609459200.5,
			expected: time.Unix(1609459200, 500000000),
		},
		{
			name:     "negative integer",
			seconds:  -86400, // 1969-12-31 00:00:00 UTC
			expected: time.Unix(-86400, 0),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			date := NewFromSeconds(tc.seconds)

			// Compare truncated times since TimePrecision is applied
			expectedTruncated := tc.expected.Truncate(TimePrecision)
			if !date.Equal(expectedTruncated) {
				t.Fatalf("expected %v, got %v", expectedTruncated, date.Time)
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	now := time.Now()
	date := New(now)

	expected := now.Truncate(TimePrecision)
	if !date.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, date.Time)
	}
}

func TestDate_MarshalJSON(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		date     Date
		expected string
	}{
		{
			name:     "zero time",
			date:     Date{time.Unix(0, 0)},
			expected: "0",
		},
		{
			name:     "positive timestamp",
			date:     Date{time.Unix(1609459200, 0)},
			expected: "1609459200",
		},
		{
			name:     "negative timestamp",
			date:     Date{time.Unix(-86400, 0)},
			expected: "-86400",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, err := tc.date.MarshalJSON()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if string(b) != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, string(b))
			}
		})
	}
}

func TestDate_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       string
		expected    time.Time
		expectError bool
	}{
		{
			name:     "zero",
			input:    "0",
			expected: time.Unix(0, 0),
		},
		{
			name:     "positive integer",
			input:    "1609459200",
			expected: time.Unix(1609459200, 0),
		},
		{
			name:     "positive float",
			input:    "1609459200.5",
			expected: time.Unix(1609459200, 500000000),
		},
		{
			name:     "negative integer",
			input:    "-86400",
			expected: time.Unix(-86400, 0),
		},
		{
			name:        "invalid json",
			input:       "invalid",
			expectError: true,
		},
		{
			name:        "string value",
			input:       `"not a number"`,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var date Date
			err := date.UnmarshalJSON([]byte(tc.input))

			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				expectedTruncated := tc.expected.Truncate(TimePrecision)
				if !date.Equal(expectedTruncated) {
					t.Fatalf("expected %v, got %v", expectedTruncated, date.Time)
				}
			}
		})
	}
}

func TestConvert(t *testing.T) {
	t.Parallel()

	testDate := New(time.Unix(1609459200, 0))

	testCases := []struct {
		name        string
		input       any
		expected    *Date
		expectError bool
		errorCheck  func(error) bool
	}{
		{
			name:     "*Date input",
			input:    testDate,
			expected: testDate,
		},
		{
			name:     "Date input",
			input:    *testDate,
			expected: testDate,
		},
		{
			name:     "float64 input",
			input:    float64(1609459200),
			expected: New(time.Unix(1609459200, 0)),
		},
		{
			name:     "float64 zero returns nil",
			input:    float64(0),
			expected: nil,
		},
		{
			name:        "unsupported type",
			input:       "string",
			expectError: true,
			errorCheck: func(err error) bool {
				return errors.Is(err, motmedelErrors.ErrUnexpectedType)
			},
		},
		{
			name:        "int type (unsupported)",
			input:       1609459200,
			expectError: true,
			errorCheck: func(err error) bool {
				return errors.Is(err, motmedelErrors.ErrUnexpectedType)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := Convert(tc.input)

			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tc.errorCheck != nil && !tc.errorCheck(err) {
					t.Fatalf("error check failed for error: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if tc.expected == nil {
					if result != nil {
						t.Fatalf("expected nil, got %v", result)
					}
				} else {
					if result == nil {
						t.Fatal("expected non-nil result, got nil")
					}
					if !result.Equal(tc.expected.Time) {
						t.Fatalf("expected %v, got %v", tc.expected.Time, result.Time)
					}
				}
			}
		})
	}
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	t.Parallel()

	original := New(time.Unix(1609459200, 0))

	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Date
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if !original.Equal(decoded.Time) {
		t.Fatalf("round trip failed: original %v, decoded %v", original.Time, decoded.Time)
	}
}
