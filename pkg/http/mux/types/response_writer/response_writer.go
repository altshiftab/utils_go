package response_writer

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	motmedelContext "github.com/altshiftab/utils_go/pkg/context"
	motmedelGzip "github.com/altshiftab/utils_go/pkg/encoding/gzip"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	muxErrors "github.com/altshiftab/utils_go/pkg/http/mux/errors"
	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	motmedelHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/content_type"
	motmedelHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

const (
	cacheControlHeaderName = "Cache-Control"
	sameOrigin             = "same-origin"
)

var DefaultHeaders = map[string]string{
	cacheControlHeaderName:         "no-store",
	"X-Content-Type-Options":       "nosniff",
	"Cross-Origin-Resource-Policy": sameOrigin,
}

const (
	// object-src 'none' — OWASP ASVS 5.0.0, V3.4.3 (L2): "Verify that the application's Content Security Policy
	// includes object-src 'none' to prevent plugin-based code execution vulnerabilities."
	//
	// upgrade-insecure-requests — transparently rewrites insecure (http) subresource requests to https.
	//
	// TODO: Consider adding back webrtc 'block' in the future — RTCPeerConnection bypasses fetch directives
	// (connect-src/default-src) and can be abused for data exfiltration; blocking it closes a channel no other
	// directive covers.
	DefaultContentSecurityPolicyString = "default-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'; upgrade-insecure-requests"
)

var DefaultDocumentHeaders = map[string]string{
	"Cross-Origin-Opener-Policy":   sameOrigin,
	"Cross-Origin-Embedder-Policy": "require-corp",
	"Content-Security-Policy":      DefaultContentSecurityPolicyString,
	"Permissions-Policy":           "geolocation=(), microphone=(), camera=(), payment=(), usb=(), display-capture=()",
	"Referrer-Policy":              sameOrigin,
}

type ResponseWriter struct {
	http.ResponseWriter
	IsHeadRequest      bool
	WriteHeaderCalled  bool
	WriteCalled        bool
	NoStoreWrittenBody bool

	WrittenStatusCode int
	WrittenBody       []byte

	DefaultHeaders         map[string]string
	DefaultDocumentHeaders map[string]string
}

func (responseWriter *ResponseWriter) WriteHeader(statusCode int) {
	responseWriter.WriteHeaderCalled = true
	responseWriter.WrittenStatusCode = statusCode
	responseWriter.ResponseWriter.WriteHeader(statusCode)
}

func (responseWriter *ResponseWriter) Write(data []byte) (int, error) {
	responseWriter.WriteCalled = true

	if !responseWriter.WriteHeaderCalled {
		statusCode := http.StatusOK
		if len(data) == 0 {
			statusCode = http.StatusNoContent
		}
		responseWriter.WriteHeader(statusCode)
	}

	if responseWriter.IsHeadRequest || len(data) == 0 {
		return 0, nil
	}

	if !responseWriter.NoStoreWrittenBody {
		responseWriter.WrittenBody = append(responseWriter.WrittenBody, data...)
	}

	n, err := responseWriter.ResponseWriter.Write(data)
	if err != nil {
		return n, motmedelErrors.NewWithTrace(fmt.Errorf("http response writer write: %w", err))
	}

	return n, nil
}

func (responseWriter *ResponseWriter) WriteResponse(
	ctx context.Context,
	response *muxTypesResponse.Response,
	acceptEncoding *motmedelHttpTypes.AcceptEncoding,
) error {
	if response == nil {
		return nil
	}

	var defaultHeaders map[string]string
	if responseWriterDefaultHeaders := responseWriter.DefaultHeaders; responseWriterDefaultHeaders == nil {
		defaultHeaders = DefaultHeaders
	} else {
		defaultHeaders = responseWriterDefaultHeaders
	}

	var defaultDocumentHeaders map[string]string
	if responseWriterDefaultDocumentHeaders := responseWriter.DefaultDocumentHeaders; responseWriterDefaultDocumentHeaders == nil {
		defaultDocumentHeaders = DefaultDocumentHeaders
	} else {
		defaultDocumentHeaders = responseWriterDefaultDocumentHeaders
	}

	skippedDefaultHeadersSet := make(map[string]struct{})

	body := response.Body
	bodyStreamer := response.BodyStreamer

	var contentTypeString *string
	var contentEncodingString *string

	cacheControlSet := make(map[string]struct{})
	var varyValues []string

	responseWriterHeader := responseWriter.Header()
	for _, header := range response.Headers {
		if header == nil || header.Name == "" {
			continue
		}

		canonicalHeaderName := http.CanonicalHeaderKey(header.Name)
		headerValue := header.Value

		if canonicalHeaderName == "Content-Type" {
			contentTypeString = &headerValue
			if len(body) == 0 && bodyStreamer == nil {
				continue
			}
		}

		if canonicalHeaderName == "Content-Encoding" {
			contentEncodingString = &headerValue
			if len(body) == 0 && bodyStreamer == nil {
				continue
			}
		}

		if canonicalHeaderName == cacheControlHeaderName {
			for _, cacheControlValue := range strings.Split(headerValue, ",") {
				cacheControlSet[strings.ToLower(strings.TrimSpace(cacheControlValue))] = struct{}{}
			}
		}

		// NOTE: By using `Overwrite` with an empty value, one effectively clears a default header without providing
		// a new value.

		if _, ok := defaultHeaders[canonicalHeaderName]; ok {
			if !header.Overwrite {
				continue
			}
			skippedDefaultHeadersSet[canonicalHeaderName] = struct{}{}
		}

		if _, ok := defaultDocumentHeaders[canonicalHeaderName]; ok {
			if !header.Overwrite {
				continue
			}
			skippedDefaultHeadersSet[canonicalHeaderName] = struct{}{}
		}

		if canonicalHeaderName == "Vary" {
			for _, varyValue := range strings.Split(headerValue, ",") {
				varyValues = append(varyValues, strings.TrimSpace(varyValue))
			}
			continue
		}

		responseWriterHeader.Add(canonicalHeaderName, headerValue)
	}
	for headerName, headerValue := range defaultHeaders {
		canonicalHeaderName := http.CanonicalHeaderKey(headerName)

		if _, ok := skippedDefaultHeadersSet[canonicalHeaderName]; ok {
			continue
		}

		if canonicalHeaderName == cacheControlHeaderName {
			for _, cacheControlValue := range strings.Split(headerValue, ",") {
				cacheControlSet[strings.ToLower(strings.TrimSpace(cacheControlValue))] = struct{}{}
			}
		}

		if canonicalHeaderName == "Vary" {
			for _, varyValue := range strings.Split(headerValue, ",") {
				varyValues = append(varyValues, strings.TrimSpace(varyValue))
			}
			continue
		}

		if headerValue == "" {
			continue
		}

		responseWriterHeader.Add(canonicalHeaderName, headerValue)
	}

	if contentTypeString != nil {
		contentTypeData := []byte(*contentTypeString)
		contentType, err := content_type.Parse(contentTypeData)
		if err != nil {
			return motmedelErrors.New(fmt.Errorf("parse content type: %w", err), contentTypeData)
		}
		if contentType == nil {
			return motmedelErrors.NewWithTrace(nil_error.New("content type"), contentTypeData)
		}

		var useDocumentHeaders bool

		effectiveContentTypeValues := []string{
			strings.ToLower(contentType.Subtype),
			contentType.GetStructuredSyntaxName(true),
		}
		for _, effectiveContentTypeValue := range effectiveContentTypeValues {
			switch effectiveContentTypeValue {
			case "html", "xhtml", "xml", "svg":
				useDocumentHeaders = true
			}
		}

		if useDocumentHeaders {
			for headerName, headerValue := range defaultDocumentHeaders {
				canonicalHeaderName := http.CanonicalHeaderKey(headerName)

				if _, ok := skippedDefaultHeadersSet[canonicalHeaderName]; ok {
					continue
				}

				// What is said about a document replaces what is said about a response in general,
				// rather than being said alongside it. A content security policy is the case that
				// matters: a browser enforces every policy it is sent, so a document carrying both
				// would be held to what the two permit between them -- which, where the general one
				// permits nothing, is nothing.
				responseWriterHeader.Set(canonicalHeaderName, headerValue)
			}
		}
	}

	// The effective policy is settled at this point, whether it came from a
	// response header (e.g. one patched by the service) or the defaults.
	if inlineScriptHashes := response.InlineScriptHashes; len(inlineScriptHashes) != 0 {
		if policyString := responseWriterHeader.Get(contentSecurityPolicyHeaderName); policyString != "" {
			mergedPolicyString, err := applyInlineScriptHashes(policyString, inlineScriptHashes)
			if err != nil {
				return fmt.Errorf("apply inline script hashes: %w", err)
			}
			responseWriterHeader.Set(contentSecurityPolicyHeaderName, mergedPolicyString)
		}
	}

	_, noStore := cacheControlSet["no-store"]

	if !noStore && len(varyValues) > 0 {
		responseWriterHeader.Add("Vary", strings.Join(varyValues, ", "))
	}

	// Try to compress the body if it is of a decent size, and
	shouldTryToCompressBody := len(body) > 1000 &&
		// ... no content encoding is applied
		contentEncodingString == nil &&
		// ... the client indicates that it supports encoded content
		acceptEncoding != nil &&
		// ... the response body is not sensitive (compressing could theoretically enable attacks)
		!response.SensitiveBody &&
		// ... the response concerns a non-static resource (static resources should provide encoded values explicitly,
		// and I don't want to add a `Vary` header like this)
		noStore

	if shouldTryToCompressBody {
		// NOTE: The case where `identify` effectively has a quality value of 0 should be handled elsewhere.
		switch motmedelHttpUtils.GetMatchingContentEncoding(acceptEncoding.GetPriorityOrderedEncodings(), []string{"gzip"}) {
		case "gzip":
			gzipBody, err := motmedelGzip.MakeGzipData(ctx, body)
			if err != nil {
				slog.WarnContext(
					motmedelContext.WithError(
						ctx,
						motmedelErrors.New(fmt.Errorf("make gzip data: %w", err), body),
					),
					"An error occurred when making Gzip data.",
				)
			}

			if len(gzipBody) < len(body) {
				body = gzipBody
				responseWriterHeader.Set("Content-Encoding", "gzip")
			}
		}
	}

	if response.StatusCode != 0 {
		responseWriter.WriteHeader(response.StatusCode)
	}

	if bodyStreamer != nil {
		flusher, ok := responseWriter.ResponseWriter.(http.Flusher)
		if !ok {
			return muxErrors.ErrNoResponseWriterFlusher
		}

		if _, ok := responseWriterHeader["Transfer-Encoding"]; ok {
			return muxErrors.ErrTransferEncodingAlreadySet
		}

		// TODO: Figure out how to support HTTP/2?
		responseWriterHeader.Set("Transfer-Encoding", "chunked")

		for bodyChunk, err := range bodyStreamer {
			if err != nil {
				return fmt.Errorf("body streamer: %w", err)
			}

			if _, err := responseWriter.Write(bodyChunk); err != nil {
				return fmt.Errorf("mux response writer write: %w", err)
			}
			flusher.Flush()
		}

		if _, err := responseWriter.Write([]byte{}); err != nil {
			return fmt.Errorf("mux response writer write (empty chunk): %w", err)
		}
	} else {
		if _, err := responseWriter.Write(body); err != nil {
			return fmt.Errorf("mux response writer write: %w", err)
		}
	}

	return nil
}
