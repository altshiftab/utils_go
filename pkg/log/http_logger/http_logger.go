// Package http_logger makes the logger an HTTP service logs through: JSON entries carrying what is
// known about the request being logged, which the service's own logging calls need say nothing
// about.
package http_logger

import (
	"log/slog"
	"runtime/debug"

	gcpHttpContextExtractor "github.com/altshiftab/utils_go/pkg/cloud/gcp/types/http_context_extractor"
	gcpLogger "github.com/altshiftab/utils_go/pkg/cloud/gcp/types/logger"
	altshiftLog "github.com/altshiftab/utils_go/pkg/log"
	altshiftContextLogger "github.com/altshiftab/utils_go/pkg/log/context_logger"
	"github.com/altshiftab/utils_go/pkg/log/entry_size_guard"
	altshiftErrorLogger "github.com/altshiftab/utils_go/pkg/log/error_logger"
	"github.com/altshiftab/utils_go/pkg/log/http_logger/http_logger_config"
)

// buildSettingKeys are the build settings labelled onto every entry: which revision the service was
// built from, and when. What a log says happened is worth little without knowing what was running.
var buildSettingKeys = []string{"vcs.revision", "vcs.time"}

func New(options ...http_logger_config.Option) *altshiftErrorLogger.Logger {
	config := http_logger_config.New(options...)

	writer := config.Writer
	handlerOptions := &slog.HandlerOptions{Level: config.LogLevel}

	httpContextExtractor := config.HttpContextExtractor

	errorContextExtractor := &altshiftLog.ErrorContextExtractor{}
	if httpContextExtractor != nil {
		errorContextExtractor.ContextExtractors = []altshiftLog.ContextExtractor{httpContextExtractor}
	}

	extractors := []altshiftLog.ContextExtractor{errorContextExtractor}
	if httpContextExtractor != nil {
		extractors = append(extractors, httpContextExtractor)
	}

	if config.Gcp {
		// Cloud Logging reads the severity and the message under names of its own, and rejects an
		// entry above its size limit outright -- the entry vanishes rather than being truncated --
		// so oversized ones are reduced before they are written.
		handlerOptions.ReplaceAttr = gcpLogger.ReplaceAttr
		writer = entry_size_guard.New(writer)

		gcpExtractor := config.GcpHttpContextExtractor
		if gcpExtractor == nil {
			gcpExtractor = gcpHttpContextExtractor.New()
		}

		extractors = append(extractors, gcpExtractor)
	}

	slogger := altshiftContextLogger.New(slog.NewJSONHandler(writer, handlerOptions), extractors...)

	if buildInfo, ok := debug.ReadBuildInfo(); ok && buildInfo != nil {
		var labelAttrs []any
		for _, buildSetting := range buildInfo.Settings {
			for _, key := range buildSettingKeys {
				if buildSetting.Key == key {
					labelAttrs = append(labelAttrs, slog.String(key, buildSetting.Value))
				}
			}
		}

		if len(labelAttrs) > 0 {
			slogger = slogger.With(slog.Group("labels", labelAttrs...))
		}
	}

	return &altshiftErrorLogger.Logger{Logger: slogger}
}
