package openapi

import (
	"encoding/json/v2"
	"net/http"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
	altshiftJsonSchema "github.com/altshiftab/utils_go/pkg/json/schema"
)

// validateAgainst validates the instance against the schema, as a reader of the document would.
func validateAgainst(t *testing.T, schema map[string]any, instance any) error {
	t.Helper()

	schemaData, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}

	parsedSchema, err := altshiftJsonSchema.New(schemaData)
	if err != nil {
		t.Fatal(err)
	}

	return parsedSchema.Validate(instance)
}

// TestProblemDetailSchemaMatchesTheWire is what binds the hand-written schema to the type it
// describes.
//
// The schema is written by hand because problem_detail.Detail's MarshalJSON lifts its extension
// members to the top level, which reflection cannot see. That makes the schema a claim about the
// marshaller rather than about the struct, and a claim is worth only the test under it: this one
// marshals a real problem detail and validates the bytes, so that either side changing fails here
// rather than in a reader's parser.
func TestProblemDetailSchemaMatchesTheWire(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		detail *problem_detail.Detail
	}{
		{
			name:   "status alone",
			detail: problem_detail.New(http.StatusForbidden),
		},
		{
			name: "with a detail",
			detail: problem_detail.New(
				http.StatusBadRequest,
				problem_detail_config.WithDetail("Bad query."),
			),
		},
		{
			name: "with an extension lifted to the top level",
			detail: problem_detail.New(
				http.StatusUnprocessableEntity,
				problem_detail_config.WithDetail("Invalid body."),
				problem_detail_config.WithExtension(map[string]any{
					"errors": []any{"one", "two"},
				}),
			),
		},
		{
			name: "with a type and an instance",
			detail: problem_detail.New(
				http.StatusNotFound,
				problem_detail_config.WithType("https://example.test/not-found"),
				problem_detail_config.WithInstance("/api/project"),
			),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(testCase.detail)
			if err != nil {
				t.Fatal(err)
			}

			var instance any
			if err := json.Unmarshal(data, &instance); err != nil {
				t.Fatal(err)
			}

			if err := validateAgainst(t, problemDetailSchema(), instance); err != nil {
				t.Errorf("%s: %s does not satisfy the documented schema: %v", testCase.name, data, err)
			}
		})
	}
}

// TestProblemDetailSchemaDocumentsNoExtensionMember is the mistake the hand-written schema exists to
// avoid: a schema reflected from the struct would document an "extension" object, which no response
// carries.
func TestProblemDetailSchemaDocumentsNoExtensionMember(t *testing.T) {
	t.Parallel()

	properties, ok := problemDetailSchema()["properties"].(map[string]any)
	if !ok {
		t.Fatal("the problem detail schema has no properties")
	}

	if _, ok := properties["extension"]; ok {
		t.Error("the schema documents an \"extension\" member, which the marshaller never writes")
	}

	for _, name := range []string{"type", "title", "status", "detail", "instance"} {
		if _, ok := properties[name]; !ok {
			t.Errorf("the schema is missing the %q member", name)
		}
	}

	// Extension members arrive as top-level members, so the object cannot be closed against them.
	if problemDetailSchema()["additionalProperties"] != true {
		t.Error("the schema is closed, and extension members would fail it")
	}
}

// TestBodyValidationProblemDetailSchemaMatchesTheWire checks the 422 shape: a problem detail whose
// extension carries the schema failures, in the form the validator produces them.
func TestBodyValidationProblemDetailSchemaMatchesTheWire(t *testing.T) {
	t.Parallel()

	// The shape pkg/json/schema reports a failure in, as json_schema_body_parser passes it on.
	validationErrors := []any{
		map[string]any{
			"error":            "missing property \"id\"",
			"keywordLocation":  "/required",
			"instanceLocation": "",
		},
	}

	detail := problem_detail.New(
		http.StatusUnprocessableEntity,
		problem_detail_config.WithDetail("Invalid body."),
		problem_detail_config.WithExtension(map[string]any{"errors": validationErrors}),
	)

	data, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}

	var instance any
	if err := json.Unmarshal(data, &instance); err != nil {
		t.Fatal(err)
	}

	// The document's schema refers to the problem detail through components, which a reader
	// resolves within the document. Validated on its own here, the reference is inlined instead.
	schema := map[string]any{
		"allOf": []any{
			problemDetailSchema(),
			bodyValidationProblemDetailSchema("")["allOf"].([]any)[1],
		},
	}

	if err := validateAgainst(t, schema, instance); err != nil {
		t.Errorf("%s does not satisfy the documented 422 schema: %v", data, err)
	}
}
