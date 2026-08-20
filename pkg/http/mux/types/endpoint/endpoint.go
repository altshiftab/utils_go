package endpoint

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	altshiftBrotli "github.com/altshiftab/utils_go/pkg/brotli"
	"github.com/altshiftab/utils_go/pkg/encoding/gzip"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	muxErrors "github.com/altshiftab/utils_go/pkg/http/mux/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_loader"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint/static_content"
	"github.com/altshiftab/utils_go/pkg/http/types/cache_control"
	muxTypesRateLimiting "github.com/altshiftab/utils_go/pkg/http/mux/types/rate_limiting"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser"
	muxResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	muxResponseError "github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/mux/utils"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
	"github.com/altshiftab/utils_go/pkg/sync/errgroup"
)

type Hint struct {
	InputType         reflect.Type
	UrlInputType      reflect.Type
	OutputType        reflect.Type
	UrlOutputType     reflect.Type
	OutputContentType string
	OutputOptional    bool
}

type Handler = func(*http.Request, []byte) (*muxResponse.Response, *muxResponseError.ResponseError)

type Endpoint struct {
	Path                      string
	Method                    string
	RateLimitingConfiguration *muxTypesRateLimiting.RateLimitingConfiguration
	AuthenticationParser      request_parser.RequestParser[any]
	UrlParser                 request_parser.RequestParser[any]
	HeaderParser              request_parser.RequestParser[any]
	BodyLoader                *body_loader.Loader
	CorsParser                request_parser.RequestParser[*altshiftHttpTypes.CorsConfiguration]
	DisableFetchMetadata      bool
	Public                    bool
	Hint                      *Hint
	Handler                   Handler
	StaticContent             *static_content.StaticContent
}

// Duplicate returns the endpoint as it would be served at each of the paths, for a response that
// answers for more than the path it was written at: a document that a frontend routes on its own is
// served at each of the routes it routes, so that a request for one arrives at the document that
// routes it rather than at a "not found".
//
// What the endpoint holds is shared with the duplicates rather than copied -- the static content
// among it, which is what makes duplicating a document cost nothing.
func Duplicate(endpoint *Endpoint, paths ...string) []*Endpoint {
	if endpoint == nil || len(paths) == 0 {
		return nil
	}

	duplicates := make([]*Endpoint, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}

		duplicate := *endpoint
		duplicate.Path = path

		duplicates = append(duplicates, &duplicate)
	}

	return duplicates
}

const robotsTxtCacheControl = "public, max-age=86400"

const htmlExtension = ".html"

func NewRobotsTxt(robotsTxt *altshiftHttpTypes.RobotsTxt) *Endpoint {
	if robotsTxt == nil {
		return nil
	}

	robotsTxtString := robotsTxt.String()
	if robotsTxtString == "" {
		return nil
	}

	data := []byte(robotsTxtString)
	etag := altshiftHttpUtils.MakeStrongEtag(data)
	lastModified := time.Now().UTC().Format(http.TimeFormat)

	return &Endpoint{
		Path:   "/robots.txt",
		Method: http.MethodGet,
		Public: true,
		StaticContent: &static_content.StaticContent{
			StaticContentData: static_content.StaticContentData{
				Data:         data,
				Etag:         etag,
				LastModified: lastModified,
				Headers:      utils.MakeStaticContentHeaders("text/plain", robotsTxtCacheControl, etag, lastModified),
			},
		},
	}
}

var supportedContentEncodings = []string{"gzip", "br"}

type StaticContentParameter struct {
	ContentType             string
	CacheControl            string
	CandidateForCompression bool
}

func (parameter *StaticContentParameter) HeaderEntries(etag string, lastModified string) []*muxResponse.HeaderEntry {
	return utils.MakeStaticContentHeaders(parameter.ContentType, parameter.CacheControl, etag, lastModified)
}

func AddContentEncodingData(staticContent *static_content.StaticContent) error {
	if staticContent == nil {
		return nil
	}

	data := staticContent.Data
	if len(data) == 0 {
		return nil
	}

	contentEncodingToData := make(map[string]*static_content.StaticContentData)
	var contentEncodingToDataLock sync.Mutex

	errGroup, errGroupCtx := errgroup.WithContext(context.Background())

loop:
	for _, contentEncoding := range supportedContentEncodings {
		select {
		case <-errGroupCtx.Done():
			break loop
		default:
			errGroup.Go(
				func() error {
					var encodedData []byte

					switch contentEncoding {
					case "gzip":
						gzipData, err := gzip.MakeGzipData(context.Background(), data)
						if err != nil {
							return fmt.Errorf("make gzip data: %w", err)
						}
						encodedData = gzipData
					case "br":
						brotliData, err := altshiftBrotli.MakeBrotliData(context.Background(), data)
						if err != nil {
							return fmt.Errorf("make brotli data: %w", err)
						}
						encodedData = brotliData
					default:
						return altshiftErrors.NewWithTrace(
							fmt.Errorf("%w: %s", muxErrors.ErrUnexpectedContentEncoding, contentEncoding),
							contentEncoding,
						)
					}

					if len(encodedData) >= len(data) {
						return nil
					}

					etag := altshiftHttpUtils.MakeStrongEtag(encodedData)

					headers := []*muxResponse.HeaderEntry{
						{Name: "Content-Encoding", Value: contentEncoding},
						{Name: "ETag", Value: etag},
					}

					for _, headerEntry := range staticContent.Headers {
						switch strings.ToLower(headerEntry.Name) {
						case "content-type", "cache-control", "last-modified":
							headers = append(
								headers,
								&muxResponse.HeaderEntry{
									Name:      headerEntry.Name,
									Value:     headerEntry.Value,
									Overwrite: headerEntry.Overwrite,
								},
							)
						}
					}

					contentEncodingToDataLock.Lock()
					defer contentEncodingToDataLock.Unlock()
					contentEncodingToData[contentEncoding] = &static_content.StaticContentData{
						Data:         encodedData,
						Etag:         etag,
						LastModified: staticContent.LastModified,
						Headers:      headers,
					}

					return nil
				},
			)
		}
	}

	if err := errGroup.Wait(); err != nil {
		return fmt.Errorf("errgroup wait: %w", err)
	}

	if len(contentEncodingToData) != 0 {
		staticContent.ContentEncodingToData = contentEncodingToData
		staticContent.Headers = append(
			staticContent.Headers,
			&muxResponse.HeaderEntry{Name: "Vary", Value: "Accept-Encoding"},
		)
	}

	return nil
}

var extensionToParameter = map[string]*StaticContentParameter{
	htmlExtension: {ContentType: "text/html", CacheControl: "no-cache", CandidateForCompression: true},
	".css":        {ContentType: "text/css", CandidateForCompression: true},
	".js":         {ContentType: "text/javascript", CandidateForCompression: true},
	".mjs":        {ContentType: "text/javascript", CandidateForCompression: true},
	".map":        {ContentType: "application/json", CandidateForCompression: true},
	".svg":        {ContentType: "image/svg+xml", CandidateForCompression: true},
	".webp":       {ContentType: "image/webp"},
	".avif":       {ContentType: "image/avif"},
	".woff2":      {ContentType: "font/woff2"},
	".txt":        {ContentType: "text/plain", CandidateForCompression: true},
	".xml":        {ContentType: "text/xml", CandidateForCompression: true},
	".pdf":        {ContentType: "application/pdf", CandidateForCompression: true},
	// NOTE: Modern image formats should be used instead.
	".png":  {ContentType: "image/png"},
	".jpg":  {ContentType: "image/jpeg"},
	".jpeg": {ContentType: "image/jpeg"},
}


// setStaticContentVisibility rewrites the Cache-Control of one body so that it
// says what `public` says. The header is parsed and written back rather than
// edited as a string: `private` may carry quoted field names, and a header is
// not a thing to take a substring of.
//
// A body whose Cache-Control does not parse is left alone. It is not this
// function's business to reject a header it was handed, and refusing to serve
// over it would turn a cache hint into an outage.
func setStaticContentVisibility(data *static_content.StaticContentData, public bool) {
	if data == nil {
		return
	}

	for _, header := range data.Headers {
		if header == nil || !strings.EqualFold(header.Name, "Cache-Control") {
			continue
		}

		parsed, err := cache_control.Parse([]byte(header.Value))
		if err != nil || parsed == nil {
			continue
		}

		parsed.SetVisibility(public)
		header.Value = parsed.String()
	}
}

// SetPublic says whether the endpoint may be reached without a session, and
// brings its static content's Cache-Control with it.
//
// The two are set together at construction and can only be set together
// afterwards. A service that gates an endpoint it generated as public -- which
// is how a mixed set of endpoints is built, some reachable and some not -- would
// otherwise leave the body still announcing itself to every shared cache on the
// way to the reader.
//
// Every content encoding is covered. Each carries its own copy of the headers,
// so changing only the one the caller happens to think of leaves the compressed
// bodies saying what the identity body no longer does.
func (endpoint *Endpoint) SetPublic(public bool) {
	if endpoint == nil {
		return
	}

	endpoint.Public = public

	staticContent := endpoint.StaticContent
	if staticContent == nil {
		return
	}

	setStaticContentVisibility(&staticContent.StaticContentData, public)
	for _, encoded := range staticContent.ContentEncodingToData {
		setStaticContentVisibility(encoded, public)
	}
}

// TODO: Use config.

func NewFromDataPath(
	path string,
	data []byte,
	lastModified string,
	addContentEncodingData bool,
	private bool,
) (*Endpoint, error) {
	if path == "" {
		return nil, nil
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	extension := strings.ToLower(filepath.Ext(path))

	parameter, ok := extensionToParameter[extension]
	if !ok {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %s", muxErrors.ErrUnsupportedFileExtension, extension),
			extension,
		)
	}
	if parameter == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("header parameter"))
	}
	if parameter.ContentType == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("content type"))
	}

	if extension == htmlExtension {
		path = strings.TrimSuffix(path, ".html")
	}

	if path == "/index" {
		path = "/"
	}

	etag := altshiftHttpUtils.MakeStrongEtag(data)

	var visibility string
	if private {
		visibility = "private"
	} else {
		visibility = "public"
	}

	// NOTE: parameter is a shared entry in extensionToParameter, so the effective Cache-Control
	// is derived locally rather than mutated in place (which would race across concurrent calls
	// and leak visibility between calls).
	cacheControl := parameter.CacheControl
	if cacheControl == "" {
		cacheControl = strings.Join(
			[]string{visibility, "max-age=31356000", "immutable"},
			", ",
		)
	}

	staticContent := &static_content.StaticContent{
		StaticContentData: static_content.StaticContentData{
			Data:         data,
			Etag:         etag,
			LastModified: lastModified,
			Headers:      utils.MakeStaticContentHeaders(parameter.ContentType, cacheControl, etag, lastModified),
		},
	}

	if extension == htmlExtension {
		staticContent.InlineScriptHashes = makeInlineScriptHashes(data)
	}

	if addContentEncodingData && parameter.CandidateForCompression && len(data) > 1000 {
		if err := AddContentEncodingData(staticContent); err != nil {
			return nil, altshiftErrors.New(
				fmt.Errorf("add content encoding data: %w", err),
				staticContent,
			)
		}
	}

	return &Endpoint{
		Path:          path,
		Method:        http.MethodGet,
		StaticContent: staticContent,
		Public:        !private,
	}, nil
}

// TODO: Use config.

func NewFromDirectory(rootPath string, addContentEncodingData bool, private bool) ([]*Endpoint, error) {
	if rootPath == "" {
		return nil, nil
	}

	if !strings.HasSuffix(rootPath, "/") {
		rootPath += "/"
	}

	var specifications []*Endpoint
	var specificationsMutex sync.Mutex

	errGroup, errGroupCtx := errgroup.WithContext(context.Background())

	err := filepath.Walk(
		rootPath,
		func(path string, fileInfo os.FileInfo, err error) error {
			if err != nil {
				return altshiftErrors.NewWithTrace(fmt.Errorf("filepath walk func: %w", err), path)
			}

			if fileInfo.IsDir() {
				return nil
			}

			select {
			case <-errGroupCtx.Done():
				return nil
			default:
				errGroup.Go(
					func() error { //nolint:contextcheck // NewFromDataPath's API deliberately takes no context; errGroupCtx is only used for cancellation checks here.
						data, err := os.ReadFile(path)
						if err != nil {
							return altshiftErrors.NewWithTrace(fmt.Errorf("read file: %w", err), path)
						}

						suggestedEndpointPath := "/" + strings.TrimPrefix(path, rootPath)
						lastModified := fileInfo.ModTime().UTC().Format(http.TimeFormat)

						specification, err := NewFromDataPath(
							suggestedEndpointPath,
							data,
							lastModified,
							addContentEncodingData,
							private,
						)
						if err != nil {
							return altshiftErrors.New(
								fmt.Errorf("endpoint specification from data path: %w", err),
								suggestedEndpointPath,
								data,
								lastModified,
							)
						}

						specificationsMutex.Lock()
						defer specificationsMutex.Unlock()
						specifications = append(specifications, specification)

						return nil
					},
				)
			}

			return nil
		},
	)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("filepath walk: %w", err), rootPath)
	}

	if err := errGroup.Wait(); err != nil {
		return nil, fmt.Errorf("errgroup wait: %w", err)
	}

	slices.SortFunc(specifications, compareEndpoints)

	return specifications, nil
}

func NewFromZip(reader *zip.Reader, addContentEncodingData bool, private bool) ([]*Endpoint, error) {
	if reader == nil {
		return nil, nil
	}

	var specifications []*Endpoint
	var specificationsMutex sync.Mutex

	errGroup, errGroupCtx := errgroup.WithContext(context.Background())

fileLoop:
	for _, file := range reader.File {
		select {
		case <-errGroupCtx.Done():
			break fileLoop
		default:

			if file.FileInfo().IsDir() {
				continue
			}

			errGroup.Go(
				func() error {
					fileReader, err := file.Open()
					if err != nil {
						return altshiftErrors.NewWithTrace(fmt.Errorf("zip file open: %w", err), file)
					}

					data, err := io.ReadAll(fileReader)
					if err := fileReader.Close(); err != nil {
						return altshiftErrors.NewWithTrace(fmt.Errorf("zip file reader close: %w", err), fileReader)
					}
					if err != nil {
						return altshiftErrors.NewWithTrace(fmt.Errorf("io read all (zip file reader): %w", err), fileReader)
					}

					path := file.Name
					lastModified := file.FileInfo().ModTime().UTC().Format(http.TimeFormat)

					specification, err := NewFromDataPath(
						path,
						data,
						lastModified,
						addContentEncodingData,
						private,
					)
					if err != nil {
						return altshiftErrors.New(
							fmt.Errorf("endpoint specification from data path: %w", err),
							path,
							data,
							lastModified,
						)
					}

					specificationsMutex.Lock()
					defer specificationsMutex.Unlock()
					specifications = append(specifications, specification)

					return nil
				},
			)
		}
	}

	if err := errGroup.Wait(); err != nil {
		return nil, fmt.Errorf("errgroup wait: %w", err)
	}

	slices.SortFunc(specifications, compareEndpoints)

	return specifications, nil
}

func compareEndpoints(a *Endpoint, b *Endpoint) int {
	if a == nil {
		if b == nil {
			return 0
		}
		return -1
	}
	if b == nil {
		return 1
	}

	if pathComparison := strings.Compare(a.Path, b.Path); pathComparison != 0 {
		return pathComparison
	}

	return strings.Compare(a.Method, b.Method)
}
