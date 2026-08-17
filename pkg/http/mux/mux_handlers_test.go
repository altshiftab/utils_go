package mux

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_loader"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_loader/body_setting"
	bodyParserPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/body_parser"
	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	staticContentPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint/static_content"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	muxResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	muxResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	muxUtils "github.com/altshiftab/utils_go/pkg/http/mux/utils"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
)

func headerValue(headers []*muxResponse.HeaderEntry, name string) string {
	for _, header := range headers {
		if header != nil && header.Name == name {
			return header.Value
		}
	}
	return ""
}

func hasHeader(headers []*muxResponse.HeaderEntry, name string) bool {
	for _, header := range headers {
		if header != nil && header.Name == name {
			return true
		}
	}
	return false
}

func corsEndpoint(configuration *altshiftHttpTypes.CorsConfiguration, responseError *muxResponseError.ResponseError) *endpointPkg.Endpoint {
	return &endpointPkg.Endpoint{
		Path:   "/x",
		Method: http.MethodGet,
		CorsParser: request_parser.New(func(*http.Request) (*altshiftHttpTypes.CorsConfiguration, *muxResponseError.ResponseError) {
			return configuration, responseError
		}),
	}
}

func TestHandleUnmatchedMethod(t *testing.T) {
	t.Parallel()

	getEndpoint := &endpointPkg.Endpoint{Path: "/x", Method: http.MethodGet}

	t.Run("405 for a non-OPTIONS method", func(t *testing.T) {
		t.Parallel()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x", nil)
		response, responseError := handleUnmatchedMethod(
			request,
			request.Header,
			map[string]*endpointPkg.Endpoint{http.MethodGet: getEndpoint},
		)
		if response != nil {
			t.Fatal("expected a nil response")
		}
		if responseError == nil || responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %#v", responseError)
		}
		allow := headerValue(responseError.Headers, "Allow")
		if !strings.Contains(allow, "GET") || !strings.Contains(allow, "HEAD") || !strings.Contains(allow, "OPTIONS") {
			t.Errorf("Allow = %q", allow)
		}
	})

	t.Run("OPTIONS lists the allowed methods", func(t *testing.T) {
		t.Parallel()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/x", nil)
		response, responseError := handleUnmatchedMethod(
			request,
			request.Header,
			map[string]*endpointPkg.Endpoint{http.MethodGet: getEndpoint},
		)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if allow := headerValue(response.Headers, "Allow"); !strings.Contains(allow, "GET") {
			t.Errorf("Allow = %q", allow)
		}
	})

	t.Run("OPTIONS preflight builds CORS headers", func(t *testing.T) {
		t.Parallel()
		endpoint := corsEndpoint(&altshiftHttpTypes.CorsConfiguration{
			Origin:      "https://example.com",
			Credentials: true,
			MaxAge:      600,
			Headers:     []string{"X-Custom"},
		}, nil)
		request := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/x", nil)
		request.Header.Set("Access-Control-Request-Method", "GET")
		request.Header.Set("Access-Control-Request-Headers", "X-Custom")

		response, responseError := handleUnmatchedMethod(
			request,
			request.Header,
			map[string]*endpointPkg.Endpoint{http.MethodGet: endpoint},
		)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if got := headerValue(response.Headers, "Access-Control-Allow-Origin"); got != "https://example.com" {
			t.Errorf("Allow-Origin = %q", got)
		}
		if got := headerValue(response.Headers, "Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("Allow-Credentials = %q", got)
		}
		if got := headerValue(response.Headers, "Access-Control-Allow-Methods"); !strings.Contains(got, "GET") || !strings.Contains(got, "HEAD") {
			t.Errorf("Allow-Methods = %q", got)
		}
		if got := headerValue(response.Headers, "Access-Control-Allow-Headers"); got != "X-Custom" {
			t.Errorf("Allow-Headers = %q", got)
		}
		if got := headerValue(response.Headers, "Access-Control-Max-Age"); got != "600" {
			t.Errorf("Max-Age = %q", got)
		}
	})

	t.Run("OPTIONS preflight parser error propagates", func(t *testing.T) {
		t.Parallel()
		corsError := &muxResponseError.ResponseError{ProblemDetail: problem_detail.New(http.StatusBadRequest)}
		endpoint := corsEndpoint(nil, corsError)
		request := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/x", nil)
		request.Header.Set("Access-Control-Request-Method", "GET")

		_, responseError := handleUnmatchedMethod(
			request,
			request.Header,
			map[string]*endpointPkg.Endpoint{http.MethodGet: endpoint},
		)
		if responseError != corsError {
			t.Fatalf("expected the cors error, got %#v", responseError)
		}
	})
}

func TestEndpointCorsHeaderEntries(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)

	t.Run("nil parser yields no headers", func(t *testing.T) {
		t.Parallel()
		entries, responseError := endpointCorsHeaderEntries(&endpointPkg.Endpoint{}, request)
		if responseError != nil || entries != nil {
			t.Fatalf("expected no headers, got %#v (err %#v)", entries, responseError)
		}
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()
		corsError := &muxResponseError.ResponseError{ProblemDetail: problem_detail.New(http.StatusBadRequest)}
		_, responseError := endpointCorsHeaderEntries(corsEndpoint(nil, corsError), request)
		if responseError != corsError {
			t.Fatalf("expected the cors error, got %#v", responseError)
		}
	})

	t.Run("nil config yields no headers", func(t *testing.T) {
		t.Parallel()
		entries, responseError := endpointCorsHeaderEntries(corsEndpoint(nil, nil), request)
		if responseError != nil || entries != nil {
			t.Fatalf("expected no headers, got %#v (err %#v)", entries, responseError)
		}
	})

	t.Run("builds the headers", func(t *testing.T) {
		t.Parallel()
		endpoint := corsEndpoint(&altshiftHttpTypes.CorsConfiguration{
			Origin:        "https://example.com",
			Credentials:   true,
			ExposeHeaders: []string{"X-A", "X-B"},
		}, nil)
		entries, responseError := endpointCorsHeaderEntries(endpoint, request)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if got := headerValue(entries, "Access-Control-Allow-Origin"); got != "https://example.com" {
			t.Errorf("Allow-Origin = %q", got)
		}
		if got := headerValue(entries, "Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("Allow-Credentials = %q", got)
		}
		if got := headerValue(entries, "Access-Control-Expose-Headers"); got != "X-A, X-B" {
			t.Errorf("Expose-Headers = %q", got)
		}
	})
}

func newBodyRequest(t *testing.T, method, contentType, body string) *http.Request {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequestWithContext(t.Context(), method, "/x", reader)
	if body != "" {
		request.Header.Set("Content-Length", strconv.Itoa(len(body)))
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	return request
}

func TestHandleRequestBody(t *testing.T) {
	t.Parallel()

	t.Run("forbidden body (GET with a body) is rejected", func(t *testing.T) {
		t.Parallel()
		request := newBodyRequest(t, http.MethodGet, "", "data")
		_, _, responseError := handleRequestBody(&endpointPkg.Endpoint{}, request, httptest.NewRecorder(), request.Header, &altshiftHttpTypes.HttpContext{})
		if responseError == nil {
			t.Fatal("expected an error for a body on a GET request")
		}
	})

	t.Run("required body missing is 411", func(t *testing.T) {
		t.Parallel()
		endpoint := &endpointPkg.Endpoint{BodyLoader: &body_loader.Loader{Setting: body_setting.Required}}
		request := newBodyRequest(t, http.MethodPost, "", "")
		_, _, responseError := handleRequestBody(endpoint, request, httptest.NewRecorder(), request.Header, &altshiftHttpTypes.HttpContext{})
		if responseError == nil || responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusLengthRequired {
			t.Fatalf("expected 411, got %#v", responseError)
		}
	})

	t.Run("content-type mismatch is 415", func(t *testing.T) {
		t.Parallel()
		endpoint := &endpointPkg.Endpoint{BodyLoader: &body_loader.Loader{ContentType: "application/json", Setting: body_setting.Optional}}
		request := newBodyRequest(t, http.MethodPost, "text/plain", `{"a":1}`)
		_, _, responseError := handleRequestBody(endpoint, request, httptest.NewRecorder(), request.Header, &altshiftHttpTypes.HttpContext{})
		if responseError == nil || responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusUnsupportedMediaType {
			t.Fatalf("expected 415, got %#v", responseError)
		}
	})

	t.Run("invalid json is 400", func(t *testing.T) {
		t.Parallel()
		endpoint := &endpointPkg.Endpoint{BodyLoader: &body_loader.Loader{ContentType: "application/json", Setting: body_setting.Optional}}
		request := newBodyRequest(t, http.MethodPost, "application/json", "{not-json")
		_, _, responseError := handleRequestBody(endpoint, request, httptest.NewRecorder(), request.Header, &altshiftHttpTypes.HttpContext{})
		if responseError == nil || responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %#v", responseError)
		}
	})

	t.Run("body over the limit is 413", func(t *testing.T) {
		t.Parallel()
		endpoint := &endpointPkg.Endpoint{BodyLoader: &body_loader.Loader{Setting: body_setting.Optional, MaxBytes: 5}}
		request := newBodyRequest(t, http.MethodPost, "", "0123456789")
		_, _, responseError := handleRequestBody(endpoint, request, httptest.NewRecorder(), request.Header, &altshiftHttpTypes.HttpContext{})
		if responseError == nil || responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusRequestEntityTooLarge {
			t.Fatalf("expected 413, got %#v", responseError)
		}
	})

	t.Run("valid body runs the parser and records it", func(t *testing.T) {
		t.Parallel()
		endpoint := &endpointPkg.Endpoint{BodyLoader: &body_loader.Loader{
			Setting: body_setting.Required,
			Parser: bodyParserPkg.New(func(_ *http.Request, body []byte) (any, *muxResponseError.ResponseError) {
				return string(body), nil
			}),
		}}
		request := newBodyRequest(t, http.MethodPost, "", "payload")
		httpContext := &altshiftHttpTypes.HttpContext{}

		updatedRequest, requestBody, responseError := handleRequestBody(endpoint, request, httptest.NewRecorder(), request.Header, httpContext)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if string(requestBody) != "payload" {
			t.Fatalf("requestBody = %q, want payload", requestBody)
		}
		if string(httpContext.RequestBody) != "payload" {
			t.Errorf("httpContext.RequestBody = %q, want payload", httpContext.RequestBody)
		}
		if parsed, ok := updatedRequest.Context().Value(muxUtils.ParsedRequestBodyContextKey).(string); !ok || parsed != "payload" {
			t.Errorf("parsed body in context = %v, want payload", updatedRequest.Context().Value(muxUtils.ParsedRequestBodyContextKey))
		}
	})

	t.Run("body parser error propagates", func(t *testing.T) {
		t.Parallel()
		parserError := &muxResponseError.ResponseError{ProblemDetail: problem_detail.New(http.StatusUnprocessableEntity)}
		endpoint := &endpointPkg.Endpoint{BodyLoader: &body_loader.Loader{
			Setting: body_setting.Required,
			Parser: bodyParserPkg.New(func(_ *http.Request, _ []byte) (any, *muxResponseError.ResponseError) {
				return nil, parserError
			}),
		}}
		request := newBodyRequest(t, http.MethodPost, "", "payload")
		_, _, responseError := handleRequestBody(endpoint, request, httptest.NewRecorder(), request.Header, &altshiftHttpTypes.HttpContext{})
		if responseError != parserError {
			t.Fatalf("expected the parser error, got %#v", responseError)
		}
	})
}

func TestProduceResponse(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)

	t.Run("handler response carries a Vary header", func(t *testing.T) {
		t.Parallel()
		endpoint := &endpointPkg.Endpoint{
			Handler: func(*http.Request, []byte) (*muxResponse.Response, *muxResponseError.ResponseError) {
				return &muxResponse.Response{Body: []byte("ok")}, nil
			},
		}
		response, responseError := produceResponse(endpoint, request, nil, request.Header)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if string(response.Body) != "ok" {
			t.Fatalf("body = %q, want ok", response.Body)
		}
		if !hasHeader(response.Headers, "Vary") {
			t.Error("expected a Vary header")
		}
	})

	t.Run("handler error propagates", func(t *testing.T) {
		t.Parallel()
		handlerError := &muxResponseError.ResponseError{ProblemDetail: problem_detail.New(http.StatusBadRequest)}
		endpoint := &endpointPkg.Endpoint{
			Handler: func(*http.Request, []byte) (*muxResponse.Response, *muxResponseError.ResponseError) {
				return nil, handlerError
			},
		}
		_, responseError := produceResponse(endpoint, request, nil, request.Header)
		if responseError != handlerError {
			t.Fatalf("expected the handler error, got %#v", responseError)
		}
	})

	t.Run("static content returns 200 with the body", func(t *testing.T) {
		t.Parallel()
		endpoint := &endpointPkg.Endpoint{StaticContent: &staticContentPkg.StaticContent{
			StaticContentData: staticContentPkg.StaticContentData{Data: []byte("static")},
		}}
		response, responseError := produceResponse(endpoint, request, nil, request.Header)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if response.StatusCode != http.StatusOK || string(response.Body) != "static" {
			t.Fatalf("unexpected response: %#v", response)
		}
	})

	t.Run("cached static content returns 304", func(t *testing.T) {
		t.Parallel()
		endpoint := &endpointPkg.Endpoint{StaticContent: &staticContentPkg.StaticContent{
			StaticContentData: staticContentPkg.StaticContentData{Etag: `"v"`, Data: []byte("x")},
		}}
		cachedRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
		cachedRequest.Header.Set("If-None-Match", `"v"`)
		response, responseError := produceResponse(endpoint, cachedRequest, nil, cachedRequest.Header)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if response.StatusCode != http.StatusNotModified {
			t.Fatalf("expected 304, got %d", response.StatusCode)
		}
	})

	t.Run("disabled fetch metadata omits the Vary header", func(t *testing.T) {
		t.Parallel()
		endpoint := &endpointPkg.Endpoint{
			DisableFetchMetadata: true,
			Handler: func(*http.Request, []byte) (*muxResponse.Response, *muxResponseError.ResponseError) {
				return &muxResponse.Response{}, nil
			},
		}
		response, responseError := produceResponse(endpoint, request, nil, request.Header)
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if hasHeader(response.Headers, "Vary") {
			t.Error("did not expect a Vary header when fetch metadata is disabled")
		}
	})
}
