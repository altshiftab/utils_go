package context_logger

import (
	motmedelLog "github.com/altshiftab/utils_go/pkg/log"
	motmedelLogHandler "github.com/altshiftab/utils_go/pkg/log/handler"
	"log/slog"
)

func New(handler slog.Handler, extractors ...motmedelLog.ContextExtractor) *slog.Logger {
	return slog.New(&motmedelLog.ContextHandler{Next: motmedelLogHandler.New(handler), Extractors: extractors})
}
