package service

import (
	"net/http"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftMux "github.com/altshiftab/utils_go/pkg/http/mux"
	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint/static_content"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	muxUtils "github.com/altshiftab/utils_go/pkg/http/mux/utils"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

const (
	securityTxtPath          = "/security.txt"
	wellKnownSecurityTxtPath = "/.well-known/security.txt"
)

func redirectEndpoint(path string, location string) *endpointPkg.Endpoint {
	return &endpointPkg.Endpoint{
		Path:   path,
		Method: http.MethodGet,
		Handler: func(_ *http.Request, _ []byte) (*response.Response, *response_error.ResponseError) {
			return &response.Response{
				StatusCode: http.StatusPermanentRedirect,
				Headers:    []*response.HeaderEntry{{Name: "Location", Value: location}},
			}, nil
		},
		Public: true,
	}
}

// patchSecurityTxtUrl points both security.txt paths at one served elsewhere.
func patchSecurityTxtUrl(mux *altshiftMux.Mux, securityTxtUrl string) error {
	if mux == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("mux"))
	}

	if securityTxtUrl == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("security txt url"))
	}

	mux.Add(
		redirectEndpoint(securityTxtPath, securityTxtUrl),
		redirectEndpoint(wellKnownSecurityTxtPath, securityTxtUrl),
	)

	return nil
}

// patchSecurityTxt serves the security.txt at the well-known path RFC 9116 gives it, with the
// path it had before the RFC redirecting there.
func patchSecurityTxt(mux *altshiftMux.Mux, securityTxt *altshiftHttpTypes.SecurityTxt) error {
	if mux == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("mux"))
	}

	if securityTxt == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("security txt"))
	}

	securityTxtString := securityTxt.String()
	if securityTxtString == "" {
		// A security.txt renders as empty only when it names no contact, which is what it exists to
		// say. Serving it would tell a reporter less than serving nothing.
		return altshiftErrors.NewWithTrace(empty_error.NewWithInstance("contacts", "security txt"))
	}

	data := []byte(securityTxtString)
	etag := altshiftHttpUtils.MakeStrongEtag(data)
	lastModified := time.Now().UTC().Format(http.TimeFormat)

	mux.Add(
		redirectEndpoint(securityTxtPath, wellKnownSecurityTxtPath),
		&endpointPkg.Endpoint{
			Path:   wellKnownSecurityTxtPath,
			Method: http.MethodGet,
			StaticContent: &static_content.StaticContent{
				StaticContentData: static_content.StaticContentData{
					Data:         data,
					Etag:         etag,
					LastModified: lastModified,
					Headers: muxUtils.MakeStaticContentHeaders(
						"text/plain; charset=utf-8",
						"no-cache",
						etag,
						lastModified,
					),
				},
			},
			Public: true,
		},
	)

	return nil
}
