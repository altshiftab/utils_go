package mux

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	altshiftHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	muxResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	muxResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

func TestMux_MaxStoredResponseBodySize(t *testing.T) {
	t.Parallel()

	body := bytes.Repeat([]byte("x"), 100)

	testCases := []struct {
		name     string
		limit    int
		wantKept int
	}{
		{name: "the whole body is kept by default", limit: 0, wantKept: 100},
		{name: "a limit keeps the start", limit: 8, wantKept: 8},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			mux := New(&endpointPkg.Endpoint{
				Path:   "/x",
				Method: http.MethodGet,
				Public: true,
				Handler: func(*http.Request, []byte) (*muxResponse.Response, *muxResponseError.ResponseError) {
					return &muxResponse.Response{Body: body}, nil
				},
			})
			mux.MaxStoredResponseBodySize = testCase.limit

			var kept, size int
			mux.DoneCallback = func(ctx context.Context) {
				if httpContext, ok := ctx.Value(altshiftHttpContext.HttpContextContextKey).(*altshiftHttpTypes.HttpContext); ok && httpContext != nil {
					kept, size = len(httpContext.ResponseBody), httpContext.ResponseBodySize
				}
			}

			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))

			if !bytes.Equal(recorder.Body.Bytes(), body) {
				t.Fatalf("the client got %d bytes, want %d", recorder.Body.Len(), len(body))
			}
			if kept != testCase.wantKept {
				t.Errorf("kept %d bytes, want %d", kept, testCase.wantKept)
			}
			if size != len(body) {
				t.Errorf("size = %d, want %d", size, len(body))
			}
		})
	}
}
