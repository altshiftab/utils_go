package openapi

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_loader/body_setting"
	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	openapiTypes "github.com/altshiftab/utils_go/pkg/http/openapi/types"
)

// acceptsBody says whether the mux will read a body for the endpoint. A loader that forbids one is
// not the same as no loader, and both mean no body.
func acceptsBody(endpoint *endpointPkg.Endpoint) bool {
	bodyLoader := endpoint.BodyLoader

	return bodyLoader != nil && bodyLoader.Setting != body_setting.Forbidden
}

// derivedErrorResponses are the failures that follow from the endpoint itself, without the handler
// saying anything.
//
// They are the mux's own: what it answers before a handler is reached, or in place of one. An
// endpoint carrying a body over its limit is answered by the loader, a request without a session by
// the authenticator, a body that fails its schema by the parser. None of it is knowable from the
// handler, and all of it is knowable from the endpoint -- which is why it is derived rather than
// written out twenty times.
func derivedErrorResponses(endpoint *endpointPkg.Endpoint) map[int]string {
	responses := map[int]string{
		http.StatusBadRequest: "The request could not be read. The problem detail may carry an " +
			"\"errors\" array naming what failed.",
		http.StatusInternalServerError: "The request could not be answered.",
	}

	if !endpoint.Public {
		responses[http.StatusUnauthorized] = "No session, or a session that is no longer valid."
		responses[http.StatusForbidden] = "The session does not permit this operation."
	}

	if acceptsBody(endpoint) {
		bodyLoader := endpoint.BodyLoader

		if bodyLoader.MaxBytes > 0 {
			responses[http.StatusRequestEntityTooLarge] = fmt.Sprintf(
				"The body is larger than the limit of %d bytes.",
				bodyLoader.MaxBytes,
			)
		}

		if bodyLoader.ContentType != "" {
			responses[http.StatusUnsupportedMediaType] = fmt.Sprintf(
				"The body is not %s.",
				bodyLoader.ContentType,
			)
		}

		if hint := endpoint.Hint; hint != nil && hint.InputType != nil {
			responses[http.StatusUnprocessableEntity] = "The body does not satisfy the schema."
		}
	}

	if endpoint.RateLimitingConfiguration != nil {
		responses[http.StatusTooManyRequests] = "Too many requests."
	}

	return responses
}

// makeErrorResponses builds the failure responses of one operation: what the endpoint implies,
// overlaid with what its hint says.
//
// A hint's entry for a status that would have been derived replaces its description rather than
// adding a second one, so that an endpoint can say what it means by a status the mux also produces.
func makeErrorResponses(endpoint *endpointPkg.Endpoint, refPrefix string) map[string]*openapiTypes.Response {
	descriptions := derivedErrorResponses(endpoint)

	if hint := endpoint.Hint; hint != nil {
		for statusCode, description := range hint.ErrorResponses {
			descriptions[statusCode] = description
		}
	}

	responses := make(map[string]*openapiTypes.Response, len(descriptions))

	for statusCode, description := range descriptions {
		schemaName := ProblemDetailSchemaName
		if statusCode == http.StatusUnprocessableEntity {
			schemaName = BodyValidationProblemDetailSchemaName
		}

		responses[strconv.Itoa(statusCode)] = &openapiTypes.Response{
			Description: description,
			Content: map[string]*openapiTypes.MediaType{
				ProblemDetailContentType: {
					Schema: map[string]any{"$ref": refPrefix + schemaName},
				},
			},
		}
	}

	return responses
}

// usesBodyValidationSchema says whether any of the endpoints would refer to the validation problem
// detail, so that a document holds the schema only where something points at it.
func usesBodyValidationSchema(endpoints []*endpointPkg.Endpoint) bool {
	for _, endpoint := range endpoints {
		if endpoint == nil {
			continue
		}

		if _, ok := derivedErrorResponses(endpoint)[http.StatusUnprocessableEntity]; ok {
			return true
		}

		if hint := endpoint.Hint; hint != nil {
			if _, ok := hint.ErrorResponses[http.StatusUnprocessableEntity]; ok {
				return true
			}
		}
	}

	return false
}
