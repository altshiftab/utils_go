package json_body_parser

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	altshiftLog "github.com/altshiftab/utils_go/pkg/log"
)

type payload struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// parseWith erases a parser's type, so that parsers of different types share a table.
func parseWith[T any](parser *Parser[T]) func([]byte) (any, *response_error.ResponseError) {
	return func(body []byte) (any, *response_error.ResponseError) {
		return parser.Parse(nil, body)
	}
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

	t.Run("null into a pointer or an interface is a 422 client error", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name  string
			parse func([]byte) (any, *response_error.ResponseError)
			body  []byte
		}{
			{name: "pointer", parse: parseWith(New[*payload]()), body: []byte(`null`)},
			{name: "pointer, surrounding whitespace", parse: parseWith(New[*payload]()), body: []byte(" \n null \t")},
			{name: "interface", parse: parseWith(New[any]()), body: []byte(`null`)},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()

				_, responseError := testCase.parse(testCase.body)
				if responseError == nil || responseError.ProblemDetail == nil {
					t.Fatalf("%s: expected a problem detail, got %#v", testCase.name, responseError)
				}
				if responseError.ServerError != nil {
					t.Fatalf("%s: expected no server error, got %#v", testCase.name, responseError.ServerError)
				}
				if !errors.Is(responseError.ClientError, altshiftErrors.ErrParseError) {
					t.Fatalf("%s: expected a client parse error, got %#v", testCase.name, responseError.ClientError)
				}
				if responseError.ProblemDetail.Status != http.StatusUnprocessableEntity {
					t.Fatalf("%s: expected 422, got %d", testCase.name, responseError.ProblemDetail.Status)
				}
			})
		}
	})

	// Only a nil pointer or interface is refused; the zero value of any other kind is a value,
	// null included where it unmarshals into one.
	t.Run("zero values are kept", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name     string
			parse    func([]byte) (any, *response_error.ResponseError)
			body     []byte
			expected any
		}{
			{name: "pointer, empty object", parse: parseWith(New[*payload]()), body: []byte(`{}`), expected: &payload{}},
			{name: "struct, empty object", parse: parseWith(New[payload]()), body: []byte(`{}`), expected: payload{}},
			{name: "struct, null", parse: parseWith(New[payload]()), body: []byte(`null`), expected: payload{}},
			{name: "map, empty object", parse: parseWith(New[map[string]any]()), body: []byte(`{}`), expected: map[string]any{}},
			{name: "map, null", parse: parseWith(New[map[string]any]()), body: []byte(`null`), expected: map[string]any(nil)},
			{name: "slice, empty array", parse: parseWith(New[[]any]()), body: []byte(`[]`), expected: []any{}},
			{name: "slice, null", parse: parseWith(New[[]any]()), body: []byte(`null`), expected: []any(nil)},
			{name: "int, zero", parse: parseWith(New[int]()), body: []byte(`0`), expected: 0},
			{name: "string, empty", parse: parseWith(New[string]()), body: []byte(`""`), expected: ""},
			{name: "bool, false", parse: parseWith(New[bool]()), body: []byte(`false`), expected: false},
			{name: "interface, empty object", parse: parseWith(New[any]()), body: []byte(`{}`), expected: map[string]any{}},
			{name: "interface, false", parse: parseWith(New[any]()), body: []byte(`false`), expected: false},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()

				result, responseError := testCase.parse(testCase.body)
				if responseError != nil {
					t.Fatalf("%s: unexpected error: %#v", testCase.name, responseError)
				}
				if !reflect.DeepEqual(result, testCase.expected) {
					t.Fatalf("%s: expected %#v, got %#v", testCase.name, testCase.expected, result)
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
