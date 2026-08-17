package log

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"log/slog"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelLog "github.com/altshiftab/utils_go/pkg/log"
	"github.com/altshiftab/utils_go/pkg/schema"
	schemaUtils "github.com/altshiftab/utils_go/pkg/schema/utils"
	motmedelTlsContext "github.com/altshiftab/utils_go/pkg/tls/context"
	motmedelTlsTypes "github.com/altshiftab/utils_go/pkg/tls/types"
)

func ParseTlsContext(tlsContext *motmedelTlsTypes.TlsContext) *schema.Base {
	if tlsContext == nil {
		return nil
	}

	connectionState := tlsContext.ConnectionState
	if connectionState == nil {
		return nil
	}

	var base schema.Base
	schemaUtils.EnrichWithTlsContext(&base, tlsContext)

	return &base
}

func ExtractTlsContext(ctx context.Context, record *slog.Record) error {
	if dnsContext, ok := ctx.Value(motmedelTlsContext.TlsContextKey).(*motmedelTlsTypes.TlsContext); ok && dnsContext != nil {
		base := ParseTlsContext(dnsContext)
		if base != nil {
			baseBytes, err := json.Marshal(base)
			if err != nil {
				return motmedelErrors.NewWithTrace(fmt.Errorf("json marshal (ecs base): %w", err), base)
			}

			var baseMap map[string]any
			if err = json.Unmarshal(baseBytes, &baseMap); err != nil {
				return motmedelErrors.NewWithTrace(fmt.Errorf("json unmarshal (ecs base map): %w", err), baseMap)
			}

			record.Add(motmedelLog.AttrsFromMap(baseMap)...)
		}
	}

	return nil
}

var TlsContextExtractor = motmedelLog.ContextExtractorFunction(ExtractTlsContext)
