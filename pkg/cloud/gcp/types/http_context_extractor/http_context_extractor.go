package http_context_extractor

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/http_context_extractor/http_context_extractor_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/log_entry"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	motmedelHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	motmedelJson "github.com/altshiftab/utils_go/pkg/json"
	motmedelLog "github.com/altshiftab/utils_go/pkg/log"
)

type Extractor struct {
	ProjectId string
}

func (e *Extractor) Handle(ctx context.Context, record *slog.Record) error {
	if record == nil {
		return nil
	}

	if httpContext, ok := ctx.Value(motmedelHttpContext.HttpContextContextKey).(*motmedelHttpTypes.HttpContext); ok && httpContext != nil {
		if logEntry := log_entry.ParseHttp(httpContext.Request, httpContext.Response); logEntry != nil {
			if projectId := e.ProjectId; projectId != "" {
				if traceId := logEntry.TraceId; traceId != "" {
					logEntry.Trace = fmt.Sprintf("projects/%s/traces/%s", projectId, logEntry.TraceId)
				}
			}

			logEntryMap, err := motmedelJson.ObjectToMap(logEntry)
			if err != nil {
				return motmedelErrors.New(fmt.Errorf("object to map: %w", err), logEntry)
			}

			record.Add(motmedelLog.AttrsFromMap(logEntryMap)...)
		}
	}

	return nil
}

func New(options ...http_context_extractor_config.Option) *Extractor {
	return &Extractor{ProjectId: http_context_extractor_config.New(options...).ProjectId}
}
