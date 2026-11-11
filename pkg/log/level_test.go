package log

import (
	"errors"
	"log/slog"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// The level is read as slog reads one, the fallback stands in for a value that is not there, and a
// value slog cannot read is refused rather than guessed at.
//
//nolint:paralleltest // Sets an environment variable, which t.Setenv forbids in a parallel test.
func TestLevelFromEnvironment(t *testing.T) {
	const name = "UTILS_GO_TEST_LOG_LEVEL"

	testCases := []struct {
		name      string
		value     string
		set       bool
		want      slog.Level
		wantError bool
	}{
		{name: "unset", want: slog.LevelInfo},
		{name: "empty", set: true, value: " ", want: slog.LevelInfo},
		{name: "debug", set: true, value: "DEBUG", want: slog.LevelDebug},
		{name: "in another case", set: true, value: "warn", want: slog.LevelWarn},
		{name: "with an offset", set: true, value: "INFO+2", want: slog.LevelInfo + 2},
		{name: "a level there is none of", set: true, value: "VERBOSE", want: slog.LevelInfo, wantError: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.set {
				t.Setenv(name, testCase.value)
			}

			got, err := LevelFromEnvironment(name, slog.LevelInfo)
			if got != testCase.want {
				t.Errorf("got %s, want %s", got, testCase.want)
			}
			if (err != nil) != testCase.wantError {
				t.Errorf("got the error %v, want one: %t", err, testCase.wantError)
			}
			if err != nil && !errors.Is(err, altshiftErrors.ErrParseError) {
				t.Errorf("the error %v is not a parse error", err)
			}
		})
	}
}
