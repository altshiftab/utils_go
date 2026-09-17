package openapi

// JSON Schema keywords the document writes. They are named here rather than spelled at each use so
// that a keyword is one string throughout, and a mistyped one is a compile error.
const (
	schemaKeyType                 = "type"
	schemaKeyFormat               = "format"
	schemaKeyItems                = "items"
	schemaKeyMinItems             = "minItems"
	schemaKeyMaxItems             = "maxItems"
	schemaKeyProperties           = "properties"
	schemaKeyRequired             = "required"
	schemaKeyDescription          = "description"
	schemaKeyAdditionalProperties = "additionalProperties"
	schemaKeyAllOf                = "allOf"
	schemaKeyRef                  = "$ref"
)

// JSON Schema type names, as the query extractor's accepted kinds map onto them.
const (
	schemaTypeString  = "string"
	schemaTypeInteger = "integer"
	schemaTypeNumber  = "number"
	schemaTypeBoolean = "boolean"
	schemaTypeArray   = "array"
	schemaTypeObject  = "object"
)
