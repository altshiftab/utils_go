package js_error_report_test

import (
	jsonv2 "encoding/json/v2"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/js_error_report"
	altshiftJsonSchema "github.com/altshiftab/utils_go/pkg/json/schema"
)

// A report is built by a page out of whatever it was handed, so the schema has to take what an
// engine actually puts in an error. The one that matters is a stack that is there and empty:
// nothing in a page can fill it in, and a report refused is an error nothing anywhere records.
func TestSchemaTakesWhatAPageReports(t *testing.T) {
	t.Parallel()

	baseSchema, err := altshiftJsonSchema.NewFromType[*js_error_report.BaseErrorBody]()
	if err != nil {
		t.Fatalf("new from type (base error body): %v", err)
	}
	if err := baseSchema.Resolve(nil); err != nil {
		t.Fatalf("resolve (base error body): %v", err)
	}

	errorSchema, err := altshiftJsonSchema.NewFromType[*js_error_report.ErrorBody]()
	if err != nil {
		t.Fatalf("new from type (error body): %v", err)
	}
	if err := errorSchema.Resolve(nil); err != nil {
		t.Fatalf("resolve (error body): %v", err)
	}

	testCases := []struct {
		name        string
		schema      *altshiftJsonSchema.Schema
		payload     string
		expectValid bool
	}{
		{
			// Verbatim what Firefox posted when a media element refused to play: the
			// exception is the browser's own, so no frame of the page's made it and the
			// stack it carries is empty.
			name:        "the DOMException a media element rejects play() with",
			schema:      baseSchema,
			payload:     `{"error":{"stack":"","name":"NotSupportedError","message":"The media resource indicated by the src attribute or assigned media provider object was not suitable.","code":9},"type":"DOMException"}`,
			expectValid: true,
		},
		{
			name:        "a rejection whose reason was thrown by the page, stack and all",
			schema:      baseSchema,
			payload:     `{"error":{"stack":"at f (https://example.com/a.js:1:1)","name":"TypeError","message":"x is not a function"},"type":"TypeError"}`,
			expectValid: true,
		},
		{
			// `Promise.reject()` carries no reason, and the page says only what it found.
			name:        "a rejection with no reason to report",
			schema:      baseSchema,
			payload:     `{"type":"undefined"}`,
			expectValid: true,
		},
		{
			// `new Error()` says nothing, and what it says is still worth having: the
			// name, the stack and the fact of it.
			name:        "an error made without a message",
			schema:      baseSchema,
			payload:     `{"error":{"message":"","name":"Error","stack":"at f (https://example.com/a.js:1:1)"},"type":"Error"}`,
			expectValid: true,
		},
		{
			name:        "a report that says nothing about what it reports",
			schema:      baseSchema,
			payload:     `{"error":{"name":"TypeError"}}`,
			expectValid: false,
		},
		{
			// The same details reach the other endpoint, and the same stack empties it.
			name:        "an error event carrying an exception the browser made",
			schema:      errorSchema,
			payload:     `{"colno":0,"filename":"https://example.com/a.js","lineno":12,"message":"failed","type":"DOMException","error":{"stack":"","name":"NotSupportedError","message":"failed","code":9}}`,
			expectValid: true,
		},
		{
			// A muted error: an exception from another origin's script, which a page is
			// told nothing about beyond that there was one. No location, and the message
			// the specification dictates.
			name:        "the little a cross-origin script's exception is reported as",
			schema:      errorSchema,
			payload:     `{"colno":0,"filename":"","lineno":0,"message":"Script error.","type":"object"}`,
			expectValid: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var instance any
			if err := jsonv2.Unmarshal([]byte(testCase.payload), &instance); err != nil {
				t.Fatalf("json unmarshal: %v", err)
			}

			err := testCase.schema.Validate(instance)
			if testCase.expectValid && err != nil {
				t.Errorf("expected the report to validate, got %v", err)
			}
			if !testCase.expectValid && err == nil {
				t.Error("expected the report to be refused")
			}
		})
	}
}
