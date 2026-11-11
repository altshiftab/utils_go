package log

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// LevelFromEnvironment is the log level the environment variable name holds, read as slog reads a
// level -- DEBUG, INFO, WARN or ERROR, in any case, with an offset such as INFO+2 -- and fallback
// where the variable is unset or empty. A value slog cannot read is an error rather than a level
// guessed at: a service told to log at a level it does not know is misconfigured, and is better
// stopped at start than left logging at one nobody chose.
func LevelFromEnvironment(name string, fallback slog.Level) (slog.Level, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(value)); err != nil {
		return fallback, altshiftErrors.NewWithTrace(fmt.Errorf("%w: level unmarshal text: %w", altshiftErrors.ErrParseError, err), name, value)
	}

	return level, nil
}
