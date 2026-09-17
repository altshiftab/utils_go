package openapi

// ProblemDetailSchemaName is what the problem detail schema is called in a document's components.
const ProblemDetailSchemaName = "ProblemDetail"

// BodyValidationProblemDetailSchemaName is what the problem detail carrying validation errors is
// called in a document's components.
const BodyValidationProblemDetailSchemaName = "BodyValidationProblemDetail"

// ProblemDetailContentType is what a problem detail is served as.
const ProblemDetailContentType = "application/problem+json"

// problemDetailSchema describes an RFC 9457 problem detail as it appears on the wire.
//
// It is written by hand rather than reflected from problem_detail.Detail, because the type and the
// wire disagree: the type keeps extension members in a map field, and its MarshalJSON lifts them to
// the top level. A reflected schema would document an "extension" object no response ever carries,
// and would close the object against the members that actually appear in it.
//
// The pairing is held together by a test, which marshals a real problem detail carrying an
// extension and validates the bytes against this schema. Either side drifting fails it.
func problemDetailSchema() map[string]any {
	return map[string]any{
		schemaKeyType:        schemaTypeObject,
		schemaKeyDescription: "An RFC 9457 problem detail.",
		schemaKeyProperties: map[string]any{
			// A member of the problem detail, which happens to be spelled as the schema keyword is.
			"type": map[string]any{
				schemaKeyType:        schemaTypeString,
				schemaKeyFormat:      "uri-reference",
				schemaKeyDescription: "Identifies the problem type.",
			},
			"title": map[string]any{
				schemaKeyType:        schemaTypeString,
				schemaKeyDescription: "A short, human-readable summary of the problem type.",
			},
			"status": map[string]any{
				schemaKeyType:        schemaTypeInteger,
				schemaKeyDescription: "The HTTP status code.",
			},
			"detail": map[string]any{
				schemaKeyType:        schemaTypeString,
				schemaKeyDescription: "An explanation specific to this occurrence of the problem.",
			},
			"instance": map[string]any{
				schemaKeyType:        schemaTypeString,
				schemaKeyFormat:      "uri-reference",
				schemaKeyDescription: "Identifies this occurrence of the problem.",
			},
		},
		// Extension members are lifted to the top level as they are written, so the object is open
		// by definition: what a particular problem adds is not knowable from the type.
		schemaKeyAdditionalProperties: true,
	}
}

// bodyValidationProblemDetailSchema describes the problem detail a body that fails schema
// validation is answered with, which carries the failures as an extension member.
func bodyValidationProblemDetailSchema(refPrefix string) map[string]any {
	return map[string]any{
		schemaKeyDescription: "A problem detail carrying the schema validation failures.",
		schemaKeyAllOf: []any{
			map[string]any{schemaKeyRef: refPrefix + ProblemDetailSchemaName},
			map[string]any{
				schemaKeyType: schemaTypeObject,
				schemaKeyProperties: map[string]any{
					"errors": map[string]any{
						schemaKeyType:        schemaTypeArray,
						schemaKeyDescription: "The validation failures, in JSON Schema basic output format.",
						schemaKeyItems: map[string]any{
							schemaKeyType: schemaTypeObject,
							schemaKeyProperties: map[string]any{
								"error": map[string]any{
									schemaKeyType:        schemaTypeString,
									schemaKeyDescription: "What failed.",
								},
								"keywordLocation": map[string]any{
									schemaKeyType:        schemaTypeString,
									schemaKeyDescription: "The location of the keyword that failed, in the schema.",
								},
								"instanceLocation": map[string]any{
									schemaKeyType:        schemaTypeString,
									schemaKeyDescription: "The location of the value that failed, in the body.",
								},
							},
							schemaKeyRequired:             []string{"error", "keywordLocation", "instanceLocation"},
							schemaKeyAdditionalProperties: false,
						},
					},
				},
				schemaKeyRequired:             []string{"errors"},
				schemaKeyAdditionalProperties: true,
			},
		},
	}
}
