package errors

import (
	"errors"
)

var (
	// ErrDuplicateOperationId is for two operations claiming one identifier. The specification
	// requires uniqueness, and a generated client keyed by it would lose one of them.
	ErrDuplicateOperationId = errors.New("duplicate operation id")
	// ErrAmbiguousInput is for a hint naming both an input type and a url input type on a method
	// that carries no body, where both would mean query parameters and nothing says which was
	// meant.
	ErrAmbiguousInput = errors.New("ambiguous input: input type and url input type on a body-less method")
	// ErrUndescribedBody is for an endpoint whose loader accepts a body that no hint describes.
	ErrUndescribedBody = errors.New("body accepted but not described by the hint")
	// ErrBodyNotAccepted is for a hint describing a request body the mux will not read, the
	// endpoint having no loader or a loader that forbids one.
	ErrBodyNotAccepted = errors.New("hint describes a body the endpoint does not accept")
	// ErrQueryMethodUnsupported is for the HTTP QUERY method under a specification version with no
	// field to put it in. There is no honest alternative encoding: documenting it as a POST would
	// send a client to a method the endpoint does not serve.
	ErrQueryMethodUnsupported = errors.New("the query method needs openapi 3.2 or later")
	// ErrUnsupportedMethod is for a method the document has no field for.
	ErrUnsupportedMethod = errors.New("unsupported method")
	// ErrMissingSecurityScheme is for a gated endpoint in a document that declares no way to
	// authenticate. Documentation that cannot tell a reader how to reach an endpoint is worse than
	// none, because it reads as though the endpoint needed nothing.
	ErrMissingSecurityScheme = errors.New("no security scheme declared for a non-public endpoint")
	// ErrBinaryOutputWithOutputType is for a hint naming an output type alongside a content type
	// whose body is bytes rather than a structure.
	ErrBinaryOutputWithOutputType = errors.New("output type with a binary output content type")
	// ErrOptionalBinaryOutput is for an optional body that is also a binary one.
	ErrOptionalBinaryOutput = errors.New("optional binary output")
	// ErrReservedSchemaName is for an endpoint type claiming a name the document writes itself.
	ErrReservedSchemaName = errors.New("reserved schema name")
	// ErrUnsupportedVersion is for a specification version the generator does not write.
	ErrUnsupportedVersion = errors.New("unsupported openapi version")
	// ErrMissingInfo is for a document with no title or no version, which the specification
	// requires and no endpoint can supply.
	ErrMissingInfo = errors.New("missing info title or version")
	// ErrDocumentDrift is for a committed document that no longer matches the endpoints it was
	// generated from.
	ErrDocumentDrift = errors.New("document differs from the endpoints")
)
