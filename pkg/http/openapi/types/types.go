// Package types holds the OpenAPI document model: the objects a document is made of, shaped so
// that marshalling one produces the document.
//
// Schemas are map[string]any rather than a type of their own. An OpenAPI 3.1 schema is a JSON
// Schema 2020-12 schema, which is what pkg/type_export/jsonschema produces, and a model for it here
// would be a second, poorer spelling of a vocabulary that is already open-ended by design.
//
// Field order is the order the members appear in the marshalled document. It follows the order the
// specification presents them in, so that a document read by a person opens with what identifies it.
package types

// Document is an OpenAPI document.
type Document struct {
	Openapi    string                `json:"openapi"`
	Info       *Info                 `json:"info"`
	Servers    []*Server             `json:"servers,omitzero"`
	Paths      map[string]*PathItem  `json:"paths"`
	Components *Components           `json:"components,omitzero"`
	Security   []map[string][]string `json:"security,omitzero"`
	Tags       []*Tag                `json:"tags,omitzero"`
}

// Info identifies the API a document describes.
type Info struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Summary     string `json:"summary,omitzero"`
	Description string `json:"description,omitzero"`
}

// Server is a base URL the API is served at.
type Server struct {
	Url         string `json:"url"`
	Description string `json:"description,omitzero"`
}

// Tag names a group operations are gathered under.
type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitzero"`
}

// PathItem holds the operations served at one path.
//
// Query is the field OpenAPI 3.2 added for the HTTP QUERY method -- a read whose parameters travel
// in the body. It is a fixed field there rather than an entry in additionalOperations, and has no
// place at all in 3.1.
type PathItem struct {
	Get     *Operation `json:"get,omitzero"`
	Put     *Operation `json:"put,omitzero"`
	Post    *Operation `json:"post,omitzero"`
	Delete  *Operation `json:"delete,omitzero"`
	Options *Operation `json:"options,omitzero"`
	Head    *Operation `json:"head,omitzero"`
	Patch   *Operation `json:"patch,omitzero"`
	Trace   *Operation `json:"trace,omitzero"`
	Query   *Operation `json:"query,omitzero"`
}

// Operation is one method at one path.
type Operation struct {
	OperationId string                `json:"operationId"`
	Summary     string                `json:"summary,omitzero"`
	Description string                `json:"description,omitzero"`
	Tags        []string              `json:"tags,omitzero"`
	Deprecated  bool                  `json:"deprecated,omitzero"`
	Parameters  []*Parameter          `json:"parameters,omitzero"`
	RequestBody *RequestBody          `json:"requestBody,omitzero"`
	Responses   map[string]*Response  `json:"responses"`
	Security    []map[string][]string `json:"security,omitzero"`
}

// Parameter is one input carried outside the body. Only query parameters are produced here: the mux
// matches literal paths, so there is nothing to template and no path parameter to describe.
type Parameter struct {
	Name        string         `json:"name"`
	In          string         `json:"in"`
	Required    bool           `json:"required,omitzero"`
	Description string         `json:"description,omitzero"`
	Schema      map[string]any `json:"schema,omitzero"`
}

// RequestBody is what a request carries, by content type.
type RequestBody struct {
	Description string                `json:"description,omitzero"`
	Required    bool                  `json:"required,omitzero"`
	Content     map[string]*MediaType `json:"content"`
}

// Response is one status a request can be answered with. Description is required of it by the
// specification, even where there is nothing to add to the status itself.
type Response struct {
	Description string                `json:"description"`
	Content     map[string]*MediaType `json:"content,omitzero"`
}

// MediaType is one content type a body may be written in.
//
// A media type with no schema is how bytes with no JSON structure are described -- a PDF, a
// pre-encoded JSON document served as-is, a COSE envelope whose plaintext is not the wire.
type MediaType struct {
	Schema map[string]any `json:"schema,omitzero"`
}

// Components holds what the document's operations refer to rather than repeat.
type Components struct {
	Schemas         map[string]any             `json:"schemas,omitzero"`
	SecuritySchemes map[string]*SecurityScheme `json:"securitySchemes,omitzero"`
}

// SecurityScheme is one way a request proves who is making it.
//
// It cannot be derived from an endpoint: an endpoint says whether a session is required, and the
// service says what a session is. The fields are the specification's, so that a scheme is declared
// the way it is documented -- apiKey with In and Name for a cookie or a header, http with Scheme
// and BearerFormat for a bearer token.
type SecurityScheme struct {
	Type         string `json:"type"`
	Description  string `json:"description,omitzero"`
	Name         string `json:"name,omitzero"`
	In           string `json:"in,omitzero"`
	Scheme       string `json:"scheme,omitzero"`
	BearerFormat string `json:"bearerFormat,omitzero"`
}
