package openapi

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_loader"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_loader/body_setting"
	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	muxTypesRateLimiting "github.com/altshiftab/utils_go/pkg/http/mux/types/rate_limiting"
	openapiErrors "github.com/altshiftab/utils_go/pkg/http/openapi/errors"
	"github.com/altshiftab/utils_go/pkg/http/openapi/openapi_config"
	openapiTypes "github.com/altshiftab/utils_go/pkg/http/openapi/types"
)

const testContentTypeCose = "application/cose"

type testOrder struct {
	Id   string `json:"id"`
	Name string `json:"name,omitzero"`
}

type testOrdersQuery struct {
	Limit int `json:"limit,omitzero"`
}

type testCandidateDetails struct {
	Note string `json:"note"`
}

// sessionSchemeOption declares the one scheme the test documents authenticate with.
func sessionSchemeOption() openapi_config.Option {
	return openapi_config.WithSecurityScheme(
		"session",
		&openapiTypes.SecurityScheme{Type: "apiKey", In: "cookie", Name: "session"},
	)
}

func testOptions(options ...openapi_config.Option) []openapi_config.Option {
	return append(
		[]openapi_config.Option{
			openapi_config.WithInfo("Test API", "1.0.0"),
			sessionSchemeOption(),
		},
		options...,
	)
}

func jsonBodyLoader(maxBytes int64) *body_loader.Loader {
	return &body_loader.Loader{ContentType: contentTypeJson, MaxBytes: maxBytes}
}

// operationOf digs one operation out of a document, so that a case can assert about it without
// walking the path item's fields.
func operationOf(t *testing.T, document *openapiTypes.Document, path string, method string) *openapiTypes.Operation {
	t.Helper()

	pathItem, ok := document.Paths[path]
	if !ok {
		t.Fatalf("no path item for %s", path)
	}

	var operation *openapiTypes.Operation
	switch method {
	case http.MethodGet:
		operation = pathItem.Get
	case http.MethodPost:
		operation = pathItem.Post
	case http.MethodDelete:
		operation = pathItem.Delete
	case MethodQuery:
		operation = pathItem.Query
	default:
		t.Fatalf("the test helper does not reach %s", method)
	}

	if operation == nil {
		t.Fatalf("no %s operation at %s", method, path)
	}

	return operation
}

func TestGenerateOperationShapes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		endpoint *endpointPkg.Endpoint
		assert   func(t *testing.T, document *openapiTypes.Document)
	}{
		{
			name: "a body-less get carries its input as query parameters",
			endpoint: &endpointPkg.Endpoint{
				Path:   "/api/project",
				Method: http.MethodGet,
				Hint: &endpointPkg.Hint{
					InputType:         reflect.TypeFor[queryIdInput](),
					OutputType:        reflect.TypeFor[testOrder](),
					OutputContentType: contentTypeJson,
				},
			},
			assert: func(t *testing.T, document *openapiTypes.Document) {
				operation := operationOf(t, document, "/api/project", http.MethodGet)

				if operation.RequestBody != nil {
					t.Error("a get was given a request body")
				}
				if len(operation.Parameters) != 1 || operation.Parameters[0].Name != "id" {
					t.Errorf("expected one \"id\" parameter, got %v", operation.Parameters)
				}
				if operation.Parameters[0].Schema["format"] != "uuid" {
					t.Errorf("the parameter lost its format: %v", operation.Parameters[0].Schema)
				}
				if operation.OperationId != "getProject" {
					t.Errorf("unexpected operation id %q", operation.OperationId)
				}
			},
		},
		{
			name: "a post carries its input as a body",
			endpoint: &endpointPkg.Endpoint{
				Path:       "/api/order",
				Method:     http.MethodPost,
				BodyLoader: jsonBodyLoader(4096),
				Hint: &endpointPkg.Hint{
					InputType:         reflect.TypeFor[testOrder](),
					OutputType:        reflect.TypeFor[testOrder](),
					OutputContentType: contentTypeJson,
				},
			},
			assert: func(t *testing.T, document *openapiTypes.Document) {
				operation := operationOf(t, document, "/api/order", http.MethodPost)

				if operation.RequestBody == nil {
					t.Fatal("a post was given no request body")
				}
				if len(operation.Parameters) != 0 {
					t.Errorf("a post was given query parameters: %v", operation.Parameters)
				}

				mediaType, ok := operation.RequestBody.Content[contentTypeJson]
				if !ok {
					t.Fatalf("no %s body: %v", contentTypeJson, operation.RequestBody.Content)
				}
				if mediaType.Schema["$ref"] != ComponentsRefPrefix+"TestOrder" {
					t.Errorf("unexpected body schema %v", mediaType.Schema)
				}
			},
		},
		{
			name: "a query operation carries a body and goes in its own field",
			endpoint: &endpointPkg.Endpoint{
				Path:       "/api/orders",
				Method:     MethodQuery,
				BodyLoader: jsonBodyLoader(4096),
				Hint: &endpointPkg.Hint{
					InputType:         reflect.TypeFor[testOrdersQuery](),
					OutputType:        reflect.TypeFor[[]testOrder](),
					OutputContentType: contentTypeJson,
				},
			},
			assert: func(t *testing.T, document *openapiTypes.Document) {
				operation := operationOf(t, document, "/api/orders", MethodQuery)

				if operation.OperationId != "queryOrders" {
					t.Errorf("unexpected operation id %q", operation.OperationId)
				}
				if operation.RequestBody == nil {
					t.Fatal("a query operation was given no request body")
				}

				response, ok := operation.Responses["200"]
				if !ok {
					t.Fatalf("no 200 response: %v", operation.Responses)
				}
				schema := response.Content[contentTypeJson].Schema
				if schema["type"] != "array" {
					t.Errorf("a slice output is not an array: %v", schema)
				}
			},
		},
		{
			name: "a binary output is described by its content type alone",
			endpoint: &endpointPkg.Endpoint{
				Path:   "/api/order/bucket-file",
				Method: http.MethodGet,
				Hint: &endpointPkg.Hint{
					InputType:         reflect.TypeFor[queryIdInput](),
					OutputContentType: "application/pdf",
				},
			},
			assert: func(t *testing.T, document *openapiTypes.Document) {
				operation := operationOf(t, document, "/api/order/bucket-file", http.MethodGet)

				response, ok := operation.Responses["200"]
				if !ok {
					t.Fatalf("no 200 response: %v", operation.Responses)
				}

				mediaType, ok := response.Content["application/pdf"]
				if !ok {
					t.Fatalf("no application/pdf response: %v", response.Content)
				}
				if mediaType.Schema != nil {
					t.Errorf("a binary body was given a schema: %v", mediaType.Schema)
				}
			},
		},
		{
			name: "a byte slice output gets no schema",
			endpoint: &endpointPkg.Endpoint{
				Path:   "/.well-known/jwks.json",
				Method: http.MethodGet,
				Public: true,
				Hint: &endpointPkg.Hint{
					OutputType:        reflect.TypeFor[[]byte](),
					OutputContentType: "application/jwk-set+json",
				},
			},
			assert: func(t *testing.T, document *openapiTypes.Document) {
				operation := operationOf(t, document, "/.well-known/jwks.json", http.MethodGet)

				mediaType := operation.Responses["200"].Content["application/jwk-set+json"]
				if mediaType.Schema != nil {
					t.Errorf("a byte slice output was called a base64 string: %v", mediaType.Schema)
				}
			},
		},
		{
			name: "an encrypted body is named but not described",
			endpoint: &endpointPkg.Endpoint{
				Path:       "/api/order/candidate-details",
				Method:     http.MethodPost,
				BodyLoader: &body_loader.Loader{ContentType: testContentTypeCose, MaxBytes: 40_000_000},
				Hint: &endpointPkg.Hint{
					InputType: reflect.TypeFor[testCandidateDetails](),
				},
			},
			assert: func(t *testing.T, document *openapiTypes.Document) {
				operation := operationOf(t, document, "/api/order/candidate-details", http.MethodPost)

				mediaType, ok := operation.RequestBody.Content[testContentTypeCose]
				if !ok {
					t.Fatalf("no %s body: %v", testContentTypeCose, operation.RequestBody.Content)
				}
				if mediaType.Schema != nil {
					t.Errorf("ciphertext was given the plaintext's schema: %v", mediaType.Schema)
				}
				if operation.RequestBody.Description == "" {
					t.Error("an encrypted body says nothing about what it holds")
				}

				// The plaintext's shape is still in the document, so the name in the description
				// resolves to something.
				if _, ok := document.Components.Schemas["TestCandidateDetails"]; !ok {
					t.Error("the plaintext type is missing from the components")
				}
			},
		},
		{
			name: "an endpoint answering nothing answers 204",
			endpoint: &endpointPkg.Endpoint{
				Path:       "/api/order/complete",
				Method:     http.MethodPost,
				BodyLoader: jsonBodyLoader(1024),
				Hint: &endpointPkg.Hint{
					InputType: reflect.TypeFor[testOrder](),
				},
			},
			assert: func(t *testing.T, document *openapiTypes.Document) {
				operation := operationOf(t, document, "/api/order/complete", http.MethodPost)

				if _, ok := operation.Responses["204"]; !ok {
					t.Errorf("expected a 204, got %v", slices.Sorted(maps(operation.Responses)))
				}
				if _, ok := operation.Responses["200"]; ok {
					t.Error("an endpoint with no output was documented as answering 200")
				}
			},
		},
		{
			name: "a status the handler chooses is the one documented",
			endpoint: &endpointPkg.Endpoint{
				Path:       "/api/person-order",
				Method:     http.MethodPost,
				BodyLoader: jsonBodyLoader(1024),
				Hint: &endpointPkg.Hint{
					InputType:        reflect.TypeFor[testOrder](),
					OutputStatusCode: http.StatusCreated,
				},
			},
			assert: func(t *testing.T, document *openapiTypes.Document) {
				operation := operationOf(t, document, "/api/person-order", http.MethodPost)

				if _, ok := operation.Responses["201"]; !ok {
					t.Errorf("expected a 201, got %v", slices.Sorted(maps(operation.Responses)))
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			document, err := Generate([]*endpointPkg.Endpoint{testCase.endpoint}, testOptions()...)
			if err != nil {
				t.Fatalf("%s: %v", testCase.name, err)
			}

			testCase.assert(t, document)
		})
	}
}

// maps is a small iterator over a map's keys, for reporting what a document holds.
func maps[V any](m map[string]V) func(func(string) bool) {
	return func(yield func(string) bool) {
		for key := range m {
			if !yield(key) {
				return
			}
		}
	}
}

func TestGenerateDerivedErrorResponses(t *testing.T) {
	t.Parallel()

	endpoint := &endpointPkg.Endpoint{
		Path:                      "/api/order",
		Method:                    http.MethodPost,
		BodyLoader:                jsonBodyLoader(4096),
		RateLimitingConfiguration: &muxTypesRateLimiting.RateLimitingConfiguration{},
		Hint: &endpointPkg.Hint{
			InputType:         reflect.TypeFor[testOrder](),
			OutputType:        reflect.TypeFor[testOrder](),
			OutputContentType: contentTypeJson,
			ErrorResponses: map[int]string{
				http.StatusNotFound:  "No order was found with the provided id.",
				http.StatusForbidden: "Not permitted to create an order for this project.",
			},
		},
	}

	document, err := Generate([]*endpointPkg.Endpoint{endpoint}, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}

	operation := operationOf(t, document, "/api/order", http.MethodPost)

	testCases := []struct {
		name                string
		statusCode          string
		expectedDescription string
	}{
		{name: "bad request is always documented", statusCode: "400"},
		{name: "a gated endpoint can answer 401", statusCode: "401"},
		{name: "a body limit means 413", statusCode: "413"},
		{name: "a content type means 415", statusCode: "415"},
		{name: "a described body means 422", statusCode: "422"},
		{name: "a rate limit means 429", statusCode: "429"},
		{name: "a server error is always documented", statusCode: "500"},
		{
			name:                "what only the handler knows comes from the hint",
			statusCode:          "404",
			expectedDescription: "No order was found with the provided id.",
		},
		{
			name:                "the hint replaces a derived description",
			statusCode:          "403",
			expectedDescription: "Not permitted to create an order for this project.",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			response, ok := operation.Responses[testCase.statusCode]
			if !ok {
				t.Fatalf("%s: no %s response", testCase.name, testCase.statusCode)
			}

			if response.Description == "" {
				t.Errorf("%s: the %s response says nothing", testCase.name, testCase.statusCode)
			}

			if testCase.expectedDescription != "" && response.Description != testCase.expectedDescription {
				t.Errorf(
					"%s: the %s response says %q, expected %q",
					testCase.name,
					testCase.statusCode,
					response.Description,
					testCase.expectedDescription,
				)
			}

			mediaType, ok := response.Content[ProblemDetailContentType]
			if !ok {
				t.Fatalf("%s: the %s response is not a problem detail", testCase.name, testCase.statusCode)
			}

			expectedSchemaName := ProblemDetailSchemaName
			if testCase.statusCode == "422" {
				expectedSchemaName = BodyValidationProblemDetailSchemaName
			}
			if mediaType.Schema["$ref"] != ComponentsRefPrefix+expectedSchemaName {
				t.Errorf(
					"%s: the %s response refers to %v",
					testCase.name,
					testCase.statusCode,
					mediaType.Schema["$ref"],
				)
			}
		})
	}
}

// TestGeneratePublicEndpointRequiresNothing checks both halves of the security arrangement: the
// document requires a session by default, and a public operation overrides that with a requirement
// of nothing.
func TestGeneratePublicEndpointRequiresNothing(t *testing.T) {
	t.Parallel()

	endpoints := []*endpointPkg.Endpoint{
		{
			Path:   "/api/settings",
			Method: http.MethodGet,
			Hint:   &endpointPkg.Hint{OutputType: reflect.TypeFor[testOrder](), OutputContentType: contentTypeJson},
		},
		{
			Path:   "/verification",
			Method: http.MethodGet,
			Public: true,
			Hint:   &endpointPkg.Hint{OutputContentType: "text/html"},
		},
	}

	document, err := Generate(endpoints, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}

	if len(document.Security) != 1 {
		t.Errorf("the document requires %v", document.Security)
	}

	gated := operationOf(t, document, "/api/settings", http.MethodGet)
	if len(gated.Security) != 1 {
		t.Errorf("a gated operation requires %v", gated.Security)
	}

	public := operationOf(t, document, "/verification", http.MethodGet)
	if public.Security == nil || len(public.Security) != 0 {
		t.Errorf("a public operation requires %v, expected an empty requirement", public.Security)
	}

	// An empty requirement has to survive marshalling as [], which is what overrides the document's
	// own. Omitted, the operation would inherit the session requirement it does not have.
	data, err := Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	var roundTripped map[string]any
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatal(err)
	}

	paths, _ := roundTripped["paths"].(map[string]any)
	verification, _ := paths["/verification"].(map[string]any)
	get, _ := verification["get"].(map[string]any)
	security, ok := get["security"].([]any)
	if !ok {
		t.Fatalf("the public operation lost its security override: %v", get)
	}
	if len(security) != 0 {
		t.Errorf("the public operation requires %v", security)
	}
}

func TestGenerateExclusions(t *testing.T) {
	t.Parallel()

	endpoints := []*endpointPkg.Endpoint{
		{
			Path:   "/api/settings",
			Method: http.MethodGet,
			Hint:   &endpointPkg.Hint{OutputType: reflect.TypeFor[testOrder](), OutputContentType: contentTypeJson},
		},
		{
			Path:       "/api/order/candidate-details",
			Method:     http.MethodPost,
			BodyLoader: jsonBodyLoader(1024),
			Hint: &endpointPkg.Hint{
				InputType: reflect.TypeFor[testCandidateDetails](),
				Internal:  true,
			},
		},
		{
			// No hint: nothing is known about what it takes or returns.
			Path:   "/api/unhinted",
			Method: http.MethodGet,
		},
	}

	document, err := Generate(endpoints, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := document.Paths["/api/order/candidate-details"]; ok {
		t.Error("an internal endpoint was documented")
	}
	if _, ok := document.Paths["/api/unhinted"]; ok {
		t.Error("an endpoint with no hint was documented")
	}
	if _, ok := document.Paths["/api/settings"]; !ok {
		t.Error("a documented endpoint is missing")
	}

	// An internal endpoint's types must not reach the components either: a reader would learn the
	// shape of what the endpoint takes without the endpoint appearing at all.
	if _, ok := document.Components.Schemas["TestCandidateDetails"]; ok {
		t.Error("an internal endpoint's type reached the components")
	}
}

func TestGenerateRefusals(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		endpoint      *endpointPkg.Endpoint
		options       []openapi_config.Option
		expectedError error
	}{
		{
			name: "the query method needs 3.2",
			endpoint: &endpointPkg.Endpoint{
				Path:       "/api/orders",
				Method:     MethodQuery,
				BodyLoader: jsonBodyLoader(4096),
				Hint:       &endpointPkg.Hint{InputType: reflect.TypeFor[testOrdersQuery]()},
			},
			options:       []openapi_config.Option{openapi_config.WithVersion(openapi_config.Version31)},
			expectedError: openapiErrors.ErrQueryMethodUnsupported,
		},
		{
			name: "a hint naming a body the endpoint will not read",
			endpoint: &endpointPkg.Endpoint{
				Path:   "/api/order",
				Method: http.MethodPost,
				Hint:   &endpointPkg.Hint{InputType: reflect.TypeFor[testOrder]()},
			},
			expectedError: openapiErrors.ErrBodyNotAccepted,
		},
		{
			name: "a body-less method whose loader accepts a body",
			endpoint: &endpointPkg.Endpoint{
				Path:       "/api/project",
				Method:     http.MethodDelete,
				BodyLoader: jsonBodyLoader(1024),
				Hint:       &endpointPkg.Hint{InputType: reflect.TypeFor[queryIdInput]()},
			},
			expectedError: openapiErrors.ErrUndescribedBody,
		},
		{
			name: "two query types on one body-less method",
			endpoint: &endpointPkg.Endpoint{
				Path:   "/api/project",
				Method: http.MethodGet,
				Hint: &endpointPkg.Hint{
					InputType:    reflect.TypeFor[queryIdInput](),
					UrlInputType: reflect.TypeFor[queryJsonFallbackInput](),
				},
			},
			expectedError: openapiErrors.ErrAmbiguousInput,
		},
		{
			name: "a binary output alongside an output type",
			endpoint: &endpointPkg.Endpoint{
				Path:   "/api/order/bucket-file",
				Method: http.MethodGet,
				Hint: &endpointPkg.Hint{
					OutputType:        reflect.TypeFor[testOrder](),
					OutputContentType: "application/pdf",
				},
			},
			expectedError: openapiErrors.ErrBinaryOutputWithOutputType,
		},
		{
			name: "an optional binary output",
			endpoint: &endpointPkg.Endpoint{
				Path:   "/api/order/bucket-file",
				Method: http.MethodGet,
				Hint: &endpointPkg.Hint{
					OutputContentType: "application/pdf",
					OutputOptional:    true,
				},
			},
			expectedError: openapiErrors.ErrOptionalBinaryOutput,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := Generate([]*endpointPkg.Endpoint{testCase.endpoint}, testOptions(testCase.options...)...)
			if !errors.Is(err, testCase.expectedError) {
				t.Errorf("%s: expected %v, got %v", testCase.name, testCase.expectedError, err)
			}
		})
	}
}

func TestGenerateDocumentRefusals(t *testing.T) {
	t.Parallel()

	settingsEndpoint := func() *endpointPkg.Endpoint {
		return &endpointPkg.Endpoint{
			Path:   "/api/settings",
			Method: http.MethodGet,
			Hint:   &endpointPkg.Hint{OutputType: reflect.TypeFor[testOrder](), OutputContentType: contentTypeJson},
		}
	}

	testCases := []struct {
		name          string
		endpoints     []*endpointPkg.Endpoint
		options       []openapi_config.Option
		expectedError error
	}{
		{
			name:          "a document with no title",
			endpoints:     []*endpointPkg.Endpoint{settingsEndpoint()},
			options:       []openapi_config.Option{sessionSchemeOption()},
			expectedError: openapiErrors.ErrMissingInfo,
		},
		{
			name:      "a version the generator does not write",
			endpoints: []*endpointPkg.Endpoint{settingsEndpoint()},
			options: []openapi_config.Option{
				openapi_config.WithInfo("Test API", "1.0.0"),
				sessionSchemeOption(),
				openapi_config.WithVersion("3.0.3"),
			},
			expectedError: openapiErrors.ErrUnsupportedVersion,
		},
		{
			name:      "a gated endpoint with no way to authenticate",
			endpoints: []*endpointPkg.Endpoint{settingsEndpoint()},
			options: []openapi_config.Option{
				openapi_config.WithInfo("Test API", "1.0.0"),
			},
			expectedError: openapiErrors.ErrMissingSecurityScheme,
		},
		{
			name: "two operations claiming one identifier",
			endpoints: []*endpointPkg.Endpoint{
				{
					Path:   "/api/order-file",
					Method: http.MethodGet,
					Hint:   &endpointPkg.Hint{OutputContentType: "text/plain", OutputType: reflect.TypeFor[string]()},
				},
				{
					Path:   "/api/order/file",
					Method: http.MethodGet,
					Hint:   &endpointPkg.Hint{OutputContentType: "text/plain", OutputType: reflect.TypeFor[string]()},
				},
			},
			options: []openapi_config.Option{
				openapi_config.WithInfo("Test API", "1.0.0"),
				sessionSchemeOption(),
			},
			expectedError: openapiErrors.ErrDuplicateOperationId,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := Generate(testCase.endpoints, testCase.options...)
			if !errors.Is(err, testCase.expectedError) {
				t.Errorf("%s: expected %v, got %v", testCase.name, testCase.expectedError, err)
			}
		})
	}
}

// TestMarshalIsDeterministic is what makes a committed document worth committing: the same
// endpoints must marshal to the same bytes, whatever order they arrive in, or the file re-diffs on
// every generation and the diff stops meaning anything.
func TestMarshalIsDeterministic(t *testing.T) {
	t.Parallel()

	endpoints := []*endpointPkg.Endpoint{
		{
			Path:       "/api/order",
			Method:     http.MethodPost,
			BodyLoader: jsonBodyLoader(4096),
			Hint: &endpointPkg.Hint{
				InputType:         reflect.TypeFor[testOrder](),
				OutputType:        reflect.TypeFor[testOrder](),
				OutputContentType: contentTypeJson,
			},
		},
		{
			Path:       "/api/orders",
			Method:     MethodQuery,
			BodyLoader: jsonBodyLoader(4096),
			Hint: &endpointPkg.Hint{
				InputType:         reflect.TypeFor[testOrdersQuery](),
				OutputType:        reflect.TypeFor[[]testOrder](),
				OutputContentType: contentTypeJson,
			},
		},
		{
			Path:   "/api/project",
			Method: http.MethodGet,
			Hint: &endpointPkg.Hint{
				InputType:         reflect.TypeFor[queryIdInput](),
				OutputType:        reflect.TypeFor[testCandidateDetails](),
				OutputContentType: contentTypeJson,
			},
		},
		{
			Path:   "/api/project",
			Method: http.MethodDelete,
			Hint: &endpointPkg.Hint{
				InputType:         reflect.TypeFor[queryIdInput](),
				OutputContentType: "text/plain",
				OutputType:        reflect.TypeFor[string](),
			},
		},
	}

	document, err := Generate(endpoints, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}

	expected, err := Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	// Every rotation of the reversed order, so that no endpoint keeps the position it was added in.
	for offset := range endpoints {
		shuffled := make([]*endpointPkg.Endpoint, 0, len(endpoints))
		for i := range endpoints {
			shuffled = append(shuffled, endpoints[(len(endpoints)-1-i+offset)%len(endpoints)])
		}

		shuffledDocument, err := Generate(shuffled, testOptions()...)
		if err != nil {
			t.Fatal(err)
		}

		actual, err := Marshal(shuffledDocument)
		if err != nil {
			t.Fatal(err)
		}

		if string(actual) != string(expected) {
			t.Fatalf("offset %d produced different bytes from the same endpoints", offset)
		}
	}
}

// TestMarshalRoundTrips checks that what is written is a document a reader can parse, and that the
// version it declares is the one it was written against.
func TestMarshalRoundTrips(t *testing.T) {
	t.Parallel()

	endpoint := &endpointPkg.Endpoint{
		Path:       "/api/order",
		Method:     http.MethodPost,
		BodyLoader: &body_loader.Loader{ContentType: contentTypeJson, MaxBytes: 4096, Setting: body_setting.Optional},
		Hint: &endpointPkg.Hint{
			InputType:         reflect.TypeFor[testOrder](),
			OutputType:        reflect.TypeFor[testOrder](),
			OutputContentType: contentTypeJson,
			Summary:           "Create an order.",
			Tags:              []string{"orders"},
		},
	}

	document, err := Generate([]*endpointPkg.Endpoint{endpoint}, testOptions()...)
	if err != nil {
		t.Fatal(err)
	}

	data, err := Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	var roundTripped map[string]any
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("the document does not parse: %v", err)
	}

	if roundTripped["openapi"] != openapi_config.Version32 {
		t.Errorf("the document declares %v", roundTripped["openapi"])
	}

	info, ok := roundTripped["info"].(map[string]any)
	if !ok || info["title"] != "Test API" {
		t.Errorf("unexpected info: %v", roundTripped["info"])
	}

	// An optional body is not required of the caller.
	paths, _ := roundTripped["paths"].(map[string]any)
	order, _ := paths["/api/order"].(map[string]any)
	post, _ := order["post"].(map[string]any)
	requestBody, _ := post["requestBody"].(map[string]any)
	if _, ok := requestBody["required"]; ok {
		t.Errorf("an optional body was documented as required: %v", requestBody)
	}

	if data[len(data)-1] != '\n' {
		t.Error("the document does not end with a newline")
	}
}
