package cbor_schema_body_parser

import (
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cbor"
	cborSchema "github.com/altshiftab/utils_go/pkg/cbor/schema"
)

type testDocument struct {
	FileName string `cborschema:"file_name,minlength:1"`
	Content  []byte `cborschema:"content,minlength:1"`
}

type testOrder struct {
	Name         string          `cborschema:"name"`
	EmailAddress string          `cborschema:"email_address,format:email"`
	Documents    []*testDocument `cborschema:"documents,optional"`
}

func validOrderData(t *testing.T) []byte {
	t.Helper()

	data, err := cbor.Encode(
		map[any]any{
			"name":          "Meriadoc",
			"email_address": "meriadoc.brandybuck@buckland.example",
			"documents": []any{
				map[any]any{"file_name": "a.pdf", "content": []byte{0x25, 0x50, 0x44, 0x46}},
			},
		},
	)
	if err != nil {
		t.Fatalf("cbor encode: %v", err)
	}

	return data
}

func newParser(t *testing.T) *Parser[*testOrder] {
	t.Helper()

	parser, err := New[*testOrder]()
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	return parser
}

func TestParseValid(t *testing.T) {
	t.Parallel()

	order, responseError := newParser(t).Parse(nil, validOrderData(t))
	if responseError != nil {
		t.Fatalf("parse: %v", responseError)
	}

	if order == nil || order.Name != "Meriadoc" || len(order.Documents) != 1 {
		t.Errorf("parse: unexpected result %#v", order)
	}
	if order.Documents[0].FileName != "a.pdf" || len(order.Documents[0].Content) != 4 {
		t.Errorf("parse: unexpected document %#v", order.Documents[0])
	}
}

func TestParseMalformedBody(t *testing.T) {
	t.Parallel()

	_, responseError := newParser(t).Parse(nil, []byte{0x9f})
	if responseError == nil {
		t.Fatal("expected a response error")
	}

	problemDetail := responseError.ProblemDetail
	if problemDetail == nil || problemDetail.Status != http.StatusBadRequest {
		t.Errorf("expected a 400 problem detail, got %#v", problemDetail)
	}
	if responseError.ClientError == nil {
		t.Error("expected a client error")
	}
}

func TestParseInvalidBody(t *testing.T) {
	t.Parallel()

	data, err := cbor.Encode(
		map[any]any{
			"name":          "Meriadoc",
			"email_address": "not-an-email",
			"documents":     []any{map[any]any{"file_name": "a.pdf", "content": []byte{}}},
			"unknown":       "value",
		},
	)
	if err != nil {
		t.Fatalf("cbor encode: %v", err)
	}

	_, responseError := newParser(t).Parse(nil, data)
	if responseError == nil {
		t.Fatal("expected a response error")
	}

	problemDetail := responseError.ProblemDetail
	if problemDetail == nil || problemDetail.Status != http.StatusUnprocessableEntity {
		t.Fatalf("expected a 422 problem detail, got %#v", problemDetail)
	}

	issues, ok := problemDetail.Extension["errors"].([]*cborSchema.Issue)
	if !ok {
		t.Fatal("expected an errors extension with issues")
	}

	// Invalid email, empty document content, and an unknown key.
	if len(issues) != 3 {
		t.Errorf("expected 3 issues, got %d: %v", len(issues), issues)
	}
}
