package mux

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	muxTypesStaticContent "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint/static_content"
	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
)

// TestHandlerWithStaticContent covers the division of labour between a handler and static content
// on the same endpoint: a handler that produces a status answers the request, and one that produces
// none contributes its headers to the page.
func TestHandlerWithStaticContent(t *testing.T) {
	t.Parallel()

	const staticContentBody = "<html>the page</html>"

	testCases := []struct {
		name               string
		handler            endpoint.Handler
		expectedStatusCode int
		expectedBody       string
		expectedHeaderName string
		expectedHeader     string
	}{
		{
			name: "no response serves the static content",
			handler: func(*http.Request, []byte) (*muxTypesResponse.Response, *response_error.ResponseError) {
				return nil, nil
			},
			expectedStatusCode: http.StatusOK,
			expectedBody:       staticContentBody,
		},
		{
			name: "headers without a status decorate the static content",
			handler: func(*http.Request, []byte) (*muxTypesResponse.Response, *response_error.ResponseError) {
				return &muxTypesResponse.Response{
					Headers: []*muxTypesResponse.HeaderEntry{{Name: "Clear-Site-Data", Value: `"cookies"`}},
				}, nil
			},
			expectedStatusCode: http.StatusOK,
			expectedBody:       staticContentBody,
			expectedHeaderName: "Clear-Site-Data",
			expectedHeader:     `"cookies"`,
		},
		{
			name: "a status takes the request from the static content",
			handler: func(*http.Request, []byte) (*muxTypesResponse.Response, *response_error.ResponseError) {
				return &muxTypesResponse.Response{
					StatusCode: http.StatusSeeOther,
					Headers:    []*muxTypesResponse.HeaderEntry{{Name: "Location", Value: "/elsewhere"}},
				}, nil
			},
			expectedStatusCode: http.StatusSeeOther,
			expectedBody:       "",
			expectedHeaderName: "Location",
			expectedHeader:     "/elsewhere",
		},
		{
			name: "a status with a body replaces the static content",
			handler: func(*http.Request, []byte) (*muxTypesResponse.Response, *response_error.ResponseError) {
				return &muxTypesResponse.Response{
					StatusCode: http.StatusOK,
					Body:       []byte("from the handler"),
					Headers:    []*muxTypesResponse.HeaderEntry{{Name: "Content-Type", Value: "text/plain"}},
				}, nil
			},
			expectedStatusCode: http.StatusOK,
			expectedBody:       "from the handler",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			mux := &Mux{}
			mux.Add(
				&endpoint.Endpoint{
					Path:                 "/page",
					Method:               http.MethodGet,
					Public:               true,
					DisableFetchMetadata: true,
					Handler:              testCase.handler,
					StaticContent: &muxTypesStaticContent.StaticContent{
						StaticContentData: muxTypesStaticContent.StaticContentData{
							Data: []byte(staticContentBody),
							Headers: []*muxTypesResponse.HeaderEntry{
								{Name: "Content-Type", Value: "text/html"},
							},
						},
					},
				},
			)

			httpServer := httptest.NewServer(mux)
			defer httpServer.Close()

			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, httpServer.URL+"/page", nil)
			if err != nil {
				t.Fatalf("http new request: %v", err)
			}

			client := &http.Client{
				CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
			}
			response, err := client.Do(request)
			if err != nil {
				t.Fatalf("client do: %v", err)
			}
			defer func() { _ = response.Body.Close() }()

			if response.StatusCode != testCase.expectedStatusCode {
				t.Errorf("got status %d, expected %d", response.StatusCode, testCase.expectedStatusCode)
			}

			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("io read all: %v", err)
			}
			if string(body) != testCase.expectedBody {
				t.Errorf("got body %q, expected %q", body, testCase.expectedBody)
			}

			if testCase.expectedHeaderName == "" {
				return
			}
			if header := response.Header.Get(testCase.expectedHeaderName); header != testCase.expectedHeader {
				t.Errorf("got %s %q, expected %q", testCase.expectedHeaderName, header, testCase.expectedHeader)
			}
		})
	}
}
