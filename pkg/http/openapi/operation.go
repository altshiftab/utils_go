package openapi

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_loader/body_setting"
	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	openapiErrors "github.com/altshiftab/utils_go/pkg/http/openapi/errors"
	openapiTypes "github.com/altshiftab/utils_go/pkg/http/openapi/types"
	jsonschemaTypes "github.com/altshiftab/utils_go/pkg/type_export/jsonschema/types"
)

// MethodQuery is the HTTP QUERY method: a safe, idempotent read whose parameters travel in the
// request body. net/http defines no constant for it.
const MethodQuery = "QUERY"

const (
	contentTypeJson = "application/json"
	contentTypeJose = "application/jose"
)

var emptyInterfaceType = reflect.TypeFor[any]()

// isEmptyType says whether a hint's type says nothing -- unset, or the empty interface it is set to
// where an endpoint has no type to name.
func isEmptyType(reflectType reflect.Type) bool {
	return reflectType == nil || reflectType == emptyInterfaceType
}

// carriesBodyMethod says whether the method carries a request body, and so how the hint's InputType
// is to be read. See the Hint documentation: the rule is the method's, and is the one the generated
// clients follow.
func carriesBodyMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodDelete:
		return false
	default:
		return true
	}
}

// isBinaryContentType says whether a body of this content type is bytes rather than a structure a
// schema could describe. The predicate is the typed client generator's, so that the two agree on
// what a response carries.
func isBinaryContentType(contentType string) bool {
	if contentType == "" {
		return false
	}

	return contentType != contentTypeJson &&
		!strings.HasSuffix(contentType, "+json") &&
		!strings.HasPrefix(contentType, "text/") &&
		contentType != contentTypeJose
}

// isByteSliceType says whether the type is a []byte, which is served as the bytes it holds rather
// than as the base64 string a JSON schema would call it.
func isByteSliceType(reflectType reflect.Type) bool {
	if reflectType == nil {
		return false
	}

	for reflectType.Kind() == reflect.Pointer {
		reflectType = reflectType.Elem()
	}

	return reflectType.Kind() == reflect.Slice && reflectType.Elem().Kind() == reflect.Uint8
}

// inputTypes says which of a hint's types are the request body and which are query parameters.
//
// It also refuses the three ways a hint and an endpoint can contradict each other, rather than
// document one of them and let the other be found by whoever tries the request.
func inputTypes(endpoint *endpointPkg.Endpoint) (reflect.Type, reflect.Type, error) {
	hint := endpoint.Hint

	var bodyType reflect.Type
	var queryType reflect.Type

	if carriesBodyMethod(endpoint.Method) {
		if !isEmptyType(hint.InputType) {
			bodyType = hint.InputType
		}
		if !isEmptyType(hint.UrlInputType) {
			queryType = hint.UrlInputType
		}

		if bodyType != nil && !acceptsBody(endpoint) {
			return nil, nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %s %s", openapiErrors.ErrBodyNotAccepted, endpoint.Method, endpoint.Path),
				endpoint.Method,
				endpoint.Path,
			)
		}

		return bodyType, queryType, nil
	}

	// A method carrying no body reads InputType as query parameters, which is what UrlInputType
	// would also mean. Both at once says two things about one query and settles neither.
	if !isEmptyType(hint.InputType) && !isEmptyType(hint.UrlInputType) {
		return nil, nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %s %s", openapiErrors.ErrAmbiguousInput, endpoint.Method, endpoint.Path),
			endpoint.Method,
			endpoint.Path,
		)
	}

	if acceptsBody(endpoint) {
		return nil, nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %s %s", openapiErrors.ErrUndescribedBody, endpoint.Method, endpoint.Path),
			endpoint.Method,
			endpoint.Path,
		)
	}

	if !isEmptyType(hint.InputType) {
		queryType = hint.InputType
	} else if !isEmptyType(hint.UrlInputType) {
		queryType = hint.UrlInputType
	}

	return nil, queryType, nil
}

// successStatusCode is the status the operation answers with when it succeeds.
//
// Zero in the hint means what the response writer does when a handler names no status: 200 where
// there is a body, 204 where there is none. The rule is the writer's rather than a flat 200, so
// that an endpoint answering nothing is not documented as answering something.
func successStatusCode(hint *endpointPkg.Hint) int {
	if hint.OutputStatusCode != 0 {
		return hint.OutputStatusCode
	}

	if hint.OutputContentType == "" && isEmptyType(hint.OutputType) {
		return http.StatusNoContent
	}

	return http.StatusOK
}

// makeSuccessResponse describes what the operation answers with when it succeeds.
func makeSuccessResponse(
	endpoint *endpointPkg.Endpoint,
	context *jsonschemaTypes.Context,
) (*openapiTypes.Response, error) {
	hint := endpoint.Hint

	contentType := hint.OutputContentType
	binary := isBinaryContentType(contentType)

	if binary {
		// The client generator refuses these two, so a hint carrying them would produce a document
		// and no client. Refusing them here keeps the pair honest.
		if !isEmptyType(hint.OutputType) {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %s", openapiErrors.ErrBinaryOutputWithOutputType, contentType),
				contentType,
			)
		}
		if hint.OutputOptional {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %s", openapiErrors.ErrOptionalBinaryOutput, contentType),
				contentType,
			)
		}
	}

	response := &openapiTypes.Response{Description: "Success."}

	if contentType == "" {
		return response, nil
	}

	mediaType := &openapiTypes.MediaType{}

	// Bytes are described by their content type alone. A schema would have to call them a base64
	// string, which is not what the body holds.
	if !binary && !isEmptyType(hint.OutputType) && !isByteSliceType(hint.OutputType) {
		schema, err := context.GetJSONSchemaType(hint.OutputType)
		if err != nil {
			return nil, altshiftErrors.New(
				fmt.Errorf("get json schema type (output): %w", err),
				hint.OutputType,
			)
		}
		mediaType.Schema = schema
	}

	response.Content = map[string]*openapiTypes.MediaType{contentType: mediaType}

	return response, nil
}

// makeRequestBody describes what the operation takes as a body.
//
// A body whose content type is not a structure gets no schema: a COSE or JOSE envelope carries
// ciphertext, and saying it is the plaintext's shape would be a claim about bytes that are not
// there. The type is still named in the description, so a reader knows what to encrypt.
func makeRequestBody(
	endpoint *endpointPkg.Endpoint,
	bodyType reflect.Type,
	context *jsonschemaTypes.Context,
) (*openapiTypes.RequestBody, error) {
	if !acceptsBody(endpoint) {
		return nil, nil
	}

	bodyLoader := endpoint.BodyLoader

	contentType := bodyLoader.ContentType
	if contentType == "" {
		contentType = contentTypeJson
	}

	mediaType := &openapiTypes.MediaType{}
	var description string

	switch {
	case bodyType == nil:
	case isBinaryContentType(contentType):
		typeName := bodyType.Name()
		if typeName != "" {
			description = fmt.Sprintf("The encoded form of %s.", typeName)
		}

		// The plaintext's shape is still worth having in the document, so that the name in the
		// description resolves to something.
		if _, err := context.GetJSONSchemaType(bodyType); err != nil {
			return nil, altshiftErrors.New(
				fmt.Errorf("get json schema type (body): %w", err),
				bodyType,
			)
		}
	default:
		schema, err := context.GetJSONSchemaType(bodyType)
		if err != nil {
			return nil, altshiftErrors.New(
				fmt.Errorf("get json schema type (body): %w", err),
				bodyType,
			)
		}
		mediaType.Schema = schema
	}

	return &openapiTypes.RequestBody{
		Description: description,
		Required:    bodyLoader.Setting != body_setting.Optional,
		Content:     map[string]*openapiTypes.MediaType{contentType: mediaType},
	}, nil
}

// MakeOperation describes one endpoint as an operation.
func MakeOperation(
	endpoint *endpointPkg.Endpoint,
	context *jsonschemaTypes.Context,
	securitySchemeNames []string,
	refPrefix string,
) (*openapiTypes.Operation, error) {
	hint := endpoint.Hint

	bodyType, queryType, err := inputTypes(endpoint)
	if err != nil {
		return nil, fmt.Errorf("input types: %w", err)
	}

	parameters, err := MakeParameters(queryType)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("make parameters: %w", err), queryType)
	}

	requestBody, err := makeRequestBody(endpoint, bodyType, context)
	if err != nil {
		return nil, fmt.Errorf("make request body: %w", err)
	}

	successResponse, err := makeSuccessResponse(endpoint, context)
	if err != nil {
		return nil, fmt.Errorf("make success response: %w", err)
	}

	responses := makeErrorResponses(endpoint, refPrefix)
	responses[strconv.Itoa(successStatusCode(hint))] = successResponse

	// An optional body is a second thing the operation may answer with, and the reader has to know
	// it may arrive empty.
	if hint.OutputOptional {
		noContent := strconv.Itoa(http.StatusNoContent)
		if _, ok := responses[noContent]; !ok {
			responses[noContent] = &openapiTypes.Response{Description: "Success, with no content."}
		}
	}

	operationId := hint.OperationId
	if operationId == "" {
		operationId = endpointPkg.OperationName(endpoint.Method, endpoint.Path)
	}

	operation := &openapiTypes.Operation{
		OperationId: operationId,
		Summary:     hint.Summary,
		Description: hint.Description,
		Tags:        hint.Tags,
		Deprecated:  hint.Deprecated,
		Parameters:  parameters,
		RequestBody: requestBody,
		Responses:   responses,
	}

	if endpoint.Public {
		// An empty requirement is how an operation says it needs nothing, overriding whatever the
		// document requires of the rest.
		operation.Security = []map[string][]string{}
	} else {
		if len(securitySchemeNames) == 0 {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %s %s", openapiErrors.ErrMissingSecurityScheme, endpoint.Method, endpoint.Path),
				endpoint.Method,
				endpoint.Path,
			)
		}

		for _, name := range securitySchemeNames {
			operation.Security = append(operation.Security, map[string][]string{name: {}})
		}
	}

	return operation, nil
}
