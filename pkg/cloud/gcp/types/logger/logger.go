package logger

import (
	"io"
	"log/slog"

	altshiftLogHandler "github.com/altshiftab/utils_go/pkg/log/handler"
)

func ReplaceAttr(groups []string, attr slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return attr
	}

	switch attr.Key {
	case slog.TimeKey:
		attr.Key = "time"
	case slog.LevelKey:
		attr.Key = "severity"
	case slog.MessageKey:
		attr.Key = "message"
	case slog.SourceKey:
		if source, ok := attr.Value.Any().(*slog.Source); ok {
			return slog.Group(
				"logging.googleapis.com/sourceLocation",
				"file", source.File,
				"line", source.Line,
				"function", source.Function,
			)
		}
	}

	return attr
}

func New(level slog.Leveler, writer io.Writer) *slog.Logger {
	return slog.New(
		altshiftLogHandler.New(
			slog.NewJSONHandler(
				writer,
				&slog.HandlerOptions{AddSource: true, Level: level, ReplaceAttr: ReplaceAttr},
			),
		),
	)
}
