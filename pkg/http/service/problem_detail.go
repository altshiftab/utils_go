package service

import (
	"fmt"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftMux "github.com/altshiftab/utils_go/pkg/http/mux"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
)

const (
	problemXmlContentType = "application/problem+xml"
	xmlContentType        = "application/xml"
)

// patchRenderableProblemDetails answers an XML problem detail as application/xml rather than as the
// application/problem+xml RFC 9457 gives it.
//
// The type is what a browser goes by, and it renders only the XML media types it knows: told
// application/problem+xml, Chrome downloads the response as a file instead of showing it (measured
// on Chrome 151, at both 200 and 403, and the same for any other unrecognised +xml type). An error
// a person is looking at is worth more rendered than saved to disk.
//
// What it costs is on the wire: a client that asked for application/problem+xml is answered with a
// laxer type than it asked for, and can no longer tell from the type alone that the body is a
// problem detail. The body is unchanged either way.
func patchRenderableProblemDetails(mux *altshiftMux.Mux) error {
	if mux == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("mux"))
	}

	mux.ProblemDetailConverter = response_error.ProblemDetailConverterFunction(
		func(detail *problem_detail.Detail, negotiation *altshiftHttpTypes.ContentNegotiation) ([]byte, string, error) {
			data, contentType, err := response_error.ConvertProblemDetail(detail, negotiation)
			if err != nil {
				return nil, "", fmt.Errorf("convert problem detail: %w", err)
			}

			if contentType == problemXmlContentType {
				contentType = xmlContentType
			}

			return data, contentType, nil
		},
	)

	return nil
}
