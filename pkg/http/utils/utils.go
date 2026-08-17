package utils

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptrace"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"

	altshiftContext "github.com/altshiftab/utils_go/pkg/context"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config/retry_config"
	altshiftTlsTypes "github.com/altshiftab/utils_go/pkg/tls/types"
	"github.com/altshiftab/utils_go/pkg/utils"
)

const AcceptContentIdentity = "identity"

// FetchPerformedMessage is the message a fetch is logged under. What is worth saying about one is in
// the HTTP context rather than here -- what was requested, and what came back -- so a log handler
// that reads that context may replace it with what it says.
const FetchPerformedMessage = "A fetch was performed."

// retryAfterHeaderDelay parses an HTTP Retry-After header into a delay. The header is
// either a non-negative number of seconds or an HTTP-date; for the latter the response
// Date header is used as the reference point when present. It returns nil when the
// header is absent or unparseable.
func retryAfterHeaderDelay(header http.Header) *time.Duration {
	if header == nil {
		return nil
	}

	retryAfterValue := header.Get("Retry-After")
	if retryAfterValue == "" {
		return nil
	}

	if retryAfterSeconds, err := strconv.Atoi(retryAfterValue); err == nil {
		// Add one more second for rounding.
		return new(time.Duration(retryAfterSeconds+1) * time.Second)
	}

	if retryAfterTimestamp, err := time.Parse(time.RFC1123, retryAfterValue); err == nil {
		referenceTime := time.Now()
		if responseDate, err := time.Parse(time.RFC1123, header.Get("Date")); err == nil {
			referenceTime = responseDate
		}
		return new(retryAfterTimestamp.Sub(referenceTime))
	}

	return nil
}

// retryWaitDuration determines how long to wait before retry attempt i (1-based),
// based on the previous response. It prefers, in order: a server-advised delay parsed
// from the response body by the configured RetryAfterFunc; the Retry-After header; and
// otherwise exponential back-off. A server-advised delay exceeding MaximumWaitTime
// yields giveUp=true (the caller would rather stop than wait that long); the back-off
// fallback is instead clamped to MaximumWaitTime.
func retryWaitDuration(
	retryConfig *retry_config.Config,
	response *http.Response,
	responseBody []byte,
	i int,
) (time.Duration, bool) {
	maximumWaitTime := retryConfig.MaximumWaitTime

	var advised *time.Duration
	if retryConfig.RetryAfterFunc != nil {
		advised = retryConfig.RetryAfterFunc(response, responseBody)
	}
	if advised == nil && response != nil {
		advised = retryAfterHeaderDelay(response.Header)
	}

	if advised != nil {
		if maximumWaitTime != 0 && *advised > maximumWaitTime {
			return 0, true
		}
		return max(*advised, 0), false
	}

	// baseDelay * 2^(i-1)
	waitDuration := retryConfig.BaseDelay * (1 << (i - 1))
	if maximumWaitTime != 0 {
		waitDuration = min(waitDuration, maximumWaitTime)
	}
	return waitDuration, false
}

//nolint:contextcheck // The logging context is deliberately detached from the caller context (which may be cancelled) while still carrying the HTTP metadata.
func fetch(ctx context.Context, request *http.Request, fetchConfig *fetch_config.Config) (*http.Response, []byte, error) {
	if request == nil {
		return nil, nil, nil
	}

	if fetchConfig == nil {
		return nil, nil, nil_error.New("fetch config")
	}

	httpContext, ok := ctx.Value(altshiftHttpContext.HttpContextContextKey).(*altshiftHttpTypes.HttpContext)
	if !ok || httpContext == nil {
		httpContext = &altshiftHttpTypes.HttpContext{}
	}

	httpContext.Request = request
	httpContext.RequestBody = fetchConfig.Body

	var err error

	defer func() {
		if slog.Default().Enabled(ctx, slog.LevelDebug) {
			debugCtx := altshiftHttpContext.WithHttpContextValue(ctx, httpContext)

			if err != nil {
				debugCtx = altshiftContext.WithError(debugCtx, err)
			}

			slog.DebugContext(
				debugCtx,
				FetchPerformedMessage,
				slog.Group(
					"event",
					slog.String("reason", FetchPerformedMessage),
				),
			)
		}
	}()

	ctxWithHttpContext := altshiftHttpContext.WithHttpContextValue(context.Background(), httpContext)

	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			if conn := info.Conn; conn != nil {
				httpContext.LocalAddr = conn.LocalAddr()
				httpContext.RemoteAddr = conn.RemoteAddr()
			}
		},
	}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))

	response, err := fetchConfig.HttpClient.Do(request) //nolint:gosec // generic fetch utility; fetching caller-supplied URLs is intended
	httpContext.Response = response
	if err != nil {
		return nil, nil, altshiftErrors.NewWithTraceCtx(
			ctxWithHttpContext,
			fmt.Errorf("http client do: %w", err),
		)
	}
	if response == nil {
		return nil, nil, altshiftErrors.NewWithTraceCtx(ctxWithHttpContext, nil_error.New("http response"))
	}
	responseBody := response.Body
	if utils.IsNil(responseBody) {
		return nil, nil, altshiftErrors.NewWithTraceCtx(ctxWithHttpContext, nil_error.New("http response body"))
	}

	var responseBodyData []byte
	if !fetchConfig.SkipReadResponseBody {
		responseBodyData, err = io.ReadAll(responseBody)
		defer func() {
			if err := responseBody.Close(); err != nil {
				slog.WarnContext(
					altshiftContext.WithError(
						ctx,
						altshiftErrors.NewWithTrace(fmt.Errorf("http response body close: %w", err)),
					),
					"An error occurred when closing the response body.",
				)
			}
		}()

		if err != nil {
			return response, nil, altshiftErrors.NewWithTraceCtx(
				ctxWithHttpContext,
				fmt.Errorf("io read all (response body): %w", err),
			)
		}

		httpContext.ResponseBody = responseBodyData
	}

	if responseTls := response.TLS; responseTls != nil {
		tlsContext := httpContext.TlsContext
		if tlsContext == nil {
			tlsContext = &altshiftTlsTypes.TlsContext{}
			httpContext.TlsContext = tlsContext
		}

		tlsContext.ConnectionState = responseTls
		tlsContext.ClientInitiated = true
	}

	if !fetchConfig.SkipErrorOnStatus {
		if !strings.HasPrefix(strconv.Itoa(response.StatusCode), "2") {
			return response, responseBodyData, altshiftErrors.NewWithTraceCtx(
				ctxWithHttpContext,
				&altshiftHttpErrors.Non2xxStatusCodeError{StatusCode: response.StatusCode},
			)
		}
	}

	return response, responseBodyData, nil
}

func fetchWithRetryConfig(
	ctx context.Context,
	request *http.Request,
	fetchConfig *fetch_config.Config,
) (*http.Response, []byte, error) {
	if request == nil {
		return nil, nil, nil
	}

	if fetchConfig == nil {
		return nil, nil, altshiftErrors.NewWithTrace(nil_error.New("fetch config"))
	}

	retryConfig := fetchConfig.RetryConfig
	if retryConfig == nil {
		return nil, nil, altshiftErrors.NewWithTrace(nil_error.New("fetch retry config"))
	}

	var err error
	var response *http.Response
	var responseBody []byte

	// TODO: Do something with http context and extra? (Or remove `extra`?)

	for i := range 1 + retryConfig.Count {
		if i != 0 {
			// Wait before the next attempt, based on the previous response.
			waitDuration, giveUp := retryWaitDuration(retryConfig, response, responseBody, i)
			if giveUp {
				break
			}

			// TODO: Add jitter?

			if waitDuration > 0 {
				time.Sleep(waitDuration)
			}

			// The previous attempt consumed the request body; replay it so the
			// retried request is not sent with a drained body and a stale
			// Content-Length. http.NewRequest populates GetBody for the buffer types
			// used here.
			if request.GetBody != nil {
				newBody, bodyErr := request.GetBody()
				if bodyErr != nil {
					return nil, nil, altshiftErrors.NewWithTrace(
						fmt.Errorf("request get body: %w", bodyErr),
					)
				}
				request.Body = newBody
			}
		}

		response, responseBody, err = fetch(ctx, request, fetchConfig)

		if !retryConfig.ResponseChecker.Check(response, err) {
			break
		}

		if err != nil && i != 0 {
			err = &altshiftHttpErrors.ReattemptFailedError{Cause: err, Attempt: i + 1}
		}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("fetch: %w", err)
	}

	return response, responseBody, nil
}

func FetchWithRequest(ctx context.Context, request *http.Request, options ...fetch_config.Option) (*http.Response, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("context err: %w", err)
	}

	if request == nil {
		return nil, nil, nil
	}

	fetchConfig := fetch_config.New(options...)
	if request.Header != nil {
		for key, value := range fetchConfig.Headers {
			request.Header.Set(key, value)
		}
	}

	var response *http.Response
	var responseBody []byte
	var err error
	var errString string

	if fetchConfig.RetryConfig != nil {
		response, responseBody, err = fetchWithRetryConfig(ctx, request, fetchConfig)
		errString = " with retry config"
	} else {
		response, responseBody, err = fetch(ctx, request, fetchConfig)
	}
	if err != nil {
		return response, responseBody, fmt.Errorf("fetch%s: %w", errString, err)
	}

	return response, responseBody, nil
}

func Fetch(ctx context.Context, url string, options ...fetch_config.Option) (*http.Response, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("context err: %w", err)
	}

	if url == "" {
		return nil, nil, altshiftErrors.NewWithTrace(empty_error.New("url"))
	}

	fetchConfig := fetch_config.New(options...)
	method := fetchConfig.Method

	request, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(fetchConfig.Body))
	if err != nil {
		return nil, nil, altshiftErrors.NewWithTrace(fmt.Errorf("http new request: %w", err), method)
	}
	if request == nil {
		return nil, nil, altshiftErrors.NewWithTrace(nil_error.New("request"))
	}

	requestHeader := request.Header
	if requestHeader == nil {
		return nil, nil, altshiftErrors.NewWithTrace(nil_error.New("request header"))
	}

	return FetchWithRequest(ctx, request, options...)
}

func FetchJson[U any](ctx context.Context, url string, options ...fetch_config.Option) (*http.Response, U, error) {
	var zero U

	if err := ctx.Err(); err != nil {
		return nil, zero, fmt.Errorf("context err: %w", err)
	}

	if url == "" {
		return nil, zero, altshiftErrors.NewWithTrace(empty_error.New("url"))
	}

	fetchConfig := fetch_config.New(options...)

	headers := maps.Clone(fetchConfig.Headers)
	if headers == nil {
		headers = make(map[string]string)
	}

	if _, ok := headers["Content-Type"]; !ok && len(fetchConfig.Body) > 0 {
		headers["Content-Type"] = "application/json"
	}

	if _, ok := headers["Accept"]; !ok {
		headers["Accept"] = "application/json"
	}

	options = append(options, fetch_config.WithHeaders(headers))
	response, responseBody, err := Fetch(ctx, url, options...)
	if err != nil {
		return response, zero, fmt.Errorf("fetch: %w", err)
	}
	if len(responseBody) == 0 {
		return response, zero, nil
	}

	var responseValue U
	if err = json.Unmarshal(responseBody, &responseValue); err != nil {
		return response, zero, altshiftErrors.NewWithTrace(
			fmt.Errorf("json unmarshal (response body): %w", err),
			responseBody,
		)
	}

	return response, responseValue, nil
}

func FetchJsonWithBody[U any, T any](ctx context.Context, url string, bodyValue T, options ...fetch_config.Option) (*http.Response, U, error) {
	var zero U

	if err := ctx.Err(); err != nil {
		return nil, zero, fmt.Errorf("context err: %w", err)
	}

	if url == "" {
		return nil, zero, altshiftErrors.NewWithTrace(empty_error.New("url"))
	}

	var requestBody []byte
	if !utils.IsNil(bodyValue) {
		var err error
		requestBody, err = json.Marshal(bodyValue)
		if err != nil {
			return nil, zero, altshiftErrors.NewWithTrace(
				fmt.Errorf("json marshal (body value): %w", err),
				bodyValue,
			)
		}

		options = append(options, fetch_config.WithBody(requestBody))
	}

	return FetchJson[U](ctx, url, options...)
}

// GetMatchingContentEncoding returns the server encoding to use for a client's
// accepted encodings. Client quality values are authoritative between quality
// groups; within a group of equal quality the server's preference order decides
// (callers list their identifiers by preference, e.g. smallest variant first).
func GetMatchingContentEncoding(
	clientSupportedEncodings []*altshiftHttpTypes.Encoding,
	serverSupportedEncodingIdentifiers []string,
) string {
	if len(clientSupportedEncodings) == 0 {
		return AcceptContentIdentity
	}

	encodings := make([]*altshiftHttpTypes.Encoding, 0, len(clientSupportedEncodings))
	for _, clientEncoding := range clientSupportedEncodings {
		if clientEncoding != nil {
			encodings = append(encodings, clientEncoding)
		}
	}
	sort.SliceStable(encodings, func(i, j int) bool {
		return encodings[i].QualityValue > encodings[j].QualityValue
	})

	disallowIdentity := false
	excludedCodings := make(map[string]struct{})
	explicitCodings := make(map[string]struct{})
	for _, clientEncoding := range encodings {
		coding := strings.ToLower(clientEncoding.Coding)
		if coding != "*" {
			explicitCodings[coding] = struct{}{}
		}
		if clientEncoding.QualityValue == 0 {
			switch coding {
			case "*", AcceptContentIdentity:
				disallowIdentity = true
			default:
				excludedCodings[coding] = struct{}{}
			}
		}
	}

	for groupStart := 0; groupStart < len(encodings); {
		qualityValue := encodings[groupStart].QualityValue
		groupEnd := groupStart
		for groupEnd < len(encodings) && encodings[groupEnd].QualityValue == qualityValue {
			groupEnd++
		}

		if qualityValue == 0 {
			break
		}

		groupCodings := make(map[string]struct{})
		groupWildcard := false
		for _, clientEncoding := range encodings[groupStart:groupEnd] {
			coding := strings.ToLower(clientEncoding.Coding)
			if coding == "*" {
				groupWildcard = true
			} else {
				groupCodings[coding] = struct{}{}
			}
		}

		for _, supportedEncoding := range serverSupportedEncodingIdentifiers {
			coding := strings.ToLower(supportedEncoding)
			if _, ok := groupCodings[coding]; ok {
				return supportedEncoding
			}
			if groupWildcard {
				_, explicit := explicitCodings[coding]
				_, excluded := excludedCodings[coding]
				if !explicit && !excluded {
					return supportedEncoding
				}
			}
		}

		if _, ok := groupCodings[AcceptContentIdentity]; ok {
			return AcceptContentIdentity
		}
		if groupWildcard && !disallowIdentity {
			if _, explicit := explicitCodings[AcceptContentIdentity]; !explicit {
				return AcceptContentIdentity
			}
		}

		groupStart = groupEnd
	}

	if !disallowIdentity {
		return AcceptContentIdentity
	}

	return ""
}

func GetMatchingAccept(
	clientSupportedMediaRanges []*altshiftHttpTypes.MediaRange,
	serverSupportedMediaRanges []*altshiftHttpTypes.ServerMediaRange,
) *altshiftHttpTypes.ServerMediaRange {
	if len(clientSupportedMediaRanges) == 0 || len(serverSupportedMediaRanges) == 0 {
		return nil
	}

	for _, clientMediaRange := range clientSupportedMediaRanges {
		if clientMediaRange == nil {
			continue
		}

		clientType := strings.ToLower(clientMediaRange.Type)
		clientSubtype := strings.ToLower(clientMediaRange.Subtype)

		for _, serverMediaRange := range serverSupportedMediaRanges {
			if serverMediaRange == nil {
				continue
			}

			if clientType == "*" && clientSubtype == "*" {
				return serverMediaRange
			}

			serverType := strings.ToLower(serverMediaRange.Type)
			serverSubtype := strings.ToLower(serverMediaRange.Subtype)

			if (clientType == "*" || clientType == serverType) && (clientSubtype == "*" || clientSubtype == serverSubtype) {
				return serverMediaRange
			}
		}
	}

	return nil
}

func ParseLastModifiedTimestamp(timestamp string) (time.Time, error) {
	t, err := time.Parse(time.RFC1123, timestamp)

	if err != nil {
		return time.Time{}, altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: time parse rfc1123: %w",
				altshiftHttpErrors.ErrBadIfModifiedSinceTimestamp,
				err,
			),
			timestamp,
		)
	}

	return t, nil
}

func IfNoneMatchCacheHit(ifNoneMatchValue string, etag string) bool {
	if ifNoneMatchValue == "" || etag == "" {
		return false
	}

	return ifNoneMatchValue == etag
}

func IfModifiedSinceCacheHit(ifModifiedSinceValue string, lastModifiedValue string) (bool, error) {
	if ifModifiedSinceValue == "" || lastModifiedValue == "" {
		return false, nil
	}

	ifModifiedSinceTimestamp, err := ParseLastModifiedTimestamp(ifModifiedSinceValue)
	if err != nil {
		return false, altshiftErrors.New(
			fmt.Errorf("parse last modified timestamp (If-Modified-Since): %w", err),
			ifModifiedSinceValue,
		)
	}

	lastModifiedTimestamp, err := ParseLastModifiedTimestamp(lastModifiedValue)
	if err != nil {
		return false, altshiftErrors.New(
			fmt.Errorf("parse last modified timestamp (Last-Modified): %w", err),
			lastModifiedValue,
		)
	}

	return ifModifiedSinceTimestamp.Equal(lastModifiedTimestamp) || lastModifiedTimestamp.Before(ifModifiedSinceTimestamp), nil
}

func MakeStrongEtag(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return fmt.Sprintf("\"%x\"", h.Sum(nil))
}

func BasicAuth(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

func GetSingleHeader(name string, header http.Header) (string, error) {
	if header == nil {
		return "", altshiftErrors.NewWithTrace(nil_error.New("map"))
	}

	name = http.CanonicalHeaderKey(name)

	headerValues, ok := header[name]
	if !ok {
		return "", altshiftErrors.NewWithTrace(fmt.Errorf("%w (%s)", altshiftHttpErrors.ErrMissingHeader, name))
	}
	if len(headerValues) != 1 {
		return "", altshiftErrors.NewWithTrace(fmt.Errorf("%w (%s)", altshiftHttpErrors.ErrMultipleHeaderValues, name))
	}

	return headerValues[0], nil
}
