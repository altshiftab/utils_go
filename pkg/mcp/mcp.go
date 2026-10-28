// Package mcp is a Model Context Protocol server over Streamable HTTP, as much of one as a server of
// tools needs: every message a POST, every answer one JSON object, no stream, no session.
//
// It speaks both eras of the protocol on the one endpoint. A request carrying its protocol version
// in its _meta is of 2026-07-28 -- stateless, every request saying who is asking and how, its
// headers repeating its method and tool -- and is answered so. Anything else is of the eras before,
// which open with initialize; with no session to remember what it settled, each request after is
// answered on what it says itself.
package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

const (
	// ModernVersion is the revision served statelessly, its version in every request.
	ModernVersion = "2026-07-28"
	// LatestLegacyVersion is what an initialize asking for a version not served is offered.
	LatestLegacyVersion = "2025-11-25"
)

// legacyVersions are the revisions before ModernVersion that are served; 2025-03-26 is what a
// request naming none is taken to be.
var legacyVersions = []string{LatestLegacyVersion, "2025-06-18", "2025-03-26"}

const (
	metaProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	metaClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
	metaServerInfo         = "io.modelcontextprotocol/serverInfo"

	methodToolsCall = "tools/call"
	keyTools        = "tools"

	headerProtocolVersion = "MCP-Protocol-Version"
	headerMethod          = "Mcp-Method"
	headerName            = "Mcp-Name"
)

// JSON-RPC's error codes, and those MCP reserves.
const (
	CodeParseError                 = -32700
	CodeInvalidRequest             = -32600
	CodeMethodNotFound             = -32601
	CodeInvalidParams              = -32602
	CodeInternalError              = -32603
	CodeHeaderMismatch             = -32020
	CodeUnsupportedProtocolVersion = -32022
)

type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Title   string `json:"title,omitzero"`
}

// Annotations are what a client is told of a tool's effects. Every hint is sent, whether true or
// not: Claude requires readOnlyHint and destructiveHint declared, and lets a read-only tool run
// without asking.
type Annotations struct {
	Title           string `json:"title,omitzero"`
	ReadOnlyHint    bool   `json:"readOnlyHint"`
	DestructiveHint bool   `json:"destructiveHint"`
	IdempotentHint  bool   `json:"idempotentHint"`
	OpenWorldHint   bool   `json:"openWorldHint"`
}

// ToolError is a tool refusing what it was asked, told to the model as the tool's result rather
// than as a protocol error, so that it can try again otherwise.
type ToolError struct {
	Message string
}

func (e *ToolError) Error() string {
	return e.Message
}

// Tool is a tool the server offers. Call answers with what becomes the result's structured
// content, a JSON object, and its text; a *ToolError is a refusal, and any other error the
// server's.
type Tool struct {
	Name         string         `json:"name"`
	Title        string         `json:"title,omitzero"`
	Description  string         `json:"description,omitzero"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema,omitzero"`
	Annotations  *Annotations   `json:"annotations,omitzero"`

	Call func(ctx context.Context, arguments jsontext.Value) (any, error) `json:"-"`
}

type Server struct {
	Info         *Implementation
	Instructions string
	Tools        []*Tool
	// AllowedOrigins are the origins a request may come with. One coming with no Origin is not a
	// browser's and is let through; one with another origin is refused, which is what keeps a page
	// from reaching the server through its visitor's browser.
	AllowedOrigins []string
	// ListTtl is how long a client may keep the tool list before asking again.
	ListTtl time.Duration
}

// Reply is what answers a POST: a status, and a JSON body unless the status is 202. Method and
// Tool say what was asked, for the log.
type Reply struct {
	Status int
	Body   []byte
	Method string
	Tool   string
}

type message struct {
	JsonRpc string         `json:"jsonrpc"`
	Id      jsontext.Value `json:"id,omitzero"`
	Method  string         `json:"method,omitzero"`
	Params  jsontext.Value `json:"params,omitzero"`
	Result  jsontext.Value `json:"result,omitzero"`
	Error   jsontext.Value `json:"error,omitzero"`
}

type params struct {
	Meta            map[string]jsontext.Value `json:"_meta,omitzero"`
	Name            string                    `json:"name,omitzero"`
	Arguments       jsontext.Value            `json:"arguments,omitzero"`
	ProtocolVersion string                    `json:"protocolVersion,omitzero"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitzero"`
}

type response struct {
	JsonRpc string         `json:"jsonrpc"`
	Id      jsontext.Value `json:"id"`
	Result  any            `json:"result,omitzero"`
	Error   *rpcError      `json:"error,omitzero"`
}

// failure is a request answered with an error, at an HTTP status.
type failure struct {
	status int
	error  *rpcError
}

func fail(status int, code int, message string) *failure {
	return &failure{status: status, error: &rpcError{Code: code, Message: message}}
}

var nullId = jsontext.Value("null")

// Handle answers one POST of the endpoint.
func (s *Server) Handle(ctx context.Context, header http.Header, body []byte) *Reply {
	if origin := header.Get("Origin"); origin != "" && !slices.Contains(s.AllowedOrigins, origin) {
		slog.WarnContext(ctx, "An MCP request from an origin not allowed was refused.", slog.String("origin", origin))
		return s.reply(nullId, nil, fail(http.StatusForbidden, CodeInvalidRequest, "The origin is not allowed."))
	}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) != 0 && trimmed[0] == '[' {
		return s.reply(nullId, nil, fail(http.StatusBadRequest, CodeInvalidRequest, "Batches are not supported."))
	}
	var incoming message
	if err := json.Unmarshal(trimmed, &incoming); err != nil {
		return s.reply(nullId, nil, fail(http.StatusBadRequest, CodeParseError, "The body is not a JSON-RPC message."))
	}
	if incoming.JsonRpc != "2.0" {
		return s.reply(nullId, nil, fail(http.StatusBadRequest, CodeInvalidRequest, "The message is not JSON-RPC 2.0."))
	}

	// A notification, or a client's response to a request this server never makes: taken, and
	// nothing to answer.
	if len(incoming.Id) == 0 {
		if incoming.Method == "" && len(incoming.Result) == 0 && len(incoming.Error) == 0 {
			return s.reply(nullId, nil, fail(http.StatusBadRequest, CodeInvalidRequest, "The message is neither a request nor a notification."))
		}
		return &Reply{Status: http.StatusAccepted}
	}
	id := incoming.Id
	if kind := id.Kind(); kind != '"' && kind != '0' {
		return s.reply(nullId, nil, fail(http.StatusBadRequest, CodeInvalidRequest, "The id is neither a string nor a number."))
	}
	if incoming.Method == "" {
		return &Reply{Status: http.StatusAccepted}
	}

	var parsed params
	if len(incoming.Params) != 0 {
		if err := json.Unmarshal(incoming.Params, &parsed); err != nil {
			return s.reply(id, nil, fail(http.StatusBadRequest, CodeInvalidParams, "The params are not an object of the expected shape."))
		}
	}

	var (
		result any
		failed *failure
	)
	if claim, ok := parsed.Meta[metaProtocolVersion]; ok {
		result, failed = s.modern(ctx, header, incoming.Method, claim, &parsed)
	} else if version := header.Get(headerProtocolVersion); version >= ModernVersion {
		failed = fail(http.StatusBadRequest, CodeInvalidParams,
			"The request names "+ModernVersion+" or later in its header, but not in its _meta.")
	} else {
		result, failed = s.legacy(ctx, header, incoming.Method, &parsed)
	}
	reply := s.reply(id, result, failed)
	reply.Method = incoming.Method
	if incoming.Method == methodToolsCall {
		reply.Tool = parsed.Name
	}
	return reply
}

func (s *Server) modern(ctx context.Context, header http.Header, method string, claim jsontext.Value, parsed *params) (any, *failure) {
	var version string
	if err := json.Unmarshal(claim, &version); err != nil || version == "" {
		return nil, fail(http.StatusBadRequest, CodeInvalidParams, "The protocol version in _meta is not a string.")
	}
	if version != ModernVersion {
		failed := fail(http.StatusBadRequest, CodeUnsupportedProtocolVersion, "Unsupported protocol version")
		failed.error.Data = map[string]any{"supported": []string{ModernVersion}, "requested": version}
		return nil, failed
	}

	if failed := checkHeader(header, headerProtocolVersion, version); failed != nil {
		return nil, failed
	}
	if failed := checkHeader(header, headerMethod, method); failed != nil {
		return nil, failed
	}
	if method == methodToolsCall {
		if failed := checkHeader(header, headerName, parsed.Name); failed != nil {
			return nil, failed
		}
	}
	if _, ok := parsed.Meta[metaClientCapabilities]; !ok {
		return nil, fail(http.StatusBadRequest, CodeInvalidParams, "The request's _meta has no client capabilities.")
	}

	envelope := map[string]any{
		"resultType": "complete",
		"_meta":      map[string]any{metaServerInfo: s.Info},
	}
	cacheable := func() {
		envelope["ttlMs"] = s.ListTtl.Milliseconds()
		envelope["cacheScope"] = "public"
	}

	switch method {
	case "server/discover":
		envelope["supportedVersions"] = []string{ModernVersion}
		envelope["capabilities"] = map[string]any{keyTools: map[string]any{}}
		if s.Instructions != "" {
			envelope["instructions"] = s.Instructions
		}
		cacheable()
	case "tools/list":
		envelope[keyTools] = s.toolList()
		cacheable()
	case methodToolsCall:
		called, failed := s.call(ctx, parsed)
		if failed != nil {
			return nil, failed
		}
		for key, value := range called {
			envelope[key] = value
		}
	default:
		return nil, fail(http.StatusNotFound, CodeMethodNotFound, "Method not found: "+method)
	}

	return envelope, nil
}

func (s *Server) legacy(ctx context.Context, header http.Header, method string, parsed *params) (any, *failure) {
	// Absent, the version is what the transport had before the header; present, it must be one
	// served.
	if version := header.Get(headerProtocolVersion); version != "" && !slices.Contains(legacyVersions, version) {
		return nil, fail(http.StatusBadRequest, CodeInvalidRequest, "Unsupported protocol version: "+version)
	}

	switch method {
	case "initialize":
		version := LatestLegacyVersion
		if slices.Contains(legacyVersions, parsed.ProtocolVersion) {
			version = parsed.ProtocolVersion
		}
		result := map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{keyTools: map[string]any{"listChanged": false}},
			"serverInfo":      s.Info,
		}
		if s.Instructions != "" {
			result["instructions"] = s.Instructions
		}
		return result, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{keyTools: s.toolList()}, nil
	case methodToolsCall:
		called, failed := s.call(ctx, parsed)
		if failed != nil {
			return nil, failed
		}
		return called, nil
	default:
		return nil, fail(http.StatusOK, CodeMethodNotFound, "Method not found: "+method)
	}
}

// toolList is the tools in the order given, which is the same every time.
func (s *Server) toolList() []*Tool {
	tools := make([]*Tool, 0, len(s.Tools))
	for _, tool := range s.Tools {
		if tool != nil {
			tools = append(tools, tool)
		}
	}
	return tools
}

func (s *Server) call(ctx context.Context, parsed *params) (map[string]any, *failure) {
	var tool *Tool
	for _, candidate := range s.Tools {
		if candidate != nil && candidate.Name == parsed.Name {
			tool = candidate
			break
		}
	}
	if tool == nil || tool.Call == nil {
		return nil, fail(http.StatusOK, CodeInvalidParams, "Unknown tool: "+parsed.Name)
	}

	arguments := parsed.Arguments
	if len(arguments) == 0 || arguments.Kind() == 'n' {
		arguments = jsontext.Value("{}")
	}

	structured, err := tool.Call(ctx, arguments)
	if refused, ok := errors.AsType[*ToolError](err); ok && refused != nil {
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": refused.Message}},
			"isError": true,
		}, nil
	}
	if err != nil {
		slog.ErrorContext(ctx, "An MCP tool failed.",
			slog.String("tool", tool.Name), slog.Any("error", altshiftErrors.New(fmt.Errorf("tool call: %w", err))))
		return nil, fail(http.StatusOK, CodeInternalError, "The tool failed.")
	}

	// The structured content, and the same as text for a client that reads only that.
	text, err := json.Marshal(structured, json.Deterministic(true))
	if err != nil {
		slog.ErrorContext(ctx, "An MCP tool's result could not be marshalled.",
			slog.String("tool", tool.Name), slog.Any("error", altshiftErrors.NewWithTrace(fmt.Errorf("json marshal: %w", err))))
		return nil, fail(http.StatusOK, CodeInternalError, "The tool failed.")
	}
	return map[string]any{
		"content":           []map[string]any{{"type": "text", "text": string(text)}},
		"structuredContent": structured,
		"isError":           false,
	}, nil
}

// checkHeader holds a header to the value the body has, decoding a value sent base64-encoded for
// not being safe as a header.
func checkHeader(header http.Header, name string, want string) *failure {
	values := header.Values(name)
	if len(values) == 0 {
		return fail(http.StatusBadRequest, CodeHeaderMismatch, fmt.Sprintf("Header mismatch: the %s header is missing.", name))
	}
	got := values[0]
	if encoded, ok := strings.CutPrefix(got, "=?base64?"); ok {
		encoded, ok = strings.CutSuffix(encoded, "?=")
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if !ok || err != nil {
			return fail(http.StatusBadRequest, CodeHeaderMismatch, fmt.Sprintf("Header mismatch: the %s header is not valid base64.", name))
		}
		got = string(decoded)
	}
	if len(values) > 1 || got != want {
		return fail(http.StatusBadRequest, CodeHeaderMismatch,
			fmt.Sprintf("Header mismatch: the %s header value %q does not match the body value %q.", name, got, want))
	}
	return nil
}

func (s *Server) reply(id jsontext.Value, result any, failed *failure) *Reply {
	status := http.StatusOK
	out := &response{JsonRpc: "2.0", Id: id}
	if failed != nil {
		status = failed.status
		out.Error = failed.error
	} else {
		out.Result = result
	}

	body, err := json.Marshal(out, json.Deterministic(true))
	if err != nil {
		slog.Error("An MCP response could not be marshalled.", slog.Any("error", altshiftErrors.NewWithTrace(fmt.Errorf("json marshal: %w", err))))
		return &Reply{
			Status: http.StatusInternalServerError,
			Body:   []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"The response could not be written."}}`),
		}
	}
	return &Reply{Status: status, Body: body}
}
