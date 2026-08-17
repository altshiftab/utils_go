package mux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	altshiftContext "github.com/altshiftab/utils_go/pkg/context"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	muxErrors "github.com/altshiftab/utils_go/pkg/http/mux/errors"
	muxTypes "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	muxTypesStaticContent "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint/static_content"
	muxTypesRateLimiting "github.com/altshiftab/utils_go/pkg/http/mux/types/rate_limiting"
	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	muxTypesResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/content_type"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

func HandleRateLimiting(
	rateLimitingConfiguration *muxTypesRateLimiting.RateLimitingConfiguration,
	request *http.Request,
) *muxTypesResponseError.ResponseError {
	if rateLimitingConfiguration == nil {
		return nil
	}

	if request == nil {
		return &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request")),
		}
	}

	getKeyFunc := rateLimitingConfiguration.GetKey
	if getKeyFunc == nil {
		getKeyFunc = muxTypesRateLimiting.DefaultGetRateLimitingKey
	}

	key, err := getKeyFunc(request)
	if err != nil {
		return &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(fmt.Errorf("get key func: %w", err)),
		}
	}

	rateLimitingConfiguration.Lookup.Mutex.Lock()
	if rateLimitingConfiguration.Lookup.Map == nil {
		rateLimitingConfiguration.Lookup.Map = make(map[string]*muxTypesRateLimiting.TimerRateLimiter)
	}
	rateLimitingConfiguration.Lookup.Mutex.Unlock()

	timerRateLimiter, ok := rateLimitingConfiguration.Lookup.Map[key]
	if !ok {
		timerRateLimiter = &muxTypesRateLimiting.TimerRateLimiter{
			RateLimiter: muxTypesRateLimiting.RateLimiter{
				Bucket:           make([]*time.Time, rateLimitingConfiguration.NumRequests),
				NumSecondsExpiry: rateLimitingConfiguration.NumSecondsExpiration,
			},
		}
		rateLimitingConfiguration.Lookup.Map[key] = timerRateLimiter
	}

	if timerRateLimiter.Timer != nil {
		timerRateLimiter.Timer.Stop()
	}
	timerRateLimiter.Timer = time.AfterFunc(
		time.Duration(2*timerRateLimiter.NumSecondsExpiry)*time.Second,
		func() {
			rateLimitingConfiguration.Lookup.Mutex.Lock()
			defer rateLimitingConfiguration.Lookup.Mutex.Unlock()
			if timerRateLimiter.NumOccupied == 0 {
				delete(rateLimitingConfiguration.Lookup.Map, key)
			}
		},
	)

	expirationTime, full := timerRateLimiter.Claim()
	if full {
		return &muxTypesResponseError.ResponseError{
			ProblemDetail: problem_detail.New(http.StatusTooManyRequests),
			Headers: []*muxTypesResponse.HeaderEntry{
				{
					Name:  "Retry-After",
					Value: expirationTime.UTC().Format(http.TimeFormat),
				},
			},
		}
	}

	return nil
}

func HandleFetchMetadata(requestHeader http.Header, method string) *muxTypesResponseError.ResponseError {
	if requestHeader == nil {
		return &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request header")),
		}
	}

	if method == "" {
		return &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(empty_error.New("method")),
		}
	}

	// NOTE: This check is opinionated; embedding is not allowed — cross-site iframe/frame navigations are
	// rejected here in addition to being blocked by the default `frame-ancestors 'none'`. For custom fetch
	// metadata logic, disable this check and implement your own in the firewall configuration e.g., plus add
	// the `Vary` header.

	fetchSite := requestHeader.Get("Sec-Fetch-Site")
	if fetchSite == "" || fetchSite == "same-origin" || fetchSite == "same-site" || fetchSite == "none" {
		return nil
	}

	fetchMode := requestHeader.Get("Sec-Fetch-Mode")
	fetchDest := requestHeader.Get("Sec-Fetch-Dest")
	// Top-level navigations only. The spec value is "document", but browsers emit "empty" in some
	// legitimate flows (observed: Chrome on the navigation completing a Google IAP sign-in redirect).
	if fetchMode == "navigate" && (fetchDest == "document" || fetchDest == "empty") && method == http.MethodGet {
		return nil
	}

	return &muxTypesResponseError.ResponseError{
		ProblemDetail: problem_detail.New(
			http.StatusForbidden,
			problem_detail_config.WithDetail("Cross-site request blocked by Fetch-Metadata policy."),
		),
	}
}

func ValidateContentType(expectedContentType string, requestHeader http.Header) *muxTypesResponseError.ResponseError {
	// TODO: Error case?
	if expectedContentType == "" {
		return nil
	}

	if requestHeader == nil {
		return &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request header")),
		}
	}

	acceptedContentTypeHeaders := []*muxTypesResponse.HeaderEntry{{Name: "Accept", Value: expectedContentType}}

	if _, ok := requestHeader["Content-Type"]; !ok {
		return &muxTypesResponseError.ResponseError{
			ProblemDetail: problem_detail.New(
				http.StatusUnsupportedMediaType,
				problem_detail_config.WithDetail("Missing Content-Type."),
			),
			Headers: acceptedContentTypeHeaders,
		}
	}

	contentTypeData := []byte(requestHeader.Get("Content-Type"))
	contentType, err := content_type.Parse(contentTypeData)
	if err != nil {
		wrappedErr := altshiftErrors.New(fmt.Errorf("content type parse: %w", err), contentTypeData)
		if altshiftErrors.IsAny(err, altshiftErrors.ErrSyntaxError, altshiftErrors.ErrSemanticError) {
			return &muxTypesResponseError.ResponseError{
				ClientError: wrappedErr,
				ProblemDetail: problem_detail.New(
					http.StatusBadRequest,
					problem_detail_config.WithDetail("Malformed Content-Type."),
				),
			}
		}
		return &muxTypesResponseError.ResponseError{ServerError: wrappedErr}
	}
	if contentType == nil {
		return &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("content type")),
		}
	}

	// TODO: The specification could require a certain charset too?
	fullNormalizeContentTypeString := contentType.GetFullType(true)
	if fullNormalizeContentTypeString != expectedContentType {
		return &muxTypesResponseError.ResponseError{
			ProblemDetail: problem_detail.New(
				http.StatusUnsupportedMediaType,
				problem_detail_config.WithDetail(
					fmt.Sprintf(
						"Expected Content-Type to be %q, observed %q.",
						expectedContentType,
						fullNormalizeContentTypeString,
					),
				),
			),
			Headers: acceptedContentTypeHeaders,
		}
	}

	return nil
}

func ValidateContentLength(allowEmpty bool, requestHeader http.Header) *muxTypesResponseError.ResponseError {
	if requestHeader == nil {
		return &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request header")),
		}
	}

	zeroContentLengthStatusCode := http.StatusLengthRequired
	zeroContentLengthMessage := "A body is expected; Content-Length must be set."

	var contentLength uint64
	if _, ok := requestHeader["Content-Length"]; ok {
		var err error
		headerValue := requestHeader.Get("Content-Length")
		contentLength, err = strconv.ParseUint(headerValue, 10, 64)
		if err != nil {
			return &muxTypesResponseError.ResponseError{
				ProblemDetail: problem_detail.New(
					http.StatusBadRequest,
					problem_detail_config.WithDetail("Malformed Content-Length."),
				),
				ClientError: altshiftErrors.NewWithTrace(
					fmt.Errorf("strconv parse uint: %w", err),
					headerValue, 10, 64,
				),
			}
		}
		if contentLength == 0 {
			zeroContentLengthStatusCode = http.StatusBadRequest
			zeroContentLengthMessage = "A body is expected; Content-Length cannot be 0."
		}
	}

	if !allowEmpty && contentLength == 0 {
		return &muxTypesResponseError.ResponseError{
			ProblemDetail: problem_detail.New(
				zeroContentLengthStatusCode,
				problem_detail_config.WithDetail(zeroContentLengthMessage),
			),
		}
	}

	return nil
}

func ObtainRequestBody(
	ctx context.Context,
	contentLength int64,
	bodyReader io.ReadCloser,
	maxBytes int64,
) ([]byte, *muxTypesResponseError.ResponseError) {
	if bodyReader == nil {
		return nil, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("http request body reader")),
		}
	}

	if contentLength >= 0 {
		if contentLength > 0 && maxBytes > 0 && contentLength > maxBytes {
			return nil, &muxTypesResponseError.ResponseError{
				ProblemDetail: problem_detail.New(
					http.StatusRequestEntityTooLarge,
					problem_detail_config.WithDetail(fmt.Sprintf("Limit: %d bytes", maxBytes)),
				),
			}
		}

		var err error
		requestBody, err := io.ReadAll(bodyReader)
		if err != nil {
			wrappedErr := altshiftErrors.NewWithTrace(
				fmt.Errorf("io read all (request body): %w", err),
				bodyReader,
			)

			// NOTE: "Request Entity Too Large" should always be picked up by the content length check, but add this
			// for completion.
			var maxBytesError *http.MaxBytesError
			if errors.As(err, &maxBytesError) {
				return nil, &muxTypesResponseError.ResponseError{
					ClientError: wrappedErr,
					ProblemDetail: problem_detail.New(
						http.StatusRequestEntityTooLarge,
						problem_detail_config.WithDetail(fmt.Sprintf("Limit: %d bytes", maxBytesError.Limit)),
					),
				}
			}

			return nil, &muxTypesResponseError.ResponseError{ServerError: wrappedErr}
		}
		defer func() {
			if err := bodyReader.Close(); err != nil {
				slog.WarnContext(
					altshiftContext.WithError(
						ctx,
						altshiftErrors.NewWithTrace(fmt.Errorf("body reader close: %w", err), bodyReader),
					),
					"An error occurred when closing the request body reader.",
				)
			}
		}()

		return requestBody, nil
	}

	return nil, nil
}

func GetEndpoint(
	endpointSpecificationMap map[string]map[string]*muxTypes.Endpoint,
	request *http.Request,
) (*muxTypes.Endpoint, map[string]*muxTypes.Endpoint, *muxTypesResponseError.ResponseError) {
	if len(endpointSpecificationMap) == 0 {
		return nil, nil, &muxTypesResponseError.ResponseError{
			ProblemDetail: problem_detail.New(http.StatusNotFound),
		}
	}

	if request == nil {
		return nil, nil, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request")),
		}
	}

	requestUrl := request.URL
	if requestUrl == nil {
		return nil, nil, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request url")),
		}
	}

	requestMethod := strings.ToUpper(request.Method)
	effectiveLookupMethod := requestMethod
	if requestMethod == http.MethodHead {
		// A HEAD request is to be processed as if it were a GET request. But signal not to write a body.
		effectiveLookupMethod = http.MethodGet
	}

	methodToEndpointSpecification, ok := endpointSpecificationMap[requestUrl.Path]
	if !ok {
		return nil, nil, &muxTypesResponseError.ResponseError{
			ProblemDetail: problem_detail.New(http.StatusNotFound),
		}
	}

	endpointSpecification, ok := methodToEndpointSpecification[effectiveLookupMethod]
	if !ok {
		return nil, methodToEndpointSpecification, nil
	}

	return endpointSpecification, methodToEndpointSpecification, nil
}

func ObtainIsCached(staticContent *muxTypesStaticContent.StaticContent, requestHeader http.Header) (bool, *muxTypesResponseError.ResponseError) {
	if staticContent == nil {
		return false, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("static content")),
		}
	}

	if requestHeader == nil {
		return false, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request header")),
		}
	}

	isCached := altshiftHttpUtils.IfNoneMatchCacheHit(requestHeader.Get("If-None-Match"), staticContent.Etag)
	if !isCached {
		var err error
		ifModifiedSince := requestHeader.Get("If-Modified-Since")
		lastModified := staticContent.LastModified
		isCached, err = altshiftHttpUtils.IfModifiedSinceCacheHit(ifModifiedSince, lastModified)
		if err != nil {
			wrappedErr := altshiftErrors.New(
				fmt.Errorf("if modified since cache hit: %w", err),
				ifModifiedSince,
				lastModified,
			)
			if errors.Is(err, altshiftHttpErrors.ErrBadIfModifiedSinceTimestamp) {
				return false, &muxTypesResponseError.ResponseError{
					ProblemDetail: problem_detail.New(
						http.StatusBadRequest,
						problem_detail_config.WithDetail("Bad If-Modified-Since value"),
					),
					ClientError: wrappedErr,
				}
			}

			return false, &muxTypesResponseError.ResponseError{ServerError: wrappedErr}
		}
	}

	return isCached, nil
}

func ObtainStaticContentResponse(
	staticContent *muxTypesStaticContent.StaticContent,
	isCached bool,
	requestHeader http.Header,
	acceptEncoding *altshiftHttpTypes.AcceptEncoding,
) (*muxTypesResponse.Response, *muxTypesResponseError.ResponseError) {
	if staticContent == nil {
		return nil, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("static content")),
		}
	}

	if requestHeader == nil {
		return nil, &muxTypesResponseError.ResponseError{
			ServerError: altshiftErrors.NewWithTrace(nil_error.New("request header")),
		}
	}

	// NOTE: It is up to the user to provide the `Vary` header.
	response := &muxTypesResponse.Response{
		Headers:            staticContent.Headers,
		InlineScriptHashes: staticContent.InlineScriptHashes,
	}
	if isCached {
		response.StatusCode = http.StatusNotModified
	} else {
		encoding := altshiftHttpUtils.AcceptContentIdentity

		if acceptEncoding != nil {
			supportedEncodings := slices.Collect(maps.Keys(staticContent.ContentEncodingToData))
			contentEncodingToData := staticContent.ContentEncodingToData

			slices.SortFunc(supportedEncodings, func(a, b string) int {
				aData := contentEncodingToData[a].Data
				bData := contentEncodingToData[b].Data
				if len(aData) < len(bData) {
					return -1
				} else if len(aData) > len(bData) {
					return 1
				}
				return 0
			})

			encoding = altshiftHttpUtils.GetMatchingContentEncoding(
				acceptEncoding.GetPriorityOrderedEncodings(),
				supportedEncodings,
			)
		}

		if encoding == "" {
			// NOTE: The problem detail won't appear in the response body because not even `identity` is acceptable;
			//	rather, the problem detail specifies the status code only.
			return nil, &muxTypesResponseError.ResponseError{
				ProblemDetail: problem_detail.New(http.StatusNotAcceptable),
			}
		}

		response.StatusCode = http.StatusOK
		if encoding == altshiftHttpUtils.AcceptContentIdentity {
			response.Body = staticContent.Data
		} else {
			response.Headers = append(
				response.Headers,
				&muxTypesResponse.HeaderEntry{Name: "Content-Encoding", Value: encoding},
			)

			contentEncodingToData := staticContent.ContentEncodingToData
			if contentEncodingToData == nil {
				return nil, &muxTypesResponseError.ResponseError{
					ServerError: altshiftErrors.NewWithTrace(nil_error.New("content-encoding to data")),
				}
			}

			staticContentData, ok := contentEncodingToData[encoding]
			if !ok {
				return nil, &muxTypesResponseError.ResponseError{
					ServerError: altshiftErrors.NewWithTrace(muxErrors.ErrContentEncodingToDataNotOk),
				}
			}
			if staticContentData == nil {
				return nil, &muxTypesResponseError.ResponseError{
					ServerError: altshiftErrors.NewWithTrace(nil_error.New("static content data")),
				}
			}

			response.Body = staticContentData.Data
		}
	}

	return response, nil
}
