// Package openapi writes the OpenAPI document describing a mux's endpoints: one operation per
// endpoint, taking and returning the types the endpoint declares, so that what the document says
// and what the server does come from one source.
//
// It is the sibling of pkg/http/client_code_generation, which reads the same hints to write a typed
// client. Where the two could disagree -- how an input is carried, what counts as a binary body,
// what an operation is called -- they are made to agree, because a document and a client that
// describe the same endpoint differently are worse than either alone.
//
// # Which endpoints are documented
//
// Those whose hint sets Hint.Documented, and no others. An endpoint is not offered to third parties
// until someone decides it should be, so a service that declares no documented endpoint gets a
// document with no operations rather than one describing everything it happens to serve.
//
// Two further exclusions are not decisions anyone makes: an endpoint with no hint says nothing
// about what it takes or returns, and one serving static content is a document rather than an
// operation.
//
// # Version
//
// The document is written against OpenAPI 3.2 by default, which is the first version with a field
// for the HTTP QUERY method. A document targeting 3.1 is otherwise the same, and an endpoint served
// as QUERY is refused rather than written as something it is not.
package openapi

import (
	"cmp"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint/static_content"
	muxUtils "github.com/altshiftab/utils_go/pkg/http/mux/utils"
	openapiErrors "github.com/altshiftab/utils_go/pkg/http/openapi/errors"
	"github.com/altshiftab/utils_go/pkg/http/openapi/openapi_config"
	openapiTypes "github.com/altshiftab/utils_go/pkg/http/openapi/types"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
	jsonschemaTypes "github.com/altshiftab/utils_go/pkg/type_export/jsonschema/types"
	typeExportContext "github.com/altshiftab/utils_go/pkg/type_export/types/context"
)

// ComponentsRefPrefix is where a document keeps the schemas its operations refer to.
const ComponentsRefPrefix = "#/components/schemas/"

// reservedSchemaNames are the schemas the document writes itself. An endpoint type claiming one
// would be overwritten by it, and every reference to either would resolve to whichever won.
var reservedSchemaNames = map[string]struct{}{
	ProblemDetailSchemaName:               {},
	BodyValidationProblemDetailSchemaName: {},
}

// isDocumented says whether an endpoint belongs in a document offered to third parties.
func isDocumented(endpoint *endpointPkg.Endpoint) bool {
	if endpoint == nil || endpoint.Hint == nil {
		return false
	}

	if !endpoint.Hint.Documented {
		return false
	}

	return endpoint.StaticContent == nil
}

// documentedEndpoints is the endpoints a document is built from, in a settled order.
//
// The order matters beyond tidiness: the type context numbers colliding type names in the order it
// meets them, so a document built from the same endpoints in a different order would name the same
// type differently. Sorting here is what makes the output the same bytes every time.
func documentedEndpoints(endpoints []*endpointPkg.Endpoint) []*endpointPkg.Endpoint {
	var documented []*endpointPkg.Endpoint
	for _, endpoint := range endpoints {
		if isDocumented(endpoint) {
			documented = append(documented, endpoint)
		}
	}

	slices.SortFunc(documented, func(a *endpointPkg.Endpoint, b *endpointPkg.Endpoint) int {
		if pathComparison := cmp.Compare(a.Path, b.Path); pathComparison != 0 {
			return pathComparison
		}

		return cmp.Compare(a.Method, b.Method)
	})

	return documented
}

// setOperation puts the operation in the field its method is served under.
func setOperation(pathItem *openapiTypes.PathItem, method string, operation *openapiTypes.Operation, version string) error {
	switch strings.ToUpper(method) {
	case http.MethodGet:
		pathItem.Get = operation
	case http.MethodPut:
		pathItem.Put = operation
	case http.MethodPost:
		pathItem.Post = operation
	case http.MethodDelete:
		pathItem.Delete = operation
	case http.MethodOptions:
		pathItem.Options = operation
	case http.MethodHead:
		pathItem.Head = operation
	case http.MethodPatch:
		pathItem.Patch = operation
	case http.MethodTrace:
		pathItem.Trace = operation
	case MethodQuery:
		// 3.1 has no field for it, and no honest substitute: documented as a POST, a client would
		// be sent to a method the endpoint does not serve.
		if version != openapi_config.Version32 {
			return altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %s", openapiErrors.ErrQueryMethodUnsupported, version),
				version,
			)
		}
		pathItem.Query = operation
	default:
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %s", openapiErrors.ErrUnsupportedMethod, method),
			method,
		)
	}

	return nil
}

// addTypes registers the types the document's schemas are built from, in the order the operations
// are written, so that the names the context derives are the same on every run.
//
// Query types are left out: their parameters are described field by field, by a walk of their own,
// and a schema for the struct would be a second description of the same thing that nothing refers
// to.
func addTypes(context *jsonschemaTypes.Context, endpoints []*endpointPkg.Endpoint) error {
	for _, endpoint := range endpoints {
		hint := endpoint.Hint

		bodyType, _, err := inputTypes(endpoint)
		if err != nil {
			return fmt.Errorf("input types: %w", err)
		}

		var typeElements []any

		if bodyType != nil {
			typeElements = append(typeElements, bodyType)
		}

		if outputType := hint.OutputType; !isEmptyType(outputType) && !isByteSliceType(outputType) &&
			!isBinaryContentType(hint.OutputContentType) {
			typeElements = append(typeElements, outputType)
		}

		if len(typeElements) == 0 {
			continue
		}

		if err := context.Add(typeElements...); err != nil {
			return altshiftErrors.New(fmt.Errorf("type context add: %w", err), typeElements)
		}
	}

	return nil
}

// Generate writes the document describing the endpoints.
func Generate(endpoints []*endpointPkg.Endpoint, options ...openapi_config.Option) (*openapiTypes.Document, error) {
	config := openapi_config.New(options...)

	switch config.Version {
	case openapi_config.Version32, openapi_config.Version31:
	default:
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %s", openapiErrors.ErrUnsupportedVersion, config.Version),
			config.Version,
		)
	}

	info := config.Info
	if info == nil || info.Title == "" || info.Version == "" {
		return nil, altshiftErrors.NewWithTrace(openapiErrors.ErrMissingInfo)
	}

	documented := documentedEndpoints(endpoints)

	securitySchemeNames := make([]string, 0, len(config.SecuritySchemes))
	for name := range config.SecuritySchemes {
		securitySchemeNames = append(securitySchemeNames, name)
	}
	slices.Sort(securitySchemeNames)

	context := &jsonschemaTypes.Context{
		Context:   typeExportContext.New(),
		RefPrefix: ComponentsRefPrefix,
	}

	if err := addTypes(context, documented); err != nil {
		return nil, fmt.Errorf("add types: %w", err)
	}

	paths := make(map[string]*openapiTypes.PathItem, len(documented))
	operationIds := make(map[string]string, len(documented))

	for _, endpoint := range documented {
		operation, err := MakeOperation(endpoint, context, securitySchemeNames, ComponentsRefPrefix)
		if err != nil {
			return nil, altshiftErrors.New(
				fmt.Errorf("make operation (%s %s): %w", endpoint.Method, endpoint.Path, err),
				endpoint.Method,
				endpoint.Path,
			)
		}

		if existing, ok := operationIds[operation.OperationId]; ok {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf(
					"%w: %s, claimed by %s and %s %s",
					openapiErrors.ErrDuplicateOperationId,
					operation.OperationId,
					existing,
					endpoint.Method,
					endpoint.Path,
				),
				operation.OperationId,
			)
		}
		operationIds[operation.OperationId] = endpoint.Method + " " + endpoint.Path

		pathItem, ok := paths[endpoint.Path]
		if !ok {
			pathItem = &openapiTypes.PathItem{}
			paths[endpoint.Path] = pathItem
		}

		if err := setOperation(pathItem, endpoint.Method, operation, config.Version); err != nil {
			return nil, altshiftErrors.New(
				fmt.Errorf("set operation (%s %s): %w", endpoint.Method, endpoint.Path, err),
				endpoint.Method,
				endpoint.Path,
			)
		}
	}

	schemas, err := context.BuildSchemas()
	if err != nil {
		return nil, fmt.Errorf("build schemas: %w", err)
	}

	for name := range schemas {
		if _, ok := reservedSchemaNames[name]; ok {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %s", openapiErrors.ErrReservedSchemaName, name),
				name,
			)
		}
	}

	schemas[ProblemDetailSchemaName] = problemDetailSchema()
	if usesBodyValidationSchema(documented) {
		schemas[BodyValidationProblemDetailSchemaName] = bodyValidationProblemDetailSchema(ComponentsRefPrefix)
	}

	document := &openapiTypes.Document{
		Openapi:    config.Version,
		Info:       info,
		Servers:    config.Servers,
		Paths:      paths,
		Tags:       config.Tags,
		Components: &openapiTypes.Components{Schemas: schemas, SecuritySchemes: config.SecuritySchemes},
	}

	// What the document requires by default, which a public operation overrides with a requirement
	// of nothing.
	for _, name := range securitySchemeNames {
		document.Security = append(document.Security, map[string][]string{name: {}})
	}

	return document, nil
}

// Marshal writes the document as the bytes it is served and committed as.
//
// The same document marshals to the same bytes every time: schemas are maps, and json/v2 does not
// order a map's members unless it is told to. A document that reordered itself between runs would
// make a committed copy re-diff on every generation, and the diff is the point of committing it.
func Marshal(document *openapiTypes.Document) ([]byte, error) {
	if document == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("document"))
	}

	data, err := json.Marshal(document, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json marshal (document): %w", err), document)
	}

	return append(data, '\n'), nil
}

// NewEndpoint serves the document as static content, pre-compressed and tagged, the way any other
// body that does not change between requests is served.
//
// It is gated unless the configuration says otherwise. The document names every operation the
// service offers, what each takes and what each returns; handing that to an unauthenticated reader
// is a decision, and the default is not to make it silently.
func NewEndpoint(data []byte, options ...openapi_config.Option) (*endpointPkg.Endpoint, error) {
	if len(data) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("data"))
	}

	config := openapi_config.New(options...)

	path := config.Path
	if path == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("path"))
	}

	cacheControl := config.CacheControl
	if cacheControl == "" {
		if config.Public {
			cacheControl = "public, max-age=300"
		} else {
			cacheControl = "private, no-cache"
		}
	}

	etag := altshiftHttpUtils.MakeStrongEtag(data)
	lastModified := time.Now().UTC().Format(http.TimeFormat)

	staticContent := &static_content.StaticContent{
		StaticContentData: static_content.StaticContentData{
			Data:         data,
			Etag:         etag,
			LastModified: lastModified,
			Headers: muxUtils.MakeStaticContentHeaders(
				openapi_config.ContentType,
				cacheControl,
				etag,
				lastModified,
			),
		},
	}

	if err := endpointPkg.AddContentEncodingData(staticContent); err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("add content encoding data: %w", err), staticContent)
	}

	endpoint := &endpointPkg.Endpoint{
		Path:          path,
		Method:        http.MethodGet,
		StaticContent: staticContent,
	}
	endpoint.SetPublic(config.Public)

	return endpoint, nil
}
