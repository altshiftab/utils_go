package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

var errBroke = errors.New("the tool broke")

func newServer() *Server {
	return &Server{
		Info:           &Implementation{Name: "example", Version: "1.0.0", Title: "Example"},
		Instructions:   "Look words up.",
		AllowedOrigins: []string{"https://claude.ai"},
		ListTtl:        5 * time.Minute,
		Tools: []*Tool{
			{
				Name:        "echo",
				Title:       "Echo",
				InputSchema: map[string]any{"type": "object"},
				Annotations: &Annotations{ReadOnlyHint: true},
				Call: func(_ context.Context, arguments jsontext.Value) (any, error) {
					var input struct {
						Say string `json:"say"`
					}
					if err := json.Unmarshal(arguments, &input); err != nil {
						return nil, &ToolError{Message: "The arguments are not of the expected shape."}
					}
					if input.Say == "" {
						return nil, &ToolError{Message: "Nothing to say."}
					}
					return map[string]any{"said": input.Say}, nil
				},
			},
			{
				Name:        "broken",
				InputSchema: map[string]any{"type": "object"},
				Call: func(context.Context, jsontext.Value) (any, error) {
					return nil, errBroke
				},
			},
		},
	}
}

const modernMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`

// headers is a header of the name-value pairs, the names canonicalized as the server receives them.
func headers(pairs ...string) http.Header {
	header := http.Header{}
	for i := 0; i+1 < len(pairs); i += 2 {
		header.Add(pairs[i], pairs[i+1])
	}
	return header
}

func modernHeader(method string, name string) http.Header {
	header := headers(headerProtocolVersion, ModernVersion, headerMethod, method)
	if name != "" {
		header.Set(headerName, name)
	}
	return header
}

type answer struct {
	Id     jsontext.Value `json:"id"`
	Result map[string]any `json:"result"`
	Error  *rpcError      `json:"error"`
}

func TestServer_Handle(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		header     http.Header
		body       string
		wantStatus int
		wantCode   int
		// want checks the result.
		want func(t *testing.T, result map[string]any)
	}{
		{
			name:       "modern discover",
			header:     modernHeader("server/discover", ""),
			body:       `{"jsonrpc":"2.0","id":"d1","method":"server/discover","params":{` + modernMeta + `}}`,
			wantStatus: http.StatusOK,
			want: func(t *testing.T, result map[string]any) {
				t.Helper()
				versions, _ := result["supportedVersions"].([]any)
				meta, _ := result["_meta"].(map[string]any)
				if result["resultType"] != "complete" || len(versions) != 1 || versions[0] != ModernVersion ||
					result["ttlMs"] != float64(300000) || result["cacheScope"] != "public" || result["instructions"] != "Look words up." ||
					meta[metaServerInfo] == nil {
					t.Errorf("discover %v", result)
				}
			},
		},
		{
			name:       "modern tools/list",
			header:     modernHeader("tools/list", ""),
			body:       `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{` + modernMeta + `}}`,
			wantStatus: http.StatusOK,
			want: func(t *testing.T, result map[string]any) {
				t.Helper()
				tools, _ := result["tools"].([]any)
				first, _ := tools[0].(map[string]any)
				annotations, _ := first["annotations"].(map[string]any)
				if len(tools) != 2 || first["name"] != "echo" || result["resultType"] != "complete" || result["cacheScope"] != "public" {
					t.Errorf("tools/list %v", result)
				}
				// Both hints, the false one too.
				if annotations["readOnlyHint"] != true || annotations["destructiveHint"] != false {
					t.Errorf("annotations %v", annotations)
				}
			},
		},
		{
			name:       "modern tools/call",
			header:     modernHeader("tools/call", "echo"),
			body:       `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"say":"hej"},` + modernMeta + `}}`,
			wantStatus: http.StatusOK,
			want: func(t *testing.T, result map[string]any) {
				t.Helper()
				structured, _ := result["structuredContent"].(map[string]any)
				content, _ := result["content"].([]any)
				text, _ := content[0].(map[string]any)
				if structured["said"] != "hej" || result["isError"] != false || text["text"] != `{"said":"hej"}` || result["resultType"] != "complete" {
					t.Errorf("tools/call %v", result)
				}
				if _, ok := result["ttlMs"]; ok {
					t.Errorf("a call is no cacheable result: %v", result)
				}
			},
		},
		{
			name:       "modern tools/call, the name base64-encoded",
			header:     modernHeader("tools/call", "=?base64?"+base64.StdEncoding.EncodeToString([]byte("echo"))+"?="),
			body:       `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"echo","arguments":{"say":"hej"},` + modernMeta + `}}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "modern, the method header missing",
			header:     headers(headerProtocolVersion, ModernVersion),
			body:       `{"jsonrpc":"2.0","id":5,"method":"tools/list","params":{` + modernMeta + `}}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeHeaderMismatch,
		},
		{
			name:       "modern, the name header another tool's",
			header:     modernHeader("tools/call", "broken"),
			body:       `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"echo","arguments":{"say":"hej"},` + modernMeta + `}}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeHeaderMismatch,
		},
		{
			name:       "modern, the version header another version",
			header:     headers(headerProtocolVersion, LatestLegacyVersion, headerMethod, "tools/list"),
			body:       `{"jsonrpc":"2.0","id":7,"method":"tools/list","params":{` + modernMeta + `}}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeHeaderMismatch,
		},
		{
			name:       "a version not served",
			header:     headers(headerProtocolVersion, "2099-01-01", headerMethod, "tools/list"),
			body:       `{"jsonrpc":"2.0","id":8,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2099-01-01","io.modelcontextprotocol/clientCapabilities":{}}}}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeUnsupportedProtocolVersion,
		},
		{
			name:       "modern, no client capabilities",
			header:     modernHeader("tools/list", ""),
			body:       `{"jsonrpc":"2.0","id":9,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeInvalidParams,
		},
		{
			name:       "modern initialize, which the revision has not",
			header:     modernHeader("initialize", ""),
			body:       `{"jsonrpc":"2.0","id":10,"method":"initialize","params":{` + modernMeta + `}}`,
			wantStatus: http.StatusNotFound,
			wantCode:   CodeMethodNotFound,
		},
		{
			name:       "modern subscriptions/listen, which is not served",
			header:     modernHeader("subscriptions/listen", ""),
			body:       `{"jsonrpc":"2.0","id":11,"method":"subscriptions/listen","params":{` + modernMeta + `}}`,
			wantStatus: http.StatusNotFound,
			wantCode:   CodeMethodNotFound,
		},
		{
			name:       "a modern header without the _meta",
			header:     modernHeader("tools/list", ""),
			body:       `{"jsonrpc":"2.0","id":12,"method":"tools/list"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeInvalidParams,
		},
		{
			name:       "legacy initialize, a version served",
			body:       `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`,
			wantStatus: http.StatusOK,
			want: func(t *testing.T, result map[string]any) {
				t.Helper()
				info, _ := result["serverInfo"].(map[string]any)
				if result["protocolVersion"] != "2025-06-18" || info["name"] != "example" || result["instructions"] != "Look words up." {
					t.Errorf("initialize %v", result)
				}
				if _, ok := result["resultType"]; ok {
					t.Errorf("a legacy result with a result type: %v", result)
				}
			},
		},
		{
			name:       "legacy initialize asking for the modern version",
			body:       `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{}}}`,
			wantStatus: http.StatusOK,
			want: func(t *testing.T, result map[string]any) {
				t.Helper()
				if result["protocolVersion"] != LatestLegacyVersion {
					t.Errorf("initialize %v", result)
				}
			},
		},
		{
			name:       "legacy ping",
			header:     headers(headerProtocolVersion, LatestLegacyVersion),
			body:       `{"jsonrpc":"2.0","id":"p","method":"ping"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "legacy tools/call",
			header:     headers(headerProtocolVersion, LatestLegacyVersion),
			body:       `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"say":"hej"}}}`,
			wantStatus: http.StatusOK,
			want: func(t *testing.T, result map[string]any) {
				t.Helper()
				if structured, _ := result["structuredContent"].(map[string]any); structured["said"] != "hej" {
					t.Errorf("tools/call %v", result)
				}
			},
		},
		{
			name:       "legacy, a version header not served",
			header:     headers(headerProtocolVersion, "2024-11-05"),
			body:       `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   CodeInvalidRequest,
		},
		{
			name:       "legacy, a method not served",
			body:       `{"jsonrpc":"2.0","id":4,"method":"resources/list"}`,
			wantStatus: http.StatusOK,
			wantCode:   CodeMethodNotFound,
		},
		{
			name:       "a tool refusing",
			body:       `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"echo","arguments":{}}}`,
			wantStatus: http.StatusOK,
			want: func(t *testing.T, result map[string]any) {
				t.Helper()
				content, _ := result["content"].([]any)
				text, _ := content[0].(map[string]any)
				if result["isError"] != true || text["text"] != "Nothing to say." {
					t.Errorf("refusal %v", result)
				}
			},
		},
		{
			name:       "a tool failing",
			body:       `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"broken"}}`,
			wantStatus: http.StatusOK,
			wantCode:   CodeInternalError,
		},
		{
			name:       "a tool not served",
			body:       `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"nothing"}}`,
			wantStatus: http.StatusOK,
			wantCode:   CodeInvalidParams,
		},
		{name: "a notification", body: `{"jsonrpc":"2.0","method":"notifications/initialized"}`, wantStatus: http.StatusAccepted},
		{name: "a response", body: `{"jsonrpc":"2.0","id":1,"result":{}}`, wantStatus: http.StatusAccepted},
		{name: "a batch", body: `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`, wantStatus: http.StatusBadRequest, wantCode: CodeInvalidRequest},
		{name: "not json", body: `{"jsonrpc":`, wantStatus: http.StatusBadRequest, wantCode: CodeParseError},
		{name: "not 2.0", body: `{"jsonrpc":"1.0","id":1,"method":"ping"}`, wantStatus: http.StatusBadRequest, wantCode: CodeInvalidRequest},
		{name: "an id that is an object", body: `{"jsonrpc":"2.0","id":{},"method":"ping"}`, wantStatus: http.StatusBadRequest, wantCode: CodeInvalidRequest},
		{
			name:       "an origin not allowed",
			header:     headers("Origin", "https://evil.example"),
			body:       `{"jsonrpc":"2.0","id":1,"method":"ping"}`,
			wantStatus: http.StatusForbidden,
			wantCode:   CodeInvalidRequest,
		},
		{
			name:       "an origin allowed",
			header:     headers("Origin", "https://claude.ai"),
			body:       `{"jsonrpc":"2.0","id":1,"method":"ping"}`,
			wantStatus: http.StatusOK,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			header := testCase.header
			if header == nil {
				header = http.Header{}
			}
			reply := newServer().Handle(t.Context(), header, []byte(testCase.body))
			if reply == nil || reply.Status != testCase.wantStatus {
				t.Fatalf("%s: reply %+v, want status %d", testCase.name, reply, testCase.wantStatus)
			}
			if reply.Status == http.StatusAccepted {
				if len(reply.Body) != 0 {
					t.Errorf("%s: a body on 202: %s", testCase.name, reply.Body)
				}
				return
			}

			var got answer
			if err := json.Unmarshal(reply.Body, &got); err != nil {
				t.Fatalf("%s: unmarshal %s: %v", testCase.name, reply.Body, err)
			}
			if !strings.Contains(string(reply.Body), `"jsonrpc":"2.0"`) {
				t.Errorf("%s: %s", testCase.name, reply.Body)
			}
			if testCase.wantCode != 0 {
				if got.Error == nil || got.Error.Code != testCase.wantCode {
					t.Errorf("%s: %s, want error %d", testCase.name, reply.Body, testCase.wantCode)
				}
				return
			}
			if got.Error != nil {
				t.Fatalf("%s: error %+v", testCase.name, got.Error)
			}
			if testCase.want != nil {
				testCase.want(t, got.Result)
			}
		})
	}
}

// What was asked is told beside the answer, for the log.
func TestServer_Handle_Reply(t *testing.T) {
	t.Parallel()

	reply := newServer().Handle(t.Context(), http.Header{}, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"say":"x"}}}`))
	if reply.Method != "tools/call" || reply.Tool != "echo" {
		t.Errorf("reply %+v", reply)
	}
	if reply := newServer().Handle(t.Context(), http.Header{}, []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)); reply.Method != "ping" || reply.Tool != "" {
		t.Errorf("reply %+v", reply)
	}
}

// The request's id comes back as it was sent, and an unsupported version names what is.
func TestServer_Handle_Envelope(t *testing.T) {
	t.Parallel()

	reply := newServer().Handle(t.Context(), headers(headerProtocolVersion, "2099-01-01"),
		[]byte(`{"jsonrpc":"2.0","id":"an-id","method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2099-01-01"}}}`))
	var got struct {
		Id    string `json:"id"`
		Error struct {
			Data struct {
				Supported []string `json:"supported"`
				Requested string   `json:"requested"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal(reply.Body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Id != "an-id" || len(got.Error.Data.Supported) != 1 || got.Error.Data.Supported[0] != ModernVersion || got.Error.Data.Requested != "2099-01-01" {
		t.Errorf("reply %s", reply.Body)
	}
}
