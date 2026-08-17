package log_entry

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/log_entry/http_request"
)

type LogEntry struct {
	HttpRequest  *http_request.Request `json:"httpRequest,omitempty"`
	Trace        string                `json:"logging.googleapis.com/trace,omitempty"`
	TraceId      string                `json:"-"`
	SpanId       string                `json:"logging.googleapis.com/spanId,omitempty"`
	TraceSampled *bool                 `json:"logging.googleapis.com/trace_sampled,omitempty"`
}

// parseXCloudTraceContext parses the X-Cloud-Trace-Context header.
// Header format: TRACE_ID/SPAN_ID;o=TRACE_TRUE
// Example: 105445aa7843bc8bf206b120001000/123;o=1
// It returns traceID (32-char hex), spanID (16-hex-digit lowercase), and sampled flag.
func parseXCloudTraceContext(h string) (string, string, *bool) {
	if h == "" {
		return "", "", nil
	}

	// Split TRACE_ID from the rest (SPAN_ID;o=TRACE_TRUE).
	traceID, rest, _ := strings.Cut(h, "/")

	// rest is like: SPAN_ID;o=1
	spanID, flag, _ := strings.Cut(rest, ";")

	// Convert decimal SPAN_ID to 16-hex-digit lowercase as required by Cloud Logging
	var spanIDHex string
	if n, err := strconv.ParseUint(spanID, 10, 64); err == nil {
		spanIDHex = fmt.Sprintf("%016x", n)
	}

	var sampled *bool
	if strings.HasPrefix(flag, "o=") {
		v := flag == "o=1"
		sampled = &v
	}

	return traceID, spanIDHex, sampled
}

func ParseHttp(request *http.Request, response *http.Response) *LogEntry {
	if request == nil && response == nil {
		return nil
	}

	var httpRequest http_request.Request
	var traceId string
	var spanId string
	var sampled *bool

	if request != nil {
		httpRequest.RequestMethod = request.Method
		httpRequest.UserAgent = request.UserAgent()
		httpRequest.RemoteIp = request.RemoteAddr
		httpRequest.Referer = request.Referer()
		httpRequest.Protocol = fmt.Sprintf("HTTP/%d.%d", request.ProtoMajor, request.ProtoMinor)

		if requestHeader := request.Header; requestHeader != nil {
			traceId, spanId, sampled = parseXCloudTraceContext(requestHeader.Get("X-Cloud-Trace-Context"))
		}
	}

	if response != nil {
		httpRequest.Status = response.StatusCode
	}

	return &LogEntry{HttpRequest: &httpRequest, TraceId: traceId, SpanId: spanId, TraceSampled: sampled}
}
