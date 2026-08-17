package mux

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	muxTesting "github.com/altshiftab/utils_go/pkg/http/mux/testing"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_loader"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_loader/body_setting"
	bodyParserAdapter "github.com/altshiftab/utils_go/pkg/http/mux/types/body_parser/adapter"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_parser/json_body_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	muxTypesStaticContent "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint/static_content"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/firewall_verdict"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/rate_limiting"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_writer"
	"github.com/altshiftab/utils_go/pkg/http/mux/utils"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
	"github.com/altshiftab/utils_go/pkg/http/types/retry_after"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

var httpServer *httptest.Server

type bodyParserTestData struct {
	Data string `json:"data"`
}

var defaultHtmlProblemDetailMediaRanges = slices.Concat(
	response_error.DefaultProblemDetailMediaRanges,
	[]*altshiftHttpTypes.ServerMediaRange{{Type: "text", Subtype: "html"}},
)

func htmlConvertProblemDetail(
	detail *problem_detail.Detail,
	negotiation *altshiftHttpTypes.ContentNegotiation,
) ([]byte, string, error) {
	if detail == nil {
		return nil, "", nil_error.New("problem detail")
	}

	if detail.Status == http.StatusTeapot && negotiation != nil {
		if negotiation.NegotiatedAccept == "" {
			matchingServerMediaRange := altshiftHttpUtils.GetMatchingAccept(
				negotiation.Accept.GetPriorityOrderedEncodings(),
				defaultHtmlProblemDetailMediaRanges,
			)
			if matchingServerMediaRange != nil {
				negotiation.NegotiatedAccept = matchingServerMediaRange.GetFullType(true)
			}
		}

		switch negotiatedAccept := negotiation.NegotiatedAccept; negotiatedAccept {
		case "text/html":
			return []byte(fmt.Sprintf("<html>%d</html>", detail.Status)), "text/html", nil
		}
	}

	return response_error.ConvertProblemDetail(detail, negotiation)
}

var HtmlProblemDetailConverter = response_error.ProblemDetailConverterFunction(htmlConvertProblemDetail)

func TestMain(m *testing.M) {
	mux := &Mux{}
	mux.FirewallParser = request_parser.New(
		func(request *http.Request) (firewall_verdict.Verdict, *response_error.ResponseError) {
			if request.URL.RawQuery != "" {
				return firewall_verdict.Reject, &response_error.ResponseError{
					ProblemDetail: problem_detail.New(
						http.StatusForbidden,
						problem_detail_config.WithDetail("URL query parameters are not allowed."),
					),
				}
			}

			if request.URL.Path == "/secret-reject" {
				return firewall_verdict.Reject, nil
			}

			if request.URL.Path == "/secret-drop" {
				return firewall_verdict.Drop, nil
			}

			return firewall_verdict.Accept, nil
		},
	)
	mux.ProblemDetailConverter = HtmlProblemDetailConverter

	mux.Add(
		&endpoint.Endpoint{
			Path:   "/hello-world",
			Method: http.MethodGet,
			Handler: func(request *http.Request, body []byte) (*muxTypesResponse.Response, *response_error.ResponseError) {
				return &muxTypesResponse.Response{
					Body: []byte("hello world"),
					Headers: []*muxTypesResponse.HeaderEntry{
						{Name: "Content-Type", Value: "application/octet-stream"},
					},
				}, nil
			},
		},
		&endpoint.Endpoint{
			Path:   "/hello-world",
			Method: http.MethodPost,
		},
		&endpoint.Endpoint{
			Path:   "/hello-world-static",
			Method: http.MethodGet,
			StaticContent: &muxTypesStaticContent.StaticContent{
				StaticContentData: muxTypesStaticContent.StaticContentData{
					Data: []byte("<html>hello world</html>"),
					Headers: []*muxTypesResponse.HeaderEntry{
						{Name: "Content-Type", Value: "text/html"},
					},
				},
			},
		},
		&endpoint.Endpoint{
			Path:   "/hello-world-fetch-metadata",
			Method: http.MethodGet,
			StaticContent: &muxTypesStaticContent.StaticContent{
				StaticContentData: muxTypesStaticContent.StaticContentData{
					Data: []byte("<html>hello world</html>"),
					Headers: []*muxTypesResponse.HeaderEntry{
						{Name: "Content-Type", Value: "text/html"},
						{Name: "Cache-Control", Value: "no-cache", Overwrite: true},
					},
				},
			},
		},
		&endpoint.Endpoint{
			Path:       "/push",
			Method:     http.MethodPost,
			BodyLoader: &body_loader.Loader{ContentType: "application/json"},
		},
		&endpoint.Endpoint{
			Path:   "/empty",
			Method: http.MethodGet,
			Handler: func(request *http.Request, body []byte) (*muxTypesResponse.Response, *response_error.ResponseError) {
				return nil, nil
			},
		},
		&endpoint.Endpoint{
			Path:   "/body-parsing",
			Method: http.MethodPost,
			Handler: func(request *http.Request, body []byte) (*muxTypesResponse.Response, *response_error.ResponseError) {
				d, responseError := utils.GetServerNonZeroParsedRequestBody[*bodyParserTestData](request.Context())
				if responseError != nil {
					return nil, responseError
				}

				if d.Data != "hello world" {
					return nil, &response_error.ResponseError{
						ProblemDetail: problem_detail.New(http.StatusInternalServerError),
					}
				}

				return nil, nil
			},
			BodyLoader: &body_loader.Loader{
				Parser:      bodyParserAdapter.New(json_body_parser.New[*bodyParserTestData]()),
				ContentType: "application/json",
			},
		},
		&endpoint.Endpoint{
			Path:   "/body-parsing-limit",
			Method: http.MethodPost,
			Handler: func(request *http.Request, body []byte) (*muxTypesResponse.Response, *response_error.ResponseError) {
				return nil, nil
			},
			BodyLoader: &body_loader.Loader{
				ContentType: "application/octet-stream",
				MaxBytes:    2,
			},
		},
		&endpoint.Endpoint{
			Path:   "/forbidden-body",
			Method: http.MethodPost,
			Handler: func(request *http.Request, body []byte) (*muxTypesResponse.Response, *response_error.ResponseError) {
				return nil, nil
			},
			BodyLoader: &body_loader.Loader{
				Setting: body_setting.Forbidden,
			},
		},
		&endpoint.Endpoint{
			Path:   "/teapot",
			Method: http.MethodGet,
			Handler: func(request *http.Request, i []byte) (*muxTypesResponse.Response, *response_error.ResponseError) {
				return nil, &response_error.ResponseError{
					ProblemDetail: problem_detail.New(http.StatusTeapot),
				}
			},
		},
		&endpoint.Endpoint{
			Path:   "/rate-limiting",
			Method: http.MethodGet,
			RateLimitingConfiguration: &rate_limiting.RateLimitingConfiguration{
				NumRequests:          3,
				NumSecondsExpiration: 5,
			},
		},
		&endpoint.Endpoint{
			Path:   "/cors",
			Method: http.MethodGet,
			CorsParser: request_parser.RequestParserFunction[*altshiftHttpTypes.CorsConfiguration](
				func(r *http.Request) (*altshiftHttpTypes.CorsConfiguration, *response_error.ResponseError) {
					return &altshiftHttpTypes.CorsConfiguration{
						Origin:        "*",
						Credentials:   true,
						Headers:       []string{"X-Custom-Header", "X-Custom-Header-2"},
						ExposeHeaders: []string{"X-Secret"},
					}, nil
				},
			),
			Handler: func(request *http.Request, body []byte) (*muxTypesResponse.Response, *response_error.ResponseError) {
				return nil, nil
			},
		},
		&endpoint.Endpoint{
			Path:   "/cors",
			Method: http.MethodPost,
			CorsParser: request_parser.RequestParserFunction[*altshiftHttpTypes.CorsConfiguration](
				func(r *http.Request) (*altshiftHttpTypes.CorsConfiguration, *response_error.ResponseError) {
					return &altshiftHttpTypes.CorsConfiguration{
						Origin:        "*",
						Credentials:   true,
						Headers:       []string{"X-Custom-Header-3", "X-Custom-Header-4"},
						ExposeHeaders: []string{"X-Secret-2"},
					}, nil
				},
			),
			Handler: func(request *http.Request, body []byte) (*muxTypesResponse.Response, *response_error.ResponseError) {
				return nil, nil
			},
		},
		&endpoint.Endpoint{
			Path:   "/cors",
			Method: http.MethodPatch,
			Handler: func(request *http.Request, body []byte) (*muxTypesResponse.Response, *response_error.ResponseError) {
				return nil, nil
			},
		},
	)

	httpServer = httptest.NewServer(mux)

	code := m.Run()
	httpServer.Close()

	os.Exit(code)
}

// TODO: Test adding and deleting from a mux.

func TestMux(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args *muxTesting.Args
	}{
		{
			name: "status ok, handler",
			args: &muxTesting.Args{
				Method:             http.MethodGet,
				Path:               "/hello-world",
				ExpectedStatusCode: http.StatusOK,
				ExpectedBody:       []byte("hello world"),
				ExpectedHeaders:    [][2]string{{"Content-Type", "application/octet-stream"}},
			},
		},
		{
			name: "status ok, handler (HEAD)",
			args: &muxTesting.Args{
				Method:             http.MethodHead,
				Path:               "/hello-world",
				ExpectedStatusCode: http.StatusOK,
				ExpectedHeaders:    [][2]string{{"Content-Type", "application/octet-stream"}},
			},
		},
		{
			name: "status no content (OPTIONS)",
			args: &muxTesting.Args{
				Method:             http.MethodOptions,
				Path:               "/hello-world",
				ExpectedStatusCode: http.StatusNoContent,
				ExpectedHeaders:    [][2]string{{"Allow", "GET, HEAD, OPTIONS, POST"}},
			},
		},
		{
			name: "status ok, static content",
			args: &muxTesting.Args{
				Method:             http.MethodGet,
				Path:               "/hello-world-static",
				ExpectedStatusCode: http.StatusOK,
				ExpectedBody:       []byte("<html>hello world</html>"),
				ExpectedHeaders:    [][2]string{{"Content-Type", "text/html"}},
			},
		},
		{
			name: "fetch metadata vary",
			args: &muxTesting.Args{
				Method:             http.MethodHead,
				Path:               "/hello-world-fetch-metadata",
				ExpectedStatusCode: http.StatusOK,
				ExpectedHeaders:    [][2]string{{"Vary", "Sec-Fetch-Dest, Sec-Fetch-Mode, Sec-Fetch-Site"}},
			},
		},
		{
			name: "fetch metadata forbidden",
			args: &muxTesting.Args{
				Method: http.MethodGet,
				Path:   "/hello-world",
				Headers: [][2]string{
					{"Sec-Fetch-Site", "cross-origin"},
				},
				ExpectedStatusCode: http.StatusForbidden,
				ExpectedProblemDetail: &problem_detail.Detail{
					Detail: "Cross-site request blocked by Fetch-Metadata policy.",
				},
			},
		},
		{
			name: "default headers",
			args: &muxTesting.Args{
				Method:             http.MethodGet,
				Path:               "/hello-world",
				ExpectedStatusCode: http.StatusOK,
				ExpectedBody:       []byte("hello world"),
				ExpectedHeaders: func() [][2]string {
					var headers [][2]string
					for key, value := range response_writer.DefaultHeaders {
						headers = append(headers, [2]string{key, value})
					}
					return headers
				}(),
			},
		},
		{
			name: "default document headers",
			args: &muxTesting.Args{
				Method:             http.MethodGet,
				Path:               "/hello-world-static",
				ExpectedStatusCode: http.StatusOK,
				ExpectedBody:       []byte("<html>hello world</html>"),
				ExpectedHeaders: func() [][2]string {
					var headers [][2]string
					for key, value := range response_writer.DefaultHeaders {
						headers = append(headers, [2]string{key, value})
					}

					for key, value := range response_writer.DefaultDocumentHeaders {
						headers = append(headers, [2]string{key, value})
					}
					return headers
				}(),
			},
		},
		{
			name: "bad method",
			args: &muxTesting.Args{
				Method:             http.MethodPatch,
				Path:               "/hello-world",
				ExpectedStatusCode: http.StatusMethodNotAllowed,
				ExpectedProblemDetail: &problem_detail.Detail{
					Detail: `Expected GET, HEAD, OPTIONS, POST.`,
				},
				ExpectedHeaders: [][2]string{{"Allow", "GET, HEAD, OPTIONS, POST"}},
			},
		},
		{
			name: "error status method not allowed, without response body",
			args: &muxTesting.Args{
				Method:             http.MethodPatch,
				Path:               "/hello-world",
				Headers:            [][2]string{{"Accept-Encoding", "*;q=0"}},
				ExpectedStatusCode: http.StatusMethodNotAllowed,
				ExpectedHeaders:    [][2]string{{"Allow", "GET, HEAD, OPTIONS, POST"}},
			},
		},
		{
			name: "status no content",
			args: &muxTesting.Args{
				Method:             http.MethodGet,
				Path:               "/empty",
				ExpectedStatusCode: http.StatusNoContent,
			},
		},
		{
			name: "error status not found",
			args: &muxTesting.Args{
				Method:                http.MethodGet,
				Path:                  "/not-found",
				ExpectedStatusCode:    http.StatusNotFound,
				ExpectedProblemDetail: &problem_detail.Detail{},
			},
		},
		{
			name: "status ok, post data",
			args: &muxTesting.Args{
				Method:             http.MethodPost,
				Path:               "/push",
				Headers:            [][2]string{{"Content-Type", "application/json"}},
				Body:               []byte(`{"data": "data"}`),
				ExpectedStatusCode: http.StatusNoContent,
			},
		},
		{
			name: "status no content, body parsing test",
			args: &muxTesting.Args{
				Method:             http.MethodPost,
				Path:               "/body-parsing",
				Headers:            [][2]string{{"Content-Type", "application/json"}},
				Body:               []byte(`{"data": "hello world"}`),
				ExpectedStatusCode: http.StatusNoContent,
			},
		},
		{
			name: "error status bad request, post no data",
			args: &muxTesting.Args{
				Method:             http.MethodPost,
				Path:               "/push",
				Headers:            [][2]string{{"Content-Type", "application/json"}},
				ExpectedStatusCode: http.StatusBadRequest,
				ExpectedProblemDetail: &problem_detail.Detail{
					Detail: "A body is expected; Content-Length cannot be 0.",
				},
			},
		},
		{
			name: "error status unsupported media type, missing content type",
			args: &muxTesting.Args{
				Method:             http.MethodPost,
				Path:               "/push",
				Body:               []byte(`{"data": "data"}`),
				ExpectedStatusCode: http.StatusUnsupportedMediaType,
				ExpectedHeaders:    [][2]string{{"Accept", "application/json"}},
				ExpectedProblemDetail: &problem_detail.Detail{
					Detail: "Missing Content-Type.",
				},
			},
		},
		{
			name: "error status bad request, malformed content type",
			args: &muxTesting.Args{
				Method:             http.MethodPost,
				Path:               "/push",
				Headers:            [][2]string{{"Content-Type", ""}},
				Body:               []byte(`{"data": "data"}`),
				ExpectedStatusCode: http.StatusBadRequest,
				ExpectedProblemDetail: &problem_detail.Detail{
					Detail: "Malformed Content-Type.",
				},
			},
		},
		{
			name: "error status unsupported media type, other content type",
			args: &muxTesting.Args{
				Method:             http.MethodPost,
				Path:               "/push",
				Headers:            [][2]string{{"Content-Type", "text/plain"}},
				ExpectedStatusCode: http.StatusUnsupportedMediaType,
				ExpectedHeaders:    [][2]string{{"Accept", "application/json"}},
				ExpectedProblemDetail: &problem_detail.Detail{
					Detail: `Expected Content-Type to be "application/json", observed "text/plain".`,
				},
			},
		},
		{
			name: "error bad request, invalid json body",
			args: &muxTesting.Args{
				Method:                http.MethodPost,
				Path:                  "/push",
				Headers:               [][2]string{{"Content-Type", "application/json"}},
				Body:                  []byte(`{"data": "data"`),
				ExpectedStatusCode:    http.StatusBadRequest,
				ExpectedProblemDetail: &problem_detail.Detail{Detail: "Invalid JSON body."},
			},
		},
		{
			name: "error status forbidden, firewall match url query parameters",
			args: &muxTesting.Args{
				Method:                http.MethodGet,
				Path:                  "/foo?bar=fuu",
				ExpectedStatusCode:    http.StatusForbidden,
				ExpectedProblemDetail: &problem_detail.Detail{Detail: "URL query parameters are not allowed."},
			},
		},
		{
			name: "error status forbidden, firewall match url secret (reject)",
			args: &muxTesting.Args{
				Method:                http.MethodGet,
				Path:                  "/secret-reject",
				ExpectedStatusCode:    http.StatusForbidden,
				ExpectedProblemDetail: &problem_detail.Detail{},
			},
		},
		{
			name: "error status forbidden, firewall match url secret (drop)",
			args: &muxTesting.Args{
				Method:                http.MethodGet,
				Path:                  "/secret-drop",
				ExpectedClientDoError: io.EOF,
			},
		},
		{
			name: "custom error representation",
			args: &muxTesting.Args{
				Method:             http.MethodGet,
				Path:               "/teapot",
				Headers:            [][2]string{{"Accept", "text/html"}},
				ExpectedStatusCode: http.StatusTeapot,
				ExpectedBody:       []byte("<html>418</html>"),
				ExpectedHeaders:    [][2]string{{"Content-Type", "text/html"}},
			},
		},
		{
			name: "max bytes too large",
			args: &muxTesting.Args{
				Method:                http.MethodPost,
				Path:                  "/body-parsing-limit",
				Headers:               [][2]string{{"Content-Type", "application/octet-stream"}},
				Body:                  []byte("123"),
				ExpectedStatusCode:    http.StatusRequestEntityTooLarge,
				ExpectedProblemDetail: &problem_detail.Detail{Detail: "Limit: 2 bytes"},
			},
		},
		{
			name: "max bytes ok",
			args: &muxTesting.Args{
				Method:             http.MethodPost,
				Path:               "/body-parsing-limit",
				Headers:            [][2]string{{"Content-Type", "application/octet-stream"}},
				Body:               []byte("12"),
				ExpectedStatusCode: http.StatusNoContent,
			},
		},
		{
			name: "forbidden body",
			args: &muxTesting.Args{
				Method:                http.MethodPost,
				Path:                  "/forbidden-body",
				Body:                  []byte("12"),
				ExpectedStatusCode:    http.StatusRequestEntityTooLarge,
				ExpectedProblemDetail: &problem_detail.Detail{Detail: "Limit: 0 bytes"},
			},
		},
		{
			name: "forbidden body get",
			args: &muxTesting.Args{
				Method:                http.MethodGet,
				Path:                  "/hello-world",
				Body:                  []byte("12"),
				ExpectedStatusCode:    http.StatusRequestEntityTooLarge,
				ExpectedProblemDetail: &problem_detail.Detail{Detail: "Limit: 0 bytes"},
			},
		},
		{
			name: "cors preflight",
			args: &muxTesting.Args{
				Method: http.MethodOptions,
				Headers: [][2]string{
					{"Origin", "https://example.com"},
					{"Access-Control-Request-Method", "POST"},
					{"Access-Control-Request-Headers", "X-Custom-Header-3, X-Custom-Header-4"},
				},
				Path:               "/cors",
				ExpectedStatusCode: http.StatusNoContent,
				ExpectedHeaders: [][2]string{
					{"Access-Control-Allow-Methods", "GET, HEAD, POST"},
					{"Access-Control-Allow-Origin", "*"},
					{"Access-Control-Allow-Credentials", "true"},
					{"Access-Control-Allow-Headers", "X-Custom-Header-3, X-Custom-Header-4"},
					{"Allow", "GET, HEAD, OPTIONS, PATCH, POST"},
				},
			},
		},
		{
			name: "cors post",
			args: &muxTesting.Args{
				Method: http.MethodPost,
				Headers: [][2]string{
					{"Origin", "https://example.com"},
				},
				Path:               "/cors",
				ExpectedStatusCode: http.StatusNoContent,
				ExpectedHeaders: [][2]string{
					{"Access-Control-Allow-Origin", "*"},
					{"Access-Control-Allow-Credentials", "true"},
					{"Access-Control-Expose-Headers", "X-Secret-2"},
				},
			},
		},
	}

	// TODO: Write test for URL parsing.

	for _, testCase := range testCases {
		t.Run(
			testCase.name,
			func(t *testing.T) {
				t.Parallel()
				muxTesting.TestArgs(t, testCase.args, httpServer.URL)
			},
		)
	}
}

func TestRateLimiting(t *testing.T) {
	t.Parallel()

	// The rate limiter is driven by wall-clock timers (time.Now/time.AfterFunc). Run
	// the test inside a synctest bubble with a fake clock and drive the mux in-process
	// (synctest cannot virtualize a real server's socket I/O). time.Sleep then advances
	// the fake clock instantly rather than waiting for the real rate-limit window.
	synctest.Test(t, func(t *testing.T) {
		mux := &Mux{}
		mux.Add(
			&endpoint.Endpoint{
				Path:   "/rate-limiting",
				Method: http.MethodGet,
				RateLimitingConfiguration: &rate_limiting.RateLimitingConfiguration{
					NumRequests:          3,
					NumSecondsExpiration: 5,
				},
			},
		)

		get := func() *httptest.ResponseRecorder {
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/rate-limiting", nil))
			return recorder
		}

		// The first NumRequests requests are allowed.
		for range 3 {
			if recorder := get(); recorder.Code != http.StatusNoContent {
				t.Errorf("got status code %d, expected %d", recorder.Code, http.StatusNoContent)
			}
		}

		// The next request exceeds the limit.
		recorder := get()
		if recorder.Code != http.StatusTooManyRequests {
			t.Errorf("got status code %d, expected %d", recorder.Code, http.StatusTooManyRequests)
		}

		retryAfterValue := recorder.Header().Get("Retry-After")
		if retryAfterValue == "" {
			t.Fatal("no Retry-After header")
		}

		retryAfter, err := retry_after.Parse([]byte(retryAfterValue))
		if err != nil {
			t.Fatalf("invalid Retry-After: %v", err)
		}

		waitTime, ok := retryAfter.WaitTime.(time.Time)
		if !ok {
			t.Fatal("invalid Retry-After wait time")
		}

		// Advance the fake clock past the rate-limit window; the bucket-freeing timers
		// fire during this sleep. synctest.Wait then ensures those timer goroutines have
		// completed before the next request is issued.
		time.Sleep(time.Until(waitTime))
		synctest.Wait()

		if recorder := get(); recorder.Code != http.StatusNoContent {
			t.Errorf("got status code %d, expected %d", recorder.Code, http.StatusNoContent)
		}
	})
}
