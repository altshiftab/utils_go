package service

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	motmedelMux "github.com/altshiftab/utils_go/pkg/http/mux"
	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint/static_content"
	muxUtils "github.com/altshiftab/utils_go/pkg/http/mux/utils"
	motmedelHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	contentTypeParsing "github.com/altshiftab/utils_go/pkg/http/types/content_type"
	motmedelHttpTypesSitemapxml "github.com/altshiftab/utils_go/pkg/http/types/sitemapxml"
	motmedelHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

// sitemapContentTypes are the response content types whose endpoints are eligible for inclusion in
// the sitemap, i.e. the document types search engines crawl and index.
var sitemapContentTypes = map[string]struct{}{
	"text/html":             {},
	"application/xhtml+xml": {},
	"application/pdf":       {},
}

// apiPathPrefix is what a crawler invited by a sitemap is still kept out of: the endpoints a
// service answers programs with, including the ones browsers post their reports to, none of which
// is a document worth indexing.
const apiPathPrefix = "/api/"

func makeSitemapUrl(
	staticContentData *static_content.StaticContentData,
	location string,
) (*motmedelHttpTypesSitemapxml.Url, error) {
	if staticContentData == nil {
		return nil, nil
	}

	if location == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("location"))
	}

	var lastModified string
	var isDocument bool
	for _, header := range staticContentData.Headers {
		switch strings.ToLower(header.Name) {
		case "content-type":
			if contentType, err := contentTypeParsing.Parse([]byte(header.Value)); err == nil && contentType != nil {
				if _, found := sitemapContentTypes[contentType.GetFullType(true)]; found {
					isDocument = true
				}
			}
		case "last-modified":
			lastModified = header.Value
		}

		if isDocument && lastModified != "" {
			break
		}
	}

	if !isDocument {
		return nil, nil
	}

	var formattedLastModified string
	if lastModified != "" {
		parsedTime, err := time.Parse(time.RFC1123, lastModified)
		if err != nil {
			return nil, motmedelErrors.NewWithTrace(fmt.Errorf("time parse: %w", err), lastModified)
		}

		formattedLastModified = parsedTime.Format(time.RFC3339)
	}

	return &motmedelHttpTypesSitemapxml.Url{Loc: location, Lastmod: formattedLastModified}, nil
}

// patchSitemap adds a sitemap.xml listing the documents the mux serves statically, and returns
// where it is served. Nothing is added, and nothing returned, where the mux serves no documents --
// an empty sitemap says less than no sitemap.
func patchSitemap(mux *motmedelMux.Mux, baseUrl *url.URL) (string, error) {
	if mux == nil {
		return "", motmedelErrors.NewWithTrace(nil_error.New("mux"))
	}

	if baseUrl == nil {
		return "", motmedelErrors.NewWithTrace(nil_error.New("base url"))
	}

	var sitemapUrls []*motmedelHttpTypesSitemapxml.Url

	// Every endpoint is considered, not only the ones the mux calls documents -- it calls only
	// text/html one, whereas a crawler indexes the other document types in sitemapContentTypes
	// just as readily.
	for _, methodToEndpoint := range mux.EndpointMap {
		for method, endpoint := range methodToEndpoint {
			if endpoint == nil || method != http.MethodGet {
				continue
			}

			staticContent := endpoint.StaticContent
			if staticContent == nil {
				continue
			}

			pathUrl := baseUrl.JoinPath(endpoint.Path)
			if pathUrl == nil {
				return "", motmedelErrors.NewWithTrace(nil_error.New("path url"), endpoint.Path)
			}

			staticContentData := staticContent.StaticContentData
			location := pathUrl.String()

			sitemapUrl, err := makeSitemapUrl(&staticContentData, location)
			if err != nil {
				return "", motmedelErrors.New(
					fmt.Errorf("make sitemap url: %w", err),
					staticContentData, location,
				)
			}
			if sitemapUrl != nil {
				sitemapUrls = append(sitemapUrls, sitemapUrl)
			}
		}
	}

	if len(sitemapUrls) == 0 {
		return "", nil
	}

	// The endpoints are held in maps, which are walked in no particular order. Sorting makes the
	// sitemap, and with it the entity tag it is served with, the same from one start to the next.
	slices.SortFunc(sitemapUrls, func(a *motmedelHttpTypesSitemapxml.Url, b *motmedelHttpTypesSitemapxml.Url) int {
		return strings.Compare(a.Loc, b.Loc)
	})

	urlSet := motmedelHttpTypesSitemapxml.UrlSet{
		Xmlns: "https://www.sitemaps.org/schemas/sitemap/0.9",
		Urls:  sitemapUrls,
	}

	urlSetData, err := xml.Marshal(urlSet)
	if err != nil {
		return "", motmedelErrors.NewWithTrace(fmt.Errorf("xml marshal: %w", err), urlSet)
	}

	data := append([]byte(xml.Header), urlSetData...)
	etag := motmedelHttpUtils.MakeStrongEtag(data)
	lastModified := time.Now().UTC().Format(http.TimeFormat)

	sitemapUrl := baseUrl.JoinPath("/sitemap.xml")
	if sitemapUrl == nil {
		return "", motmedelErrors.NewWithTrace(nil_error.New("sitemap url"))
	}

	mux.Add(
		&endpointPkg.Endpoint{
			Path:   "/sitemap.xml",
			Method: http.MethodGet,
			StaticContent: &static_content.StaticContent{
				StaticContentData: static_content.StaticContentData{
					Data:         data,
					Etag:         etag,
					LastModified: lastModified,
					Headers: muxUtils.MakeStaticContentHeaders(
						"application/xml",
						"no-cache",
						etag,
						lastModified,
					),
				},
			},
			Public: true,
		},
	)

	return sitemapUrl.String(), nil
}

// patchRobotsTxt adds a robots.txt. Where sitemapUrl names a sitemap, every crawler is invited to
// everything but the API paths and pointed at it; otherwise every crawler is told to keep out,
// there being nothing meant for one to index.
func patchRobotsTxt(mux *motmedelMux.Mux, sitemapUrl string) error {
	if mux == nil {
		return motmedelErrors.NewWithTrace(nil_error.New("mux"))
	}

	// One group, for every crawler there is. Naming the ones that may crawl would leave out every
	// crawler that appears after the naming, which is a list that only ever grows stale.
	group := &motmedelHttpTypes.RobotsTxtGroup{UserAgents: []string{"*"}}

	if sitemapUrl != "" {
		group.Disallowed = []string{apiPathPrefix}
		group.OtherRecords = [][2]string{{"Sitemap", sitemapUrl}}
	} else {
		group.Disallowed = []string{"/"}
	}

	robotsTxtEndpoint := endpointPkg.NewRobotsTxt(
		&motmedelHttpTypes.RobotsTxt{Groups: []*motmedelHttpTypes.RobotsTxtGroup{group}},
	)
	if robotsTxtEndpoint == nil {
		return motmedelErrors.NewWithTrace(nil_error.New("robots txt endpoint"))
	}

	mux.Add(robotsTxtEndpoint)

	return nil
}
