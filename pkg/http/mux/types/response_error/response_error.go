package response_error

import (
	"encoding/json/v2"
	"encoding/xml"
	"fmt"
	"net/http"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	muxErrors "github.com/altshiftab/utils_go/pkg/http/mux/errors"
	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	motmedelHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	motmedelHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

type ResponseErrorType int

const (
	ResponseErrorType_Invalid ResponseErrorType = iota
	ResponseErrorType_ClientError
	ResponseErrorType_ServerError
)

type ProblemDetailConverter interface {
	Convert(*problem_detail.Detail, *motmedelHttpTypes.ContentNegotiation) ([]byte, string, error)
}

type ProblemDetailConverterFunction func(*problem_detail.Detail, *motmedelHttpTypes.ContentNegotiation) ([]byte, string, error)

func (f ProblemDetailConverterFunction) Convert(
	problemDetail *problem_detail.Detail,
	contentNegotiation *motmedelHttpTypes.ContentNegotiation,
) ([]byte, string, error) {
	return f(problemDetail, contentNegotiation)
}

const applicationType = "application"

var DefaultProblemDetailMediaRanges = []*motmedelHttpTypes.ServerMediaRange{
	{Type: applicationType, Subtype: "problem+json"},
	{Type: applicationType, Subtype: "json"},
	{Type: applicationType, Subtype: "problem+xml"},
	{Type: applicationType, Subtype: "xml"},
	{Type: "text", Subtype: "plain"},
}

// TODO: Move to `problem_detail` package.
func ConvertProblemDetail(
	detail *problem_detail.Detail,
	negotiation *motmedelHttpTypes.ContentNegotiation,
) ([]byte, string, error) {
	if detail == nil {
		return nil, "", nil
	}

	if negotiation != nil {
		if negotiation.NegotiatedAccept == "" && negotiation.Accept != nil {
			matchingServerMediaRange := motmedelHttpUtils.GetMatchingAccept(
				negotiation.Accept.GetPriorityOrderedEncodings(),
				DefaultProblemDetailMediaRanges,
			)
			if matchingServerMediaRange != nil {
				negotiation.NegotiatedAccept = matchingServerMediaRange.GetFullType(true)
			}
		}

		switch negotiatedAccept := negotiation.NegotiatedAccept; negotiatedAccept {
		case "application/problem+xml", "application/xml":
			data, err := xml.Marshal(detail)
			if err != nil {
				return nil, "", motmedelErrors.New(fmt.Errorf("xml marshal: %w", err), detail)
			}

			output := []byte(`<?xml version="1.0" encoding="UTF-8"?>`)
			output = append(output, data...)

			return output, "application/problem+xml", nil
		case "text/plain":
			text, err := detail.String()
			if err != nil {
				return nil, "", motmedelErrors.New(fmt.Errorf("problem detail string: %w", err), detail)
			}
			return []byte(text), negotiatedAccept, nil
		}
	}

	// Default to using JSON.
	data, err := json.Marshal(detail)
	if err != nil {
		return nil, "", motmedelErrors.New(fmt.Errorf("json marshal: %w", err), detail)
	}

	return data, "application/problem+json", nil
}

var DefaultProblemDetailConverter = ProblemDetailConverterFunction(ConvertProblemDetail)

type ResponseError struct {
	ProblemDetail          *problem_detail.Detail
	Headers                []*muxTypesResponse.HeaderEntry
	ClientError            error
	ServerError            error
	ProblemDetailConverter ProblemDetailConverter
}

func (responseError *ResponseError) Type() ResponseErrorType {
	if responseError.ServerError != nil {
		return ResponseErrorType_ServerError
	} else if responseError.ClientError != nil {
		return ResponseErrorType_ClientError
	} else if problemDetail := responseError.ProblemDetail; problemDetail != nil {
		statusCode := problemDetail.Status
		if statusCode >= 400 && statusCode < 500 {
			return ResponseErrorType_ClientError
		} else if statusCode >= 500 && statusCode < 600 {
			return ResponseErrorType_ServerError
		}
	}

	return ResponseErrorType_Invalid
}

func (responseError *ResponseError) GetEffectiveProblemDetail() (*problem_detail.Detail, error) {
	if problemDetail := responseError.ProblemDetail; problemDetail != nil {
		return problemDetail, nil
	}

	if responseError.ClientError != nil && responseError.ServerError != nil {
		return nil, motmedelErrors.NewWithTrace(muxErrors.ErrMultipleResponseErrorErrors)
	}

	if responseError.ServerError != nil {
		return problem_detail.New(http.StatusInternalServerError), nil
	}

	if responseError.ClientError != nil {
		return problem_detail.New(http.StatusBadRequest), nil
	}

	return nil, motmedelErrors.NewWithTrace(
		fmt.Errorf(
			"%w: %w, %w",
			muxErrors.ErrUnusableResponseError,
			nil_error.New("problem detail"),
			empty_error.New("response error errors"),
		),
	)
}

func (responseError *ResponseError) MakeResponse(
	negotiation *motmedelHttpTypes.ContentNegotiation,
) (*muxTypesResponse.Response, error) {
	problemDetail := responseError.ProblemDetail
	if problemDetail == nil {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: %w", muxErrors.ErrUnusableResponseError, nil_error.New("problem detail")),
		)
	}

	statusCode := problemDetail.Status
	if statusCode == 0 {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: problem detail: %w", muxErrors.ErrUnusableResponseError, empty_error.New("status")),
		)
	}

	headers := responseError.Headers
	if len(headers) != 0 {
		for i, header := range headers {
			if header == nil || header.Name == "" {
				continue
			}

			switch http.CanonicalHeaderKey(header.Name) {
			case "Content-Type":
				// Clear any pre-existing Content-Type header.
				headers[i] = nil
			case "Location":
				// Use a redirect status code if a Location header is present.
				// It is the responsibility of the setter of the header to make sure it is present only in navigation
				// responses.
				statusCode = http.StatusSeeOther
			}
		}
	}

	supportsResponseBody := true
	if negotiation != nil && negotiation.AcceptEncoding != nil {
		supportsResponseBody = motmedelHttpUtils.GetMatchingContentEncoding(
			negotiation.AcceptEncoding.GetPriorityOrderedEncodings(),
			[]string{motmedelHttpUtils.AcceptContentIdentity},
		) == motmedelHttpUtils.AcceptContentIdentity
	}

	var body []byte

	if supportsResponseBody {
		converter := responseError.ProblemDetailConverter
		if converter == nil {
			converter = DefaultProblemDetailConverter
		}

		var contentType string
		var err error
		body, contentType, err = converter.Convert(problemDetail, negotiation)
		if err != nil {
			return nil, motmedelErrors.New(
				fmt.Errorf("convert: %w", err),
				problemDetail, negotiation,
			)
		}

		if len(body) != 0 {
			if contentType == "" {
				return nil, motmedelErrors.NewWithTrace(empty_error.New("response error content type"))
			}

			headers = append(
				headers,
				&muxTypesResponse.HeaderEntry{Name: "Content-Type", Value: contentType},
			)
		}
	}

	return &muxTypesResponse.Response{StatusCode: statusCode, Body: body, Headers: headers}, nil
}
