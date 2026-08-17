package context_logger

import (
	altshiftLog "github.com/altshiftab/utils_go/pkg/log"
	altshiftLogHandler "github.com/altshiftab/utils_go/pkg/log/handler"
	"log/slog"
)

func New(handler slog.Handler, extractors ...altshiftLog.ContextExtractor) *slog.Logger {
	return slog.New(&altshiftLog.ContextHandler{Next: altshiftLogHandler.New(handler), Extractors: extractors})
}
