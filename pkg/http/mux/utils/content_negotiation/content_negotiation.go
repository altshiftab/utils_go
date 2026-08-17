package content_negotiation

import (
	"fmt"
	"net/http"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	altshiftHttpHeadersParsingAccept "github.com/altshiftab/utils_go/pkg/http/types/accept"
	altshiftHttpHeadersParsingAcceptEncoding "github.com/altshiftab/utils_go/pkg/http/types/accept_encoding"
	altshiftHttpHeadersParsingAcceptLanguage "github.com/altshiftab/utils_go/pkg/http/types/accept_language"
)

// TODO: Log warnings in non-strict cases?
// TODO: Maybe this should be a request parser instead. `strict` and `logErrors` can be initialized.

func GetContentNegotiation(requestHeader http.Header, strict bool) (*altshiftHttpTypes.ContentNegotiation, error) {
	if len(requestHeader) == 0 {
		return nil, nil
	}

	var contentNegotiation altshiftHttpTypes.ContentNegotiation

	if acceptValue := requestHeader.Get("Accept"); acceptValue != "" {
		acceptData := []byte(acceptValue)
		accept, err := altshiftHttpHeadersParsingAccept.Parse(acceptData)
		if err != nil && strict {
			return nil, altshiftErrors.New(fmt.Errorf("parse accept: %w", err), acceptData)
		}
		contentNegotiation.Accept = accept
	}

	if acceptEncodingValue := requestHeader.Get("Accept-Encoding"); acceptEncodingValue != "" {
		acceptEncodingData := []byte(acceptEncodingValue)
		acceptEncoding, err := altshiftHttpHeadersParsingAcceptEncoding.Parse(acceptEncodingData)
		if err != nil && strict {
			return nil, altshiftErrors.New(fmt.Errorf("parse accept encoding: %w", err), acceptEncodingData)
		}
		contentNegotiation.AcceptEncoding = acceptEncoding
	}

	if acceptLanguageValue := requestHeader.Get("Accept-Language"); acceptLanguageValue != "" {
		acceptLanguageData := []byte(acceptLanguageValue)
		acceptLanguage, err := altshiftHttpHeadersParsingAcceptLanguage.Parse(acceptLanguageData)
		if err != nil && strict {
			return nil, altshiftErrors.New(fmt.Errorf("parse accept language: %w", err), acceptLanguageData)
		}
		contentNegotiation.AcceptLanguage = acceptLanguage
	}

	return &contentNegotiation, nil
}
