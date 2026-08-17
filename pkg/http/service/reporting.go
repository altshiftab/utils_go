package service

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"

	motmedelContext "github.com/altshiftab/utils_go/pkg/context"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	motmedelHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	motmedelMux "github.com/altshiftab/utils_go/pkg/http/mux"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_loader"
	bodyParserAdapter "github.com/altshiftab/utils_go/pkg/http/mux/types/body_parser/adapter"
	jsonSchemaBodyParser "github.com/altshiftab/utils_go/pkg/http/mux/types/body_parser/json_schema_body_parser"
	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	muxUtils "github.com/altshiftab/utils_go/pkg/http/mux/utils"
	motmedelHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/content_security_policy"
	"github.com/altshiftab/utils_go/pkg/http/types/integrity_policy"
	"github.com/altshiftab/utils_go/pkg/http/types/js_error_report"
	"github.com/altshiftab/utils_go/pkg/http/types/reporting_api"
	motmedelJson "github.com/altshiftab/utils_go/pkg/json"
	motmedelLog "github.com/altshiftab/utils_go/pkg/log"
	"github.com/altshiftab/utils_go/pkg/schema"
	"github.com/altshiftab/utils_go/pkg/utils"
)

// The paths the browser, and a page's own JavaScript, post their reports to. They are under the API
// prefix a robots.txt keeps crawlers out of, none of them being a document.
const (
	CspReportToPath              = "/api/report/csp-report-to"
	CspReportUriPath             = "/api/report/csp-report-uri"
	IntegrityPath                = "/api/report/integrity-endpoint"
	JsErrorPath                  = "/api/report/error"
	JsUnhandledRejectionPath     = "/api/report/unhandled-rejection"
	reportingEndpointsHeaderName = "Reporting-Endpoints"
	integrityPolicyHeaderName    = "Integrity-Policy"
	// integrityPolicyReportOnlyHeaderName says what the enforcing header would have blocked,
	// without blocking it.
	integrityPolicyReportOnlyHeaderName = "Integrity-Policy-Report-Only"
	cspReportToEndpointName             = "csp-report-to"
	integrityEndpointName               = "integrity-endpoint"
)

// maxReportBytes bounds a report body. A report is a small object describing one thing a browser
// blocked; anything larger is not one.
const maxReportBytes = 8192

// httpContextFromRequest retrieves the mux's http context, which the reports are attached to so
// that they are logged with what is known about the request that carried them. A report that
// arrives without one is still worth logging, so a failure is reported rather than returned.
func httpContextFromRequest(request *http.Request) *motmedelHttpTypes.HttpContext {
	ctx := request.Context()

	httpContext, err := utils.GetNonZeroContextValue[*motmedelHttpTypes.HttpContext](
		ctx,
		motmedelMux.MuxHttpContextContextKey,
	)
	if err != nil {
		slog.ErrorContext(
			motmedelContext.WithError(ctx, fmt.Errorf("get non-zero context value: %w", err)),
			"An error occurred when retrieving the mux http context.",
		)
	}

	return httpContext
}

// reportingFromHttpContext returns the http context's reporting, making it where it has none.
func reportingFromHttpContext(httpContext *motmedelHttpTypes.HttpContext) *schema.HttpReporting {
	if httpContext == nil {
		return nil
	}

	if httpContext.Reporting == nil {
		httpContext.Reporting = &schema.HttpReporting{}
	}

	return httpContext.Reporting
}

// messageFromReports is what a batch of reports is logged as: what the single report says, where
// there is one, and how many there were otherwise.
func messageFromReports[T interface{ Message() string }](
	reports []*reporting_api.Report[T],
	singular string,
	plural string,
) string {
	if len(reports) != 1 {
		return plural
	}

	if report := reports[0]; report != nil {
		var zero T
		if reportBody := report.Body; any(reportBody) != any(zero) {
			if message := reportBody.Message(); message != "" {
				return message
			}
		}
	}

	return singular
}

// patchReportingHeaders asks the browser to report what it blocks on the service's documents: the
// content security policy violations, through both the reporting endpoint the current
// specification defines and the report-uri the previous one did, and the integrity violations.
//
// report-uri is deprecated, and kept: it is the only way Firefox and Safari report a violation at
// all, neither having implemented the reporting endpoints that replaced it.
func patchReportingHeaders(mux *motmedelMux.Mux, integrityPolicyEnforced bool) error {
	if mux == nil {
		return motmedelErrors.NewWithTrace(nil_error.New("mux"))
	}

	defaultDocumentHeaders := mux.DefaultDocumentHeaders
	if defaultDocumentHeaders == nil {
		return motmedelErrors.NewWithTrace(nil_error.NewWithInstance("map", "default document headers"))
	}

	defaultDocumentHeaders[reportingEndpointsHeaderName] = fmt.Sprintf(
		"%s=\"%s\", %s=\"%s\"",
		cspReportToEndpointName,
		CspReportToPath,
		integrityEndpointName,
		IntegrityPath,
	)

	err := patchContentSecurityPolicy(
		mux,
		func(csp *content_security_policy.ContentSecurityPolicy) error {
			if reportToDirective := csp.GetReportTo(); reportToDirective != nil {
				reportToDirective.Token = cspReportToEndpointName
			} else {
				csp.Directives = append(
					csp.Directives,
					&content_security_policy.ReportToDirective{Token: cspReportToEndpointName},
				)
			}

			if reportUriDirective := csp.GetReportUri(); reportUriDirective != nil {
				if !slices.Contains(reportUriDirective.UriReferences, CspReportUriPath) {
					reportUriDirective.UriReferences = append(reportUriDirective.UriReferences, CspReportUriPath)
				}
			} else {
				csp.Directives = append(
					csp.Directives,
					&content_security_policy.ReportUriDirective{UriReferences: []string{CspReportUriPath}},
				)
			}

			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("patch content security policy: %w", err)
	}

	// Enforcing the policy is a bet that every browser reaching the service attaches the integrity
	// metadata a document gives it. A browser that loses it instead blocks the scripts and renders
	// nothing, which is why saying so is a decision a service makes rather than one made for it.
	integrityPolicyName := integrityPolicyReportOnlyHeaderName
	if integrityPolicyEnforced {
		integrityPolicyName = integrityPolicyHeaderName
	}

	defaultDocumentHeaders[integrityPolicyName] = fmt.Sprintf(
		"blocked-destinations=(script), endpoints=(%s)",
		integrityEndpointName,
	)

	return nil
}

// reportsEndpoint serves what a browser posts through the Reporting API: a batch of reports, which
// is attached to what is known about the request carrying them and then logged.
func reportsEndpoint[T interface{ Message() string }](
	path string,
	attach func(*schema.HttpReporting, []*reporting_api.Report[T]),
	singular string,
	plural string,
	reason string,
	action string,
) (*endpointPkg.Endpoint, error) {
	bodyParser, err := jsonSchemaBodyParser.New[[]*reporting_api.Report[T]]()
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("json schema body parser new: %w", err), path)
	}

	return &endpointPkg.Endpoint{
		Public: true,
		Path:   path,
		Method: http.MethodPost,
		BodyLoader: &body_loader.Loader{
			ContentType: "application/reports+json",
			MaxBytes:    maxReportBytes,
			Parser:      bodyParserAdapter.New(bodyParser),
		},
		Handler: func(request *http.Request, _ []byte) (*response.Response, *response_error.ResponseError) {
			ctx := request.Context()

			reports, err := muxUtils.GetParsedRequestBody[[]*reporting_api.Report[T]](ctx)
			if err != nil {
				return nil, &response_error.ResponseError{
					ServerError: motmedelErrors.New(fmt.Errorf("get parsed request body: %w", err)),
				}
			}

			httpContext := httpContextFromRequest(request)
			if reporting := reportingFromHttpContext(httpContext); reporting != nil && attach != nil {
				attach(reporting, reports)
			}

			slog.WarnContext(
				motmedelHttpContext.WithHttpContextValue(ctx, httpContext),
				messageFromReports(reports, singular, plural),
				slog.Group(
					"event",
					slog.String("reason", reason),
					slog.String("action", action),
				),
			)

			return nil, nil
		},
	}, nil
}

func cspReportToEndpoint() (*endpointPkg.Endpoint, error) {
	return reportsEndpoint(
		CspReportToPath,
		func(reporting *schema.HttpReporting, reports []*reporting_api.Report[*content_security_policy.CSPViolationReportBody]) {
			reporting.CspViolations = reports
		},
		"A CSP violation was reported.",
		"Multiple CSP violations were reported.",
		"CSP violations were reported.",
		"log_csp_violations",
	)
}

func cspReportUriEndpoint() (*endpointPkg.Endpoint, error) {
	bodyParser, err := jsonSchemaBodyParser.New[*content_security_policy.ReportEnvelope]()
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("json schema body parser new: %w", err))
	}

	return &endpointPkg.Endpoint{
		Public: true,
		Path:   CspReportUriPath,
		Method: http.MethodPost,
		BodyLoader: &body_loader.Loader{
			ContentType: "application/csp-report",
			MaxBytes:    maxReportBytes,
			Parser:      bodyParserAdapter.New(bodyParser),
		},
		Handler: func(request *http.Request, _ []byte) (*response.Response, *response_error.ResponseError) {
			ctx := request.Context()

			report, responseError := muxUtils.GetServerNonZeroParsedRequestBody[*content_security_policy.ReportEnvelope](ctx)
			if responseError != nil {
				return nil, responseError
			}

			httpContext := httpContextFromRequest(request)
			if reporting := reportingFromHttpContext(httpContext); reporting != nil {
				reporting.CspReport = report
			}

			slog.WarnContext(
				motmedelHttpContext.WithHttpContextValue(ctx, httpContext),
				report.Message(),
				slog.Group(
					"event",
					slog.String("reason", "A CSP violation was reported."),
					slog.String("action", "log_csp_report"),
				),
			)

			return nil, nil
		},
	}, nil
}

func integrityEndpoint() (*endpointPkg.Endpoint, error) {
	return reportsEndpoint(
		IntegrityPath,
		func(reporting *schema.HttpReporting, reports []*reporting_api.Report[*integrity_policy.IntegrityViolationReportBody]) {
			reporting.IntegrityViolations = reports
		},
		"An integrity violation was reported.",
		"Multiple integrity violations were reported.",
		"Integrity violations were reported.",
		"log_integrity_violations",
	)
}

// logJsError logs what a page's own JavaScript reported, as an error the way the schema records
// one rather than as the body it arrived as.
func logJsError(
	request *http.Request,
	errorDetails *js_error_report.ErrorDetails,
	errorType string,
	message string,
	reason string,
	action string,
) {
	ctx := request.Context()

	schemaError := &schema.Error{Type: errorType}
	if errorDetails != nil {
		schemaError.Message = errorDetails.Message
		schemaError.StackTrace = errorDetails.Stack
		if errorDetails.Code != 0 {
			schemaError.Code = strconv.Itoa(errorDetails.Code)
		}
	}

	errorMap, err := motmedelJson.ObjectToMap(schemaError)
	if err != nil {
		slog.ErrorContext(
			motmedelContext.WithError(ctx, fmt.Errorf("object to map: %w", err)),
			"An error occurred when converting the schema error to a map.",
		)
	}

	slog.WarnContext(
		motmedelHttpContext.WithHttpContextValue(ctx, httpContextFromRequest(request)),
		message,
		slog.Group("error", motmedelLog.AttrsFromMap(errorMap)...),
		slog.Group(
			"event",
			slog.String("reason", reason),
			slog.String("action", action),
		),
	)
}

func jsErrorEndpoint() (*endpointPkg.Endpoint, error) {
	bodyParser, err := jsonSchemaBodyParser.New[*js_error_report.ErrorBody]()
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("json schema body parser new: %w", err))
	}

	return &endpointPkg.Endpoint{
		Public: true,
		Path:   JsErrorPath,
		Method: http.MethodPost,
		BodyLoader: &body_loader.Loader{
			ContentType: "application/json",
			MaxBytes:    maxReportBytes,
			Parser:      bodyParserAdapter.New(bodyParser),
		},
		Handler: func(request *http.Request, _ []byte) (*response.Response, *response_error.ResponseError) {
			body, responseError := muxUtils.GetServerNonZeroParsedRequestBody[*js_error_report.ErrorBody](request.Context())
			if responseError != nil {
				return nil, responseError
			}

			message := body.Message
			if message == "" {
				message = "A JavaScript error was reported."
			}

			logJsError(
				request,
				body.Error,
				body.Type,
				message,
				"A JavaScript error was reported.",
				"log_js_error",
			)

			return nil, nil
		},
	}, nil
}

func jsUnhandledRejectionEndpoint() (*endpointPkg.Endpoint, error) {
	bodyParser, err := jsonSchemaBodyParser.New[*js_error_report.BaseErrorBody]()
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("json schema body parser new: %w", err))
	}

	return &endpointPkg.Endpoint{
		Public: true,
		Path:   JsUnhandledRejectionPath,
		Method: http.MethodPost,
		BodyLoader: &body_loader.Loader{
			ContentType: "application/json",
			MaxBytes:    maxReportBytes,
			Parser:      bodyParserAdapter.New(bodyParser),
		},
		Handler: func(request *http.Request, _ []byte) (*response.Response, *response_error.ResponseError) {
			body, responseError := muxUtils.GetServerNonZeroParsedRequestBody[*js_error_report.BaseErrorBody](request.Context())
			if responseError != nil {
				return nil, responseError
			}

			message := "A JavaScript unhandled rejection was reported."
			if errorDetails := body.Error; errorDetails != nil && errorDetails.Message != "" {
				message = errorDetails.Message
			}

			logJsError(
				request,
				body.Error,
				body.Type,
				message,
				"A JavaScript unhandled rejection was reported.",
				"log_js_unhandled_rejection",
			)

			return nil, nil
		},
	}, nil
}

// patchReporting asks the browser to report what it blocks, and serves the endpoints the reports go
// to -- the browser's own, and the ones a page's JavaScript posts its errors to.
func patchReporting(mux *motmedelMux.Mux, integrityPolicyEnforced bool) error {
	if mux == nil {
		return motmedelErrors.NewWithTrace(nil_error.New("mux"))
	}

	if err := patchReportingHeaders(mux, integrityPolicyEnforced); err != nil {
		return fmt.Errorf("patch reporting headers: %w", err)
	}

	makeEndpoints := []func() (*endpointPkg.Endpoint, error){
		cspReportToEndpoint,
		cspReportUriEndpoint,
		integrityEndpoint,
		jsErrorEndpoint,
		jsUnhandledRejectionEndpoint,
	}

	endpoints := make([]*endpointPkg.Endpoint, 0, len(makeEndpoints))
	for _, makeEndpoint := range makeEndpoints {
		endpoint, err := makeEndpoint()
		if err != nil {
			return fmt.Errorf("make endpoint: %w", err)
		}
		if endpoint == nil {
			return motmedelErrors.NewWithTrace(nil_error.New("endpoint"))
		}

		endpoints = append(endpoints, endpoint)
	}

	mux.Add(endpoints...)

	return nil
}
