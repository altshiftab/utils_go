package mux

import (
	"context"
	"encoding/json/jsontext"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"

	altshiftContext "github.com/altshiftab/utils_go/pkg/context"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	muxContext "github.com/altshiftab/utils_go/pkg/http/mux/context"
	muxErrors "github.com/altshiftab/utils_go/pkg/http/mux/errors"
	muxInternal "github.com/altshiftab/utils_go/pkg/http/mux/internal"
	muxInternalMux "github.com/altshiftab/utils_go/pkg/http/mux/internal/mux"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_loader/body_setting"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_parser"
	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	muxTypesFirewall "github.com/altshiftab/utils_go/pkg/http/mux/types/firewall_verdict"
	muxTypesMiddleware "github.com/altshiftab/utils_go/pkg/http/mux/types/middleware"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	muxTypesResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	muxTypesResponseWriter "github.com/altshiftab/utils_go/pkg/http/mux/types/response_writer"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/userer"
	utils2 "github.com/altshiftab/utils_go/pkg/http/mux/utils"
	muxUtilsContentNegotiation "github.com/altshiftab/utils_go/pkg/http/mux/utils/content_negotiation"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/cache_control"
	"github.com/altshiftab/utils_go/pkg/http/types/content_security_policy"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
	altshiftIter "github.com/altshiftab/utils_go/pkg/iter"
	"github.com/altshiftab/utils_go/pkg/utils"
	"github.com/altshiftab/utils_go/pkg/uuid"
)

const (
	contentSecurityPolicyHeaderName = "Content-Security-Policy"
)

// The messages the mux logs an error response under, for a log handler that reads the HTTP context
// to match against rather than write out a second time; see the internal package for what they are
// for.
const (
	ClientErrorMessage    = muxInternal.ClientErrorMessage
	ServerErrorMessage    = muxInternal.ServerErrorMessage
	ResponseServedMessage = muxInternal.ResponseServedMessage
)

type muxHttpContextContextType struct{}

var MuxHttpContextContextKey muxHttpContextContextType

// TODO: Do all of these need to be here, or can they be moved to the `Mux` struct?
type baseMux struct {
	SetContextKeyValuePairs [][2]any
	ResponseErrorHandler    func(context.Context, *muxTypesResponseError.ResponseError, *muxTypesResponseWriter.ResponseWriter)
	DoneCallback            func(context.Context)
	FirewallParser          request_parser.RequestParser[muxTypesFirewall.Verdict]
	DefaultHeaders          map[string]string
	DefaultDocumentHeaders  map[string]string
	Middleware              []muxTypesMiddleware.Middleware
	ProblemDetailConverter  muxTypesResponseError.ProblemDetailConverter
}

//nolint:contextcheck,fatcontext // The request context is deliberately extended and reassigned via request.WithContext; contextcheck cannot track the chain, and the loop builds one derived context from the configured pairs.
func (bm *baseMux) ServeHttpWithCallback(
	originalResponseWriter http.ResponseWriter,
	request *http.Request,
	callback func(*http.Request, *muxTypesResponseWriter.ResponseWriter) (*muxTypesResponse.Response, *muxTypesResponseError.ResponseError),
) {
	if originalResponseWriter == nil {
		return
	}

	if request == nil {
		return
	}

	if callback == nil {
		return
	}

	responseErrorHandler := bm.ResponseErrorHandler
	if responseErrorHandler == nil {
		responseErrorHandler = muxInternal.DefaultResponseErrorHandler
	}

	// Create an HTTP context and populate it with the request and put it in the request context.

	httpContext := &altshiftHttpTypes.HttpContext{Request: request}
	request = request.WithContext(
		context.WithValue(request.Context(), MuxHttpContextContextKey, httpContext),
	)

	requestId, err := uuid.NewV7()
	if err != nil {
		slog.WarnContext(
			altshiftContext.WithError(
				request.Context(),
				altshiftErrors.NewWithTrace(fmt.Errorf("uuid new v7: %w", err)),
			),
			"An error occurred when generating a request id.",
		)
	} else {
		contextRequest := request.WithContext(
			context.WithValue(request.Context(), altshiftHttpContext.RequestIdContextKey, requestId.String()),
		)
		if contextRequest != nil {
			request = contextRequest
		}
	}

	if len(bm.SetContextKeyValuePairs) != 0 {
		ctx := request.Context()
		for _, pair := range bm.SetContextKeyValuePairs {
			ctx = context.WithValue(ctx, pair[0], pair[1])
		}
		if contextRequest := request.WithContext(ctx); contextRequest != nil {
			request = contextRequest
		}
	}

	// Use a custom response writer.

	var responseWriter *muxTypesResponseWriter.ResponseWriter

	if convertedResponseWriter, ok := originalResponseWriter.(*muxTypesResponseWriter.ResponseWriter); ok {
		responseWriter = convertedResponseWriter
		originalResponseWriter = convertedResponseWriter.ResponseWriter

		responseWriter.DefaultHeaders = bm.DefaultHeaders
		responseWriter.DefaultDocumentHeaders = bm.DefaultDocumentHeaders
	} else {
		responseWriter = &muxTypesResponseWriter.ResponseWriter{
			ResponseWriter:         originalResponseWriter,
			DefaultHeaders:         bm.DefaultHeaders,
			DefaultDocumentHeaders: bm.DefaultDocumentHeaders,
		}
	}

	responseWriter.IsHeadRequest = strings.ToUpper(request.Method) == http.MethodHead

	// Perform firewall check.

	verdict := muxTypesFirewall.Accept
	var firewallResponseError *muxTypesResponseError.ResponseError
	if firewallParser := bm.FirewallParser; !utils.IsNil(firewallParser) {
		verdict, firewallResponseError = firewallParser.Parse(request)
	}

	switch verdict {
	case muxTypesFirewall.Drop:
		hijacker, ok := originalResponseWriter.(http.Hijacker)
		if ok {
			connection, _, err := hijacker.Hijack()
			if err != nil {
				responseErrorHandler(
					altshiftHttpContext.WithHttpContextValue(request.Context(), httpContext),
					&muxTypesResponseError.ResponseError{
						ServerError: altshiftErrors.NewWithTrace(
							fmt.Errorf("response writer hijacker hijack: %w", err),
						),
					},
					responseWriter,
				)
			}
			if connection != nil {
				if err := connection.Close(); err != nil {
					slog.ErrorContext(
						altshiftContext.WithError(
							request.Context(),
							altshiftErrors.NewWithTrace(
								fmt.Errorf("connection close: %w", err),
							),
						),
						"An error occurred when closing a connection.",
					)
				}
			}
			return
		}

		// Trigger a termination of the connection.
		panic(http.ErrAbortHandler)
	case muxTypesFirewall.Reject:
		if firewallResponseError == nil {
			firewallResponseError = &muxTypesResponseError.ResponseError{
				ProblemDetail: problem_detail.New(http.StatusForbidden),
			}
		}
		responseErrorHandler(altshiftHttpContext.WithHttpContextValue(request.Context(), httpContext), firewallResponseError, responseWriter)
	default:
		for _, middleware := range bm.Middleware {
			if middleware != nil {
				if middlewareRequest := middleware(request); middlewareRequest != nil {
					request = middlewareRequest
				}
			}
		}

		var acceptEncoding *altshiftHttpTypes.AcceptEncoding

		if contentNegotiation, _ := muxUtilsContentNegotiation.GetContentNegotiation(request.Header, false); contentNegotiation != nil {
			request = request.WithContext(
				context.WithValue(request.Context(), muxContext.ContentNegotiationContextKey, contentNegotiation),
			)
			acceptEncoding = contentNegotiation.AcceptEncoding
		}

		// Respond to the request.

		response, responseError := callback(request, responseWriter)

		if !responseWriter.WriteHeaderCalled {
			if responseError != nil {
				responseErrorHandler(altshiftHttpContext.WithHttpContextValue(request.Context(), httpContext), responseError, responseWriter)
			} else {
				if response == nil {
					response = &muxTypesResponse.Response{}
				}

				if err := responseWriter.WriteResponse(request.Context(), response, acceptEncoding); err != nil {
					responseErrorHandler(
						altshiftHttpContext.WithHttpContextValue(request.Context(), httpContext),
						&muxTypesResponseError.ResponseError{
							ServerError: altshiftErrors.New(
								fmt.Errorf("write response: %w", err),
								response,
							),
						},
						responseWriter,
					)
				}
			}
		}

		httpContext.Response = &http.Response{
			StatusCode: responseWriter.WrittenStatusCode,
			Header:     responseWriter.Header(),
		}
		httpContext.ResponseBody = responseWriter.WrittenBody
	}

	// Handle the case when no response was produced.

	if !responseWriter.WriteHeaderCalled {
		responseErrorHandler(
			altshiftHttpContext.WithHttpContextValue(request.Context(), httpContext),
			&muxTypesResponseError.ResponseError{
				ServerError: altshiftErrors.NewWithTrace(muxErrors.ErrNoResponseWritten),
			},
			responseWriter,
		)
	}

	if doneCallback := bm.DoneCallback; doneCallback != nil {
		doneCallback(altshiftHttpContext.WithHttpContextValue(request.Context(), httpContext))
	}
}

type Mux struct {
	baseMux
	EndpointMap map[string]map[string]*endpointPkg.Endpoint
}

func muxHandleRequest(
	mux *Mux,
	request *http.Request,
	responseWriter http.ResponseWriter,
) (*muxTypesResponse.Response, *muxTypesResponseError.ResponseError) {
	if mux == nil {
		return nil, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("mux")),
		}
	}

	if request == nil {
		return nil, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request")),
		}
	}

	requestHeader := request.Header
	if requestHeader == nil {
		return nil, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request header")),
		}
	}

	httpContext, ok := request.Context().Value(MuxHttpContextContextKey).(*altshiftHttpTypes.HttpContext)
	if !ok {
		return nil, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(muxErrors.ErrCouldNotObtainHttpContext),
		}
	}
	if httpContext == nil {
		return nil, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("http context")),
		}
	}

	// Locate the endpoint.

	endpoint, methodToEndpoint, responseError := muxInternalMux.GetEndpoint(
		mux.EndpointMap,
		request,
	)
	if responseError != nil {
		return nil, responseError
	}

	// There exists no endpoint for the given method,
	if endpoint == nil {
		// and for no other methods either, which is an error (as "Not Found" should be produced by `GetEndpoint`)
		if len(methodToEndpoint) == 0 {
			return nil, &muxTypesResponseError.ResponseError{
				ServerError: altshiftErrors.NewWithTrace(nil_error.New("endpoint specification")),
			}
		}

		return handleUnmatchedMethod(request, requestHeader, methodToEndpoint)
	}

	corsHeaderEntries, responseError := endpointCorsHeaderEntries(endpoint, request)
	if responseError != nil {
		return nil, responseError
	}

	// Perform rate limiting, if specified.

	if rateLimitingConfiguration := endpoint.RateLimitingConfiguration; rateLimitingConfiguration != nil {
		if responseError := muxInternalMux.HandleRateLimiting(rateLimitingConfiguration, request); responseError != nil {
			responseError.Headers = append(responseError.Headers, corsHeaderEntries...)
			return nil, responseError
		}
	}

	// Examine fetch metadata

	if !endpoint.DisableFetchMetadata {
		if responseError := muxInternalMux.HandleFetchMetadata(requestHeader, request.Method); responseError != nil {
			responseError.Headers = append(responseError.Headers, corsHeaderEntries...)
			return nil, responseError
		}
	}

	// Check authentication.

	if authenticationParser := endpoint.AuthenticationParser; !utils.IsNil(authenticationParser) {
		parsedAuthentication, responseError := authenticationParser.Parse(request)
		if responseError != nil {
			responseError.Headers = append(responseError.Headers, corsHeaderEntries...)
			return nil, responseError
		}

		request = request.WithContext(
			context.WithValue(request.Context(), utils2.ParsedRequestAuthenticationContextKey, parsedAuthentication),
		)

		if usererAuthenticationData, ok := parsedAuthentication.(userer.Userer); ok {
			if !utils.IsNil(usererAuthenticationData) {
				httpContext.User = usererAuthenticationData.GetUser()
			}
		}
	}

	// Obtain the parsed url.

	if urlParser := endpoint.UrlParser; !utils.IsNil(urlParser) {
		parsedUrl, responseError := urlParser.Parse(request)
		if responseError != nil {
			responseError.Headers = append(responseError.Headers, corsHeaderEntries...)
			return nil, responseError
		}

		request = request.WithContext(
			context.WithValue(request.Context(), utils2.ParsedRequestUrlContextKey, parsedUrl),
		)
	}

	// Obtain the parsed header.

	if headerParser := endpoint.HeaderParser; !utils.IsNil(headerParser) {
		parsedHeader, responseError := headerParser.Parse(request)
		if responseError != nil {
			responseError.Headers = append(responseError.Headers, corsHeaderEntries...)
			return nil, responseError
		}

		request = request.WithContext(
			context.WithValue(request.Context(), utils2.ParsedRequestHeaderContextKey, parsedHeader),
		)
	}

	// Validate and obtain the request body.
	request, requestBody, responseError := handleRequestBody(endpoint, request, responseWriter, requestHeader, httpContext)
	if responseError != nil {
		responseError.Headers = append(responseError.Headers, corsHeaderEntries...)
		return nil, responseError
	}

	// Produce the response (handler and/or static content).
	response, responseError := produceResponse(endpoint, request, requestBody, requestHeader)
	if responseError != nil {
		responseError.Headers = append(responseError.Headers, corsHeaderEntries...)
		return nil, responseError
	}
	if response == nil {
		response = &muxTypesResponse.Response{}
	}

	response.Headers = append(response.Headers, corsHeaderEntries...)
	return response, nil
}

// handleUnmatchedMethod produces the response for a request whose path exists but whose
// method has no endpoint: an OPTIONS response (optionally with CORS preflight headers) or
// a 405 Method Not Allowed. methodToEndpoint must be non-empty.
func handleUnmatchedMethod(
	request *http.Request,
	requestHeader http.Header,
	methodToEndpoint map[string]*endpointPkg.Endpoint,
) (*muxTypesResponse.Response, *muxTypesResponseError.ResponseError) {
	var allowedMethods []string
	var corsEndpoints []*endpointPkg.Endpoint

	for method, otherEndpoint := range methodToEndpoint {
		if otherEndpoint == nil {
			continue
		}

		allowedMethods = append(allowedMethods, method)

		if corsParser := otherEndpoint.CorsParser; !utils.IsNil(corsParser) {
			corsEndpoints = append(corsEndpoints, otherEndpoint)
		}
	}

	if _, ok := methodToEndpoint[http.MethodHead]; !ok {
		if _, ok := methodToEndpoint[http.MethodGet]; ok {
			allowedMethods = append(allowedMethods, http.MethodHead)
		}
	}
	if _, ok := methodToEndpoint[http.MethodOptions]; !ok {
		allowedMethods = append(allowedMethods, http.MethodOptions)
	}
	slices.Sort(allowedMethods)

	expectedMethodsString := strings.Join(allowedMethods, ", ")
	headerEntries := []*muxTypesResponse.HeaderEntry{{Name: "Allow", Value: expectedMethodsString}}

	if strings.ToUpper(request.Method) == http.MethodOptions {
		if len(corsEndpoints) > 0 {
			var corsConfiguration altshiftHttpTypes.CorsConfiguration
			accessControlRequestMethod := strings.ToUpper(requestHeader.Get("Access-Control-Request-Method"))
			accessControlRequestHeaders := requestHeader.Get("Access-Control-Request-Headers")

			for _, corsEndpoint := range corsEndpoints {
				method := strings.ToUpper(corsEndpoint.Method)

				corsConfiguration.Methods = append(corsConfiguration.Methods, strings.ToUpper(method))
				if method == http.MethodGet {
					corsConfiguration.Methods = append(corsConfiguration.Methods, http.MethodHead)
				}

				if method != accessControlRequestMethod {
					continue
				}

				corsParser := corsEndpoint.CorsParser
				// Sanity check. Should not be `nil` based on the previous check.
				if utils.IsNil(corsParser) {
					return nil, &muxTypesResponseError.ResponseError{
						ServerError: altshiftErrors.NewWithTrace(
							nil_error.NewWithInstance("request parser", "cors"),
						),
					}
				}

				endpointCorsConfiguration, responseError := corsParser.Parse(request)
				if responseError != nil {
					return nil, responseError
				}
				if endpointCorsConfiguration == nil {
					continue
				}

				corsConfiguration.Origin = endpointCorsConfiguration.Origin
				corsConfiguration.Headers = endpointCorsConfiguration.Headers
				corsConfiguration.Credentials = endpointCorsConfiguration.Credentials
				corsConfiguration.MaxAge = endpointCorsConfiguration.MaxAge
			}

			if origin := corsConfiguration.Origin; origin != "" {
				headerEntries = append(
					headerEntries,
					&muxTypesResponse.HeaderEntry{Name: "Access-Control-Allow-Origin", Value: origin},
				)
			}

			if methods := corsConfiguration.Methods; len(methods) > 0 {
				uniqueMethods := altshiftIter.Set(methods)
				slices.Sort(uniqueMethods)

				headerEntries = append(
					headerEntries,
					&muxTypesResponse.HeaderEntry{Name: "Access-Control-Allow-Methods", Value: strings.Join(uniqueMethods, ", ")},
				)
			}

			if headers := corsConfiguration.Headers; len(headers) > 0 && accessControlRequestHeaders != "" {
				headerEntries = append(
					headerEntries,
					&muxTypesResponse.HeaderEntry{Name: "Access-Control-Allow-Headers", Value: strings.Join(headers, ", ")},
				)
			}

			if credentials := corsConfiguration.Credentials; credentials {
				headerEntries = append(
					headerEntries,
					&muxTypesResponse.HeaderEntry{Name: "Access-Control-Allow-Credentials", Value: "true"},
				)
			}

			if maxAge := corsConfiguration.MaxAge; maxAge > 0 {
				headerEntries = append(
					headerEntries,
					&muxTypesResponse.HeaderEntry{Name: "Access-Control-Max-Age", Value: fmt.Sprintf("%d", maxAge)},
				)
			}
		}

		return &muxTypesResponse.Response{Headers: headerEntries}, nil
	}

	return nil, &muxTypesResponseError.ResponseError{
		ProblemDetail: problem_detail.New(
			http.StatusMethodNotAllowed,
			problem_detail_config.WithDetail(fmt.Sprintf("Expected %s.", expectedMethodsString)),
		),
		Headers: headerEntries,
	}
}

// endpointCorsHeaderEntries computes the CORS response headers for the matched endpoint's
// actual (non-preflight) request from its CorsParser.
func endpointCorsHeaderEntries(
	endpoint *endpointPkg.Endpoint,
	request *http.Request,
) ([]*muxTypesResponse.HeaderEntry, *muxTypesResponseError.ResponseError) {
	corsParser := endpoint.CorsParser
	if utils.IsNil(corsParser) {
		return nil, nil
	}

	corsConfiguration, responseError := corsParser.Parse(request)
	if responseError != nil {
		return nil, responseError
	}
	if corsConfiguration == nil {
		return nil, nil
	}

	var corsHeaderEntries []*muxTypesResponse.HeaderEntry
	if origin := corsConfiguration.Origin; origin != "" {
		corsHeaderEntries = append(
			corsHeaderEntries,
			&muxTypesResponse.HeaderEntry{Name: "Access-Control-Allow-Origin", Value: origin},
		)
	}
	if credentials := corsConfiguration.Credentials; credentials {
		corsHeaderEntries = append(
			corsHeaderEntries,
			&muxTypesResponse.HeaderEntry{Name: "Access-Control-Allow-Credentials", Value: "true"},
		)
	}
	if exposeHeaders := corsConfiguration.ExposeHeaders; len(exposeHeaders) > 0 {
		corsHeaderEntries = append(
			corsHeaderEntries,
			&muxTypesResponse.HeaderEntry{
				Name:  "Access-Control-Expose-Headers",
				Value: strings.Join(exposeHeaders, ", "),
			},
		)
	}

	return corsHeaderEntries, nil
}

// handleRequestBody validates and obtains the request body according to the endpoint's body
// loader, records it on the http context, and parses it. It returns the (possibly updated)
// request carrying the parsed body. Returned errors do not carry CORS headers; the caller
// attaches them.
func handleRequestBody(
	endpoint *endpointPkg.Endpoint,
	request *http.Request,
	responseWriter http.ResponseWriter,
	requestHeader http.Header,
	httpContext *altshiftHttpTypes.HttpContext,
) (*http.Request, []byte, *muxTypesResponseError.ResponseError) {
	var expectedContentType string
	var maxBytes int64
	var bodyParser body_parser.BodyParser[any]

	emptyOption := body_setting.Optional
	if request.Method == http.MethodGet || request.Method == http.MethodHead || request.Method == http.MethodTrace || request.Method == http.MethodDelete {
		emptyOption = body_setting.Forbidden
	}

	bodyLoader := endpoint.BodyLoader
	if bodyLoader != nil {
		emptyOption = bodyLoader.Setting
		expectedContentType = bodyLoader.ContentType
		maxBytes = bodyLoader.MaxBytes
		bodyParser = bodyLoader.Parser
	}

	// Validate Content-Type (parse and match header value against accepted value)
	if expectedContentType != "" {
		if responseError := muxInternalMux.ValidateContentType(expectedContentType, requestHeader); responseError != nil {
			return request, nil, responseError
		}
	}

	if emptyOption == body_setting.Forbidden {
		request.Body = http.MaxBytesReader(responseWriter, request.Body, 0)
	} else if maxBytes > 0 {
		request.Body = http.MaxBytesReader(responseWriter, request.Body, maxBytes)
	}

	allowEmptyBody := emptyOption == body_setting.Optional || emptyOption == body_setting.Forbidden

	// Validate Content-Length (parse and check if empty is accepted)
	if responseError := muxInternalMux.ValidateContentLength(allowEmptyBody, requestHeader); responseError != nil {
		return request, nil, responseError
	}

	// Obtain the request body
	requestBody, responseError := muxInternalMux.ObtainRequestBody(
		request.Context(),
		request.ContentLength,
		request.Body,
		maxBytes,
	)
	if responseError != nil {
		return request, nil, responseError
	}
	httpContext.RequestBody = requestBody

	if !allowEmptyBody && len(requestBody) == 0 {
		return request, nil, &muxTypesResponseError.ResponseError{
			ProblemDetail: problem_detail.New(
				http.StatusBadRequest,
				problem_detail_config.WithDetail("A body is expected."),
			),
		}
	}

	// Basic check to see if the request body conforms to the expected content type.
	switch expectedContentType {
	case "application/json":
		if !jsontext.Value(requestBody).IsValid() {
			return request, nil, &muxTypesResponseError.ResponseError{
				ProblemDetail: problem_detail.New(
					http.StatusBadRequest,
					problem_detail_config.WithDetail("Invalid JSON body."),
				),
			}
		}
	}

	// Parse the body. The explicit nil check lets the static analyzer prove non-nilness;
	// utils.IsNil additionally rejects a typed-nil parser.
	if bodyParser != nil && !utils.IsNil(bodyParser) {
		parsedBody, responseError := bodyParser.Parse(request, requestBody)
		if responseError != nil {
			return request, nil, responseError
		}

		request = request.WithContext(
			context.WithValue(request.Context(), utils2.ParsedRequestBodyContextKey, parsedBody),
		)
	}

	return request, requestBody, nil
}

// produceResponse builds the endpoint's response from its handler and/or static content and
// appends the Vary header. Returned errors do not carry CORS headers; the caller attaches them.
func produceResponse(
	endpoint *endpointPkg.Endpoint,
	request *http.Request,
	requestBody []byte,
	requestHeader http.Header,
) (*muxTypesResponse.Response, *muxTypesResponseError.ResponseError) {
	var handlerResponseHeaders []*muxTypesResponse.HeaderEntry
	var response *muxTypesResponse.Response
	var responseError *muxTypesResponseError.ResponseError

	// Respond with dynamic content via a handler.
	handler := endpoint.Handler
	if handler != nil {
		response, responseError = handler(request, requestBody)
		if responseError != nil {
			return nil, responseError
		}
		if response != nil {
			handlerResponseHeaders = response.Headers
		}
	}

	// A handler that produced a status of its own has answered the request. An endpoint can then
	// carry static content and still respond with something else when the occasion calls for it --
	// a redirect, say -- rather than having to choose between the two up front. A handler that
	// produced no status is taken to be contributing headers to the static content instead, which
	// is the arrangement further down.
	handlerAnswered := response != nil && response.StatusCode != 0

	// Respond with static content.
	staticContent := endpoint.StaticContent
	if staticContent != nil && !handlerAnswered {
		var isCached bool
		isCached, responseError = muxInternalMux.ObtainIsCached(staticContent, requestHeader)
		if responseError != nil {
			return nil, responseError
		}

		var acceptEncoding *altshiftHttpTypes.AcceptEncoding
		contentNegotiation, _ := request.Context().Value(muxContext.ContentNegotiationContextKey).(*altshiftHttpTypes.ContentNegotiation)
		if contentNegotiation != nil {
			acceptEncoding = contentNegotiation.AcceptEncoding
		}

		response, responseError = muxInternalMux.ObtainStaticContentResponse(
			staticContent,
			isCached,
			requestHeader,
			acceptEncoding,
		)
		if responseError != nil {
			return nil, responseError
		}

		// A body behind a session must not be handed to a shared cache. The
		// header is set where the content is built, which is before anything
		// knows whether the endpoint would end up gated, so it is answered for
		// here instead -- where whether a session is required is finally known.
		//
		// Downgraded only, never the other way. A service may have good reason
		// to keep a reachable body out of shared caches; none to put a gated one
		// in, and a mistake in that direction is not one the reader would see.
		if response != nil && !endpoint.Public {
			response.Headers = privateCacheControlHeaders(response.Headers)
		}
	}

	if response == nil {
		response = &muxTypesResponse.Response{}
	}

	// If both a handler and static content are specified, the handler response headers are added to the static content
	// response headers. When the handler answered, its response is already the one being returned,
	// headers and all.
	if handler != nil && staticContent != nil && !handlerAnswered {
		response.Headers = append(response.Headers, handlerResponseHeaders...)
	}

	if !endpoint.DisableFetchMetadata {
		response.Headers = append(
			response.Headers,
			&muxTypesResponse.HeaderEntry{
				Name:  "Vary",
				Value: "Sec-Fetch-Dest, Sec-Fetch-Mode, Sec-Fetch-Site",
			},
		)
	}

	return response, nil
}

// privateCacheControlHeaders returns the headers with any Cache-Control that
// says `public` saying `private` instead.
//
// The entries belong to the endpoint and are read by every request it serves,
// so the one that changes is replaced rather than written to: a shared header
// edited in place would race, and would outlast the request that did it.
func privateCacheControlHeaders(headers []*muxTypesResponse.HeaderEntry) []*muxTypesResponse.HeaderEntry {
	out := headers
	copied := false

	for i, header := range headers {
		if header == nil || !strings.EqualFold(header.Name, "Cache-Control") {
			continue
		}

		parsed, err := cache_control.Parse([]byte(header.Value))
		if err != nil || parsed == nil || !parsed.Public() {
			continue
		}

		if !copied {
			// Not slices.Clone: it answers nil for a nil slice, and the
			// assignment below would then be to nothing. The loop cannot be
			// running over a nil slice, but that is not something a reader --
			// or a checker -- should have to work out from here.
			cloned := make([]*muxTypesResponse.HeaderEntry, len(headers))
			copy(cloned, headers)
			out = cloned
			copied = true
		}

		parsed.SetVisibility(false)
		replacement := *header
		replacement.Value = parsed.String()
		out[i] = &replacement
	}

	return out
}

func (mux *Mux) ServeHTTP(originalResponseWriter http.ResponseWriter, request *http.Request) {
	mux.ServeHttpWithCallback(
		originalResponseWriter,
		request,
		func(request *http.Request, responseWriter *muxTypesResponseWriter.ResponseWriter) (*muxTypesResponse.Response, *muxTypesResponseError.ResponseError) {
			response, responseError := muxHandleRequest(mux, request, responseWriter)
			if responseError != nil {
				responseError.ProblemDetailConverter = mux.ProblemDetailConverter
			}

			if responseWriter != nil {
				responseWriter.DefaultHeaders = mux.DefaultHeaders
				responseWriter.DefaultDocumentHeaders = mux.DefaultDocumentHeaders
			}

			return response, responseError
		},
	)
}

func (mux *Mux) Add(endpoints ...*endpointPkg.Endpoint) {
	if len(endpoints) == 0 {
		return
	}

	endpointMap := mux.EndpointMap
	if endpointMap == nil {
		endpointMap = make(map[string]map[string]*endpointPkg.Endpoint)
	}

	for _, endpoint := range endpoints {
		if endpoint == nil {
			continue
		}

		if endpoint.Method == "" {
			slog.Warn("Endpoint with empty method.")
		}

		if endpoint.Path == "" {
			slog.Warn("Endpoint with empty path.")
		}

		// Public and AuthenticationParser both say whether a session is needed,
		// and only the parser enforces it. They are warned about separately
		// because they go wrong in opposite ways.

		// Declared as needing a session, with nothing to require one: served to
		// anyone who asks.
		if !endpoint.Public && utils.IsNil(endpoint.AuthenticationParser) {
			slog.Warn(
				fmt.Sprintf(
					"Non-public endpoint without authentication parser: %s %s.",
					endpoint.Method,
					endpoint.Path,
				),
			)
		}

		// Requiring a session while declaring that it does not. The body is
		// safe -- the parser refuses it -- but everything that reads Public
		// rather than the parser is now wrong about it: the Cache-Control it
		// was generated with offers it to shared caches, and a generated client
		// will not send credentials. Nothing here fails, which is why it is
		// worth saying out loud.
		if endpoint.Public && !utils.IsNil(endpoint.AuthenticationParser) {
			slog.Warn(
				fmt.Sprintf(
					"Public endpoint with an authentication parser: %s %s.",
					endpoint.Method,
					endpoint.Path,
				),
			)
		}

		methodToEndpoint, ok := endpointMap[endpoint.Path]
		if !ok {
			methodToEndpoint = make(map[string]*endpointPkg.Endpoint)
			endpointMap[endpoint.Path] = methodToEndpoint
		}

		methodToEndpoint[strings.ToUpper(endpoint.Method)] = endpoint
	}

	mux.EndpointMap = endpointMap
}

func (mux *Mux) Delete(endpoints ...*endpointPkg.Endpoint) {
	if len(endpoints) == 0 {
		return
	}

	endpointSpecificationMap := mux.EndpointMap
	if endpointSpecificationMap == nil {
		return
	}

	for _, endpoint := range endpoints {
		methodToEndpoint, ok := endpointSpecificationMap[endpoint.Path]
		if !ok {
			return
		}

		delete(methodToEndpoint, strings.ToUpper(endpoint.Method))

		if len(methodToEndpoint) == 0 {
			delete(endpointSpecificationMap, endpoint.Path)
		}
	}
}

func (mux *Mux) Get(path string, method string) *endpointPkg.Endpoint {
	endpointMap := mux.EndpointMap
	if endpointMap == nil {
		return nil
	}

	methodToEndpoint, ok := endpointMap[path]
	if !ok || methodToEndpoint == nil {
		return nil
	}

	return methodToEndpoint[strings.ToUpper(method)]
}

func (mux *Mux) GetDocumentEndpointSpecifications() []*endpointPkg.Endpoint {
	var endpoints []*endpointPkg.Endpoint

	for _, methodMap := range mux.EndpointMap {
		for _, endpoint := range methodMap {
			staticContent := endpoint.StaticContent
			if staticContent == nil {
				continue
			}

			var isDocument bool
			for _, header := range staticContent.Headers {
				if header == nil {
					continue
				}

				if strings.ToLower(header.Name) == "content-type" && strings.ToLower(header.Value) == "text/html" {
					isDocument = true
					break
				}
			}

			if !isDocument {
				continue
			}

			endpoints = append(endpoints, endpoint)
		}
	}

	return endpoints
}

func (mux *Mux) DuplicateEndpointSpecification(endpoint *endpointPkg.Endpoint, routes ...string) error {
	if endpoint == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("endpoint"))
	}

	mux.Add(endpointPkg.Duplicate(endpoint, routes...)...)

	return nil
}

func (mux *Mux) GetContentSecurityPolicy() (*content_security_policy.ContentSecurityPolicy, error) {
	contentSecurityPolicyString := mux.DefaultDocumentHeaders[contentSecurityPolicyHeaderName]
	if contentSecurityPolicyString == "" {
		return nil, nil
	}

	csp, err := content_security_policy.Parse([]byte(contentSecurityPolicyString))
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	return csp, nil
}

func (mux *Mux) SetContentSecurityPolicy(csp *content_security_policy.ContentSecurityPolicy) error {
	defaultDocumentHeaders := mux.DefaultDocumentHeaders
	if defaultDocumentHeaders == nil {
		return altshiftErrors.NewWithTrace(nil_error.NewWithInstance("map", "default document headers"))
	}

	if csp == nil {
		defaultDocumentHeaders[contentSecurityPolicyHeaderName] = ""
	} else {
		defaultDocumentHeaders[contentSecurityPolicyHeaderName] = csp.String()
	}

	return nil
}

func New(endpoints ...*endpointPkg.Endpoint) *Mux {
	var mux Mux
	mux.DefaultHeaders = maps.Clone(muxTypesResponseWriter.DefaultHeaders)
	mux.DefaultDocumentHeaders = maps.Clone(muxTypesResponseWriter.DefaultDocumentHeaders)
	// Set on the mux alone, not on a vhost mux that fronts it: both call the callback, and a
	// request served through the two would otherwise be logged by each of them. Assigning the field
	// replaces it; assigning nil silences it.
	mux.DoneCallback = muxInternal.DefaultDoneCallback
	mux.Add(endpoints...)
	return &mux
}
