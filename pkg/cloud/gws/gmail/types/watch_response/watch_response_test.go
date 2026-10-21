package watch_response

import (
	"testing"
	"time"
)

// TestExpiresAt covers the conversion every caller of Watch would otherwise
// work out for itself, from a field that names neither its unit nor its epoch.
func TestExpiresAt(t *testing.T) {
	testCases := []struct {
		name       string
		expiration string
		expected   time.Time
		wantErr    bool
	}{
		{
			// Google's own documented example.
			name:       "milliseconds since the epoch",
			expiration: "1431990098200",
			expected:   time.UnixMilli(1431990098200).UTC(),
		},
		{
			// Not seconds: read as seconds this would be in 1970, and a watch
			// in the past is renewed on every tick for ever.
			name:       "a recent expiry is in this century",
			expiration: "1790000000000",
			expected:   time.UnixMilli(1790000000000).UTC(),
		},
		{
			// Gmail omits it from a response that carried no watch. Zero is due,
			// which is the answer a caller wants.
			name:       "absent",
			expiration: "",
			expected:   time.Time{},
		},
		{name: "not a number", expiration: "soon", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			actual, err := (&WatchResponse{Expiration: testCase.expiration}).ExpiresAt()

			if testCase.wantErr {
				if err == nil {
					t.Fatalf("%s: expected an error", testCase.name)
				}

				return
			}

			if err != nil {
				t.Fatalf("%s: unexpected error: %v", testCase.name, err)
			}
			if !actual.Equal(testCase.expected) {
				t.Errorf("%s: got %v, want %v", testCase.name, actual, testCase.expected)
			}
		})
	}
}

func TestExpiresAtOfNothing(t *testing.T) {
	t.Parallel()

	// A nil receiver rather than a panic: a caller that did not check the
	// response is the caller most likely to reach for this.
	actual, err := (*WatchResponse)(nil).ExpiresAt()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !actual.IsZero() {
		t.Errorf("got %v, want the zero time", actual)
	}
}
