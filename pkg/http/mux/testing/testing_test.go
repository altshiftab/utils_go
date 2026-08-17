package testing

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hello":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("X-Test", "value")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello"))
		case "/problem":
			detail := &problem_detail.Detail{
				Status: http.StatusBadRequest,
				Title:  http.StatusText(http.StatusBadRequest),
				Detail: "bad",
			}
			data, err := json.Marshal(detail)
			if err != nil {
				t.Errorf("marshal problem detail: %v", err)
				return
			}
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(data)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func TestTestArgs(t *testing.T) {
	t.Parallel()

	server := newServer(t)

	t.Run("status, body, and headers", func(t *testing.T) {
		t.Parallel()
		TestArgs(t, &Args{
			Method:                    http.MethodGet,
			Path:                      "/hello",
			ExpectedStatusCode:        http.StatusOK,
			ExpectedBody:              []byte("hello"),
			ExpectedHeaders:           [][2]string{{"X-Test", "value"}},
			ExpectedHeadersPresent:    []string{"Content-Type"},
			ExpectedHeadersNotPresent: []string{"X-Absent"},
		}, server.URL)
	})

	t.Run("non-empty body sentinel", func(t *testing.T) {
		t.Parallel()
		TestArgs(t, &Args{
			Method:             http.MethodGet,
			Path:               "/hello",
			ExpectedStatusCode: http.StatusOK,
			ExpectedBody:       []byte(ExpectedBodyNonEmpty),
		}, server.URL)
	})

	t.Run("problem detail", func(t *testing.T) {
		t.Parallel()
		TestArgs(t, &Args{
			Method:                http.MethodGet,
			Path:                  "/problem",
			ExpectedStatusCode:    http.StatusBadRequest,
			ExpectedProblemDetail: &problem_detail.Detail{Detail: "bad"},
		}, server.URL)
	})
}
