package content_negotiation

import (
	"fmt"
	"net/http"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	motmedelHttpHeadersParsingAccept "github.com/altshiftab/utils_go/pkg/http/types/accept"
	motmedelHttpHeadersParsingAcceptEncoding "github.com/altshiftab/utils_go/pkg/http/types/accept_encoding"
	motmedelHttpHeadersParsingAcceptLanguage "github.com/altshiftab/utils_go/pkg/http/types/accept_language"
)

// TODO: Log warnings in non-strict cases?
// TODO: Maybe this should be a request parser instead. `strict` and `logErrors` can be initialized.

func GetContentNegotiation(requestHeader http.Header, strict bool) (*motmedelHttpTypes.ContentNegotiation, error) {
	if len(requestHeader) == 0 {
		return nil, nil
	}

	var contentNegotiation motmedelHttpTypes.ContentNegotiation

	if acceptValue := requestHeader.Get("Accept"); acceptValue != "" {
		acceptData := []byte(acceptValue)
		accept, err := motmedelHttpHeadersParsingAccept.Parse(acceptData)
		if err != nil && strict {
			return nil, motmedelErrors.New(fmt.Errorf("parse accept: %w", err), acceptData)
		}
		contentNegotiation.Accept = accept
	}

	if acceptEncodingValue := requestHeader.Get("Accept-Encoding"); acceptEncodingValue != "" {
		acceptEncodingData := []byte(acceptEncodingValue)
		acceptEncoding, err := motmedelHttpHeadersParsingAcceptEncoding.Parse(acceptEncodingData)
		if err != nil && strict {
			return nil, motmedelErrors.New(fmt.Errorf("parse accept encoding: %w", err), acceptEncodingData)
		}
		contentNegotiation.AcceptEncoding = acceptEncoding
	}

	if acceptLanguageValue := requestHeader.Get("Accept-Language"); acceptLanguageValue != "" {
		acceptLanguageData := []byte(acceptLanguageValue)
		acceptLanguage, err := motmedelHttpHeadersParsingAcceptLanguage.Parse(acceptLanguageData)
		if err != nil && strict {
			return nil, motmedelErrors.New(fmt.Errorf("parse accept language: %w", err), acceptLanguageData)
		}
		contentNegotiation.AcceptLanguage = acceptLanguage
	}

	return &contentNegotiation, nil
}
