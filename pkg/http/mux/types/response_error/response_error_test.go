package response_error

import (
	"errors"
	"net/http"
	"testing"

	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
)

var errBoom = errors.New("boom")

func TestConvertProblemDetail(t *testing.T) {
	t.Parallel()

	detail := problem_detail.New(http.StatusBadRequest)

	testCases := []struct {
		name            string
		detail          *problem_detail.Detail
		negotiation     *altshiftHttpTypes.ContentNegotiation
		wantContentType string
		wantEmpty       bool
	}{
		{name: "nil detail", detail: nil, wantEmpty: true},
		{name: "default json without negotiation", detail: detail, wantContentType: "application/problem+json"},
		{
			name:            "unmatched negotiation falls back to json",
			detail:          detail,
			negotiation:     &altshiftHttpTypes.ContentNegotiation{NegotiatedAccept: "application/problem+json"},
			wantContentType: "application/problem+json",
		},
		{
			name:            "xml",
			detail:          detail,
			negotiation:     &altshiftHttpTypes.ContentNegotiation{NegotiatedAccept: "application/xml"},
			wantContentType: "application/problem+xml",
		},
		{
			name:            "problem xml",
			detail:          detail,
			negotiation:     &altshiftHttpTypes.ContentNegotiation{NegotiatedAccept: "application/problem+xml"},
			wantContentType: "application/problem+xml",
		},
		{
			name:            "text plain",
			detail:          detail,
			negotiation:     &altshiftHttpTypes.ContentNegotiation{NegotiatedAccept: "text/plain"},
			wantContentType: "text/plain",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			data, contentType, err := ConvertProblemDetail(testCase.detail, testCase.negotiation)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if testCase.wantEmpty {
				if data != nil || contentType != "" {
					t.Fatalf("expected empty result, got data=%q contentType=%q", data, contentType)
				}
				return
			}

			if contentType != testCase.wantContentType {
				t.Fatalf("expected content type %q, got %q", testCase.wantContentType, contentType)
			}
			if len(data) == 0 {
				t.Fatal("expected non-empty data")
			}
		})
	}
}

func TestResponseErrorType(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		re   *ResponseError
		want ResponseErrorType
	}{
		{name: "server error", re: &ResponseError{ServerError: errBoom}, want: ResponseErrorType_ServerError},
		{name: "client error", re: &ResponseError{ClientError: errBoom}, want: ResponseErrorType_ClientError},
		{name: "4xx problem detail", re: &ResponseError{ProblemDetail: problem_detail.New(http.StatusBadRequest)}, want: ResponseErrorType_ClientError},
		{name: "5xx problem detail", re: &ResponseError{ProblemDetail: problem_detail.New(http.StatusInternalServerError)}, want: ResponseErrorType_ServerError},
		{name: "non-error status", re: &ResponseError{ProblemDetail: problem_detail.New(http.StatusOK)}, want: ResponseErrorType_Invalid},
		{name: "empty", re: &ResponseError{}, want: ResponseErrorType_Invalid},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := testCase.re.Type(); got != testCase.want {
				t.Fatalf("expected %v, got %v", testCase.want, got)
			}
		})
	}
}

func TestGetEffectiveProblemDetail(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		re         *ResponseError
		wantStatus int
		wantErr    bool
	}{
		{name: "explicit problem detail", re: &ResponseError{ProblemDetail: problem_detail.New(http.StatusTeapot)}, wantStatus: http.StatusTeapot},
		{name: "server error defaults to 500", re: &ResponseError{ServerError: errBoom}, wantStatus: http.StatusInternalServerError},
		{name: "client error defaults to 400", re: &ResponseError{ClientError: errBoom}, wantStatus: http.StatusBadRequest},
		{name: "both errors is unusable", re: &ResponseError{ClientError: errBoom, ServerError: errBoom}, wantErr: true},
		{name: "empty is unusable", re: &ResponseError{}, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			detail, err := testCase.re.GetEffectiveProblemDetail()
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if detail == nil || detail.Status != testCase.wantStatus {
				t.Fatalf("expected status %d, got %#v", testCase.wantStatus, detail)
			}
		})
	}
}

func TestMakeResponse(t *testing.T) {
	t.Parallel()

	t.Run("nil problem detail is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := (&ResponseError{}).MakeResponse(nil); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("zero status is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := (&ResponseError{ProblemDetail: &problem_detail.Detail{}}).MakeResponse(nil); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("produces a json response", func(t *testing.T) {
		t.Parallel()
		responseError := &ResponseError{ProblemDetail: problem_detail.New(http.StatusBadRequest)}
		response, err := responseError.MakeResponse(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusBadRequest)
		}
		if len(response.Body) == 0 {
			t.Error("expected a body")
		}

		var hasContentType bool
		for _, header := range response.Headers {
			if header != nil && header.Name == "Content-Type" {
				hasContentType = true
			}
		}
		if !hasContentType {
			t.Error("expected a Content-Type header")
		}
	})

	t.Run("location header forces a redirect status", func(t *testing.T) {
		t.Parallel()
		responseError := &ResponseError{
			ProblemDetail: problem_detail.New(http.StatusUnauthorized),
			Headers:       []*muxTypesResponse.HeaderEntry{{Name: "Location", Value: "/login"}},
		}
		response, err := responseError.MakeResponse(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if response.StatusCode != http.StatusSeeOther {
			t.Fatalf("expected 303, got %d", response.StatusCode)
		}
	})
}
