package testing

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"

	motmedelContext "github.com/altshiftab/utils_go/pkg/context"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/utils"
	"github.com/altshiftab/utils_go/pkg/testing/cmp"
)

const ExpectedBodyNonEmpty = "*non-empty-body"

type Args struct {
	Method                    string
	Path                      string
	Headers                   [][2]string
	Body                      []byte
	ExpectedStatusCode        int
	ExpectedHeaders           [][2]string
	ExpectedHeadersPresent    []string
	ExpectedHeadersNotPresent []string
	ExpectedBody              []byte
	ExpectedProblemDetail     *problem_detail.Detail
	ExpectedClientDoError     error
}

func TestArgs(t *testing.T, args *Args, serverUrl string) {
	t.Helper()

	if args == nil {
		t.Fatalf("args is nil")
	}

	if serverUrl == "" {
		t.Fatalf("server url is empty")
	}

	var requestBody io.Reader
	if testCaseBody := args.Body; len(testCaseBody) != 0 {
		requestBody = bytes.NewReader(testCaseBody)
	}

	request, err := http.NewRequestWithContext(t.Context(), args.Method, serverUrl+args.Path, requestBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	for _, header := range args.Headers {
		request.Header.Add(header[0], header[1])
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	response, err := client.Do(request)
	if err != nil {
		if args.ExpectedClientDoError != nil {
			if errors.Is(err, args.ExpectedClientDoError) {
				return
			}
			t.Fatalf("http client do: %v", err)
		}
	}
	if response == nil {
		t.Fatalf("http client do returned nil response")
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			slog.WarnContext(
				motmedelContext.WithError(
					t.Context(),
					motmedelErrors.NewWithTrace(fmt.Errorf("response body close: %w", err)),
				),
				"An error occurred when closing the response body.",
			)
		}
	}()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("io read all response body: %v", err)
	}

	if response.StatusCode != args.ExpectedStatusCode {
		t.Errorf("got status code %d, expected %d", response.StatusCode, args.ExpectedStatusCode)
	}

	if expectedHeaders := args.ExpectedHeaders; len(expectedHeaders) != 0 {
		responseHeader := response.Header
		if responseHeader == nil {
			t.Fatalf("response header is nil")
		}

		for _, header := range expectedHeaders {
			headerName := header[0]
			expectedHeaderValue := header[1]
			headerValue, err := utils.GetSingleHeader(headerName, responseHeader)
			if err != nil {
				if errors.Is(err, motmedelHttpErrors.ErrMissingHeader) {
					t.Errorf("expected header %q to be present", headerName)
				} else if errors.Is(err, motmedelHttpErrors.ErrMultipleHeaderValues) {
					t.Errorf("multiple header values for header %q", headerName)
				} else {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if headerValue != expectedHeaderValue {
				t.Errorf("got %q, expected header %q to be %q", headerValue, headerName, expectedHeaderValue)
			}
		}
	}

	if expectedHeadersPresent := args.ExpectedHeadersPresent; len(expectedHeadersPresent) != 0 {
		for _, header := range expectedHeadersPresent {
			if _, err := utils.GetSingleHeader(header, response.Header); err != nil {
				if errors.Is(err, motmedelHttpErrors.ErrMissingHeader) {
					t.Errorf("expected header %q to be present", header)
				} else if errors.Is(err, motmedelHttpErrors.ErrMultipleHeaderValues) {
					continue
				} else {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		}
	}

	if expectedHeadersNotPresent := args.ExpectedHeadersNotPresent; len(expectedHeadersNotPresent) != 0 {
		for _, header := range expectedHeadersNotPresent {
			if _, ok := response.Header[header]; ok {
				t.Errorf("expected header %q to not be present", header)
			}
		}
	}

	if expectedProblemDetail := args.ExpectedProblemDetail; expectedProblemDetail != nil {
		var problemDetail *problem_detail.Detail
		if err := json.Unmarshal(responseBody, &problemDetail); err != nil {
			t.Fatalf("json unmarshal response body: %v", err)
		}

		opts := []cmp.Option{
			cmp.IgnoreFields(problem_detail.Detail{}, "Type"),
			cmp.IgnoreFields(problem_detail.Detail{}, "Instance"),
			cmp.EquateEmpty(),
		}

		expectedStatusCode := args.ExpectedStatusCode
		expectedProblemDetail.Title = http.StatusText(expectedStatusCode)
		expectedProblemDetail.Status = expectedStatusCode

		if diff := cmp.Diff(expectedProblemDetail, problemDetail, opts...); diff != "" {
			t.Errorf("problem detail mismatch (-expected +got):\n%s", diff)
		}
	} else {
		if string(args.ExpectedBody) == ExpectedBodyNonEmpty {
			if len(responseBody) == 0 {
				t.Errorf("expected non-empty response body, got empty body")
			}
		} else {
			if !bytes.Equal(responseBody, args.ExpectedBody) {
				t.Errorf("got response body %q, expected response body %q", responseBody, args.ExpectedBody)
			}
		}
	}
}
