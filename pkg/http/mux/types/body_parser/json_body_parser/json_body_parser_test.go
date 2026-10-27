package json_body_parser

import (
	jsonv2 "encoding/json/v2"
	"fmt"
	"net/http"
	"strings"
	"testing"

	altshiftLog "github.com/altshiftab/utils_go/pkg/log"
)

type payload struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestParse(t *testing.T) {
	t.Parallel()

	parser := New[payload]()

	t.Run("valid body", func(t *testing.T) {
		t.Parallel()
		result, responseError := parser.Parse(nil, []byte(`{"name":"alice","age":30}`))
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if result.Name != "alice" || result.Age != 30 {
			t.Fatalf("unexpected result: %#v", result)
		}
	})

	t.Run("semantic errors are 422 client errors", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name string
			body []byte
		}{
			{name: "type mismatch", body: []byte(`{"age":"not-a-number"}`)},
			{name: "unknown member", body: []byte(`{"name":"alice","extra":1}`)},
			{name: "case-mismatched member", body: []byte(`{"Name":"alice"}`)},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()
				_, responseError := parser.Parse(nil, testCase.body)
				if responseError == nil || responseError.ProblemDetail == nil {
					t.Fatalf("expected a problem detail, got %#v", responseError)
				}
				if responseError.ClientError == nil {
					t.Fatalf("expected a client error, got %#v", responseError)
				}
				if responseError.ServerError != nil {
					t.Fatalf("expected no server error, got %#v", responseError.ServerError)
				}
				if responseError.ProblemDetail.Status != http.StatusUnprocessableEntity {
					t.Fatalf("expected 422, got %d", responseError.ProblemDetail.Status)
				}
			})
		}
	})

	// A body that fails to parse is not attached to the error, which the log writes out apart from
	// the masks the request log applies to the body.
	t.Run("the body is not in the error", func(t *testing.T) {
		t.Parallel()

		const secret = "person@example.com"

		testCases := []struct {
			name string
			body []byte
		}{
			{name: "type mismatch", body: []byte(`{"name":["` + secret + `"]}`)},
			{name: "unknown member", body: []byte(`{"name":"alice","extra":"` + secret + `"}`)},
			{name: "malformed", body: []byte(`{"name":"` + secret + `",`)},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()

				_, responseError := parser.Parse(nil, testCase.body)
				if responseError == nil || responseError.ClientError == nil {
					t.Fatalf("%s: expected a client error, got %#v", testCase.name, responseError)
				}

				written := fmt.Sprint((&altshiftLog.ErrorContextExtractor{}).MakeErrorAttrs(responseError.ClientError))
				if strings.Contains(written, secret) {
					t.Errorf("%s: the body is in the error: %s", testCase.name, written)
				}
			})
		}
	})

	t.Run("malformed body is a 400 client error", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name string
			body []byte
		}{
			{name: "garbage", body: []byte(`{malformed`)},
			{name: "truncated", body: []byte(`{"name":"alice"`)},
			{name: "empty", body: []byte(``)},
			{name: "duplicate member", body: []byte(`{"name":"a","name":"b"}`)},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()
				_, responseError := parser.Parse(nil, testCase.body)
				if responseError == nil || responseError.ProblemDetail == nil {
					t.Fatalf("expected a problem detail, got %#v", responseError)
				}
				if responseError.ClientError == nil {
					t.Fatalf("expected a client error, got %#v", responseError)
				}
				if responseError.ServerError != nil {
					t.Fatalf("expected no server error, got %#v", responseError.ServerError)
				}
				if responseError.ProblemDetail.Status != http.StatusBadRequest {
					t.Fatalf("expected 400, got %d", responseError.ProblemDetail.Status)
				}
			})
		}
	})

	t.Run("options can allow unknown members", func(t *testing.T) {
		t.Parallel()

		lenientParser := New[payload](jsonv2.RejectUnknownMembers(false))

		result, responseError := lenientParser.Parse(nil, []byte(`{"name":"alice","extra":1}`))
		if responseError != nil {
			t.Fatalf("unexpected error: %#v", responseError)
		}
		if result.Name != "alice" {
			t.Fatalf("unexpected result: %#v", result)
		}
	})
}
