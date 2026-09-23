package service

import (
	"bytes"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/service/service_config"
	"github.com/altshiftab/utils_go/pkg/http/types/http_context_extractor"
	"github.com/altshiftab/utils_go/pkg/http/types/http_context_extractor/http_context_extractor_config"
	altshiftHttpLogger "github.com/altshiftab/utils_go/pkg/log/http_logger"
	"github.com/altshiftab/utils_go/pkg/log/http_logger/http_logger_config"
	"github.com/altshiftab/utils_go/pkg/schema"
)

// The credential a page carries in its address, which the reports about that page name.
const reportingTestToken = "zhhjetavkELn1GOhywaXHdOatMAmhMro6ksgHu2XP7o" //nolint:gosec // G101: a stand-in, not a token.

const reportedPageUrl = "https://example.com/verification?token=" + reportingTestToken

// A report says what a browser blocked on a page, and names that page by its full address. Where
// the address is the credential, the report carries the credential -- into the entry the violation
// is logged under, into the access line for the request that delivered it, and into the raw body
// recorded alongside both.
//
// One declaration of what is secret covers all of them. Remove the MaskedUrlParams entry below and
// this fails.
//
//nolint:paralleltest // Replaces the default logger, which is global.
func TestReportedUrlsAreMasked(t *testing.T) {
	testCases := []struct {
		name        string
		path        string
		contentType string
		body        string
	}{
		{
			name:        "the reporting api endpoint",
			path:        CspReportToPath,
			contentType: "application/reports+json",
			body: `[{"age":44274,"type":"csp-violation","url":"` + reportedPageUrl + `",` +
				`"user_agent":"ua","body":{"documentURL":"` + reportedPageUrl + `",` +
				`"referrer":"` + reportedPageUrl + `","blockedURL":"trusted-types-policy",` +
				`"effectiveDirective":"trusted-types","originalPolicy":"trusted-types lit-html",` +
				`"disposition":"enforce","statusCode":200}}]`,
		},
		{
			name:        "the deprecated report-uri endpoint",
			path:        CspReportUriPath,
			contentType: "application/csp-report",
			body: `{"csp-report":{"document-uri":"` + reportedPageUrl + `",` +
				`"referrer":"` + reportedPageUrl + `","violated-directive":"script-src",` +
				`"effective-directive":"script-src","original-policy":"script-src 'self'",` +
				`"blocked-uri":"inline","disposition":"enforce","status-code":200}}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// What a service declares: the page whose query is a credential, and the report
			// endpoints whose bodies are recorded verbatim. The first covers every URL the entry
			// carries -- the Referer, and each URL the parsed report names. The second is needed
			// besides it because a body is opaque bytes rather than a URL, and is kept as sent.
			extractor := http_context_extractor.New(
				http_context_extractor_config.WithMaskedUrlParams(
					&schema.Url{Path: "/verification", Query: "token"},
				),
				http_context_extractor_config.WithMaskedRequestBodyUrls(
					&schema.Url{Path: CspReportToPath},
					&schema.Url{Path: CspReportUriPath},
				),
			)

			var buffer bytes.Buffer
			logger := altshiftHttpLogger.New(
				http_logger_config.WithLogLevel(slog.LevelDebug),
				http_logger_config.WithHttpContextExtractor(extractor),
				http_logger_config.WithWriter(&buffer),
			)

			previous := slog.Default()
			slog.SetDefault(logger.Logger)
			t.Cleanup(func() { slog.SetDefault(previous) })

			service, err := New(
				service_config.WithEndpoints(noContentEndpoint()),
				service_config.WithReporting(true),
			)
			if err != nil {
				t.Fatalf("new: %v", err)
			}

			address := serveListener(t, service)

			request, err := http.NewRequestWithContext(
				t.Context(),
				http.MethodPost,
				"http://"+address+testCase.path,
				strings.NewReader(testCase.body),
			)
			if err != nil {
				t.Fatalf("http new request with context: %v", err)
			}
			request.Header.Set("Content-Type", testCase.contentType)
			// The page hands its address to everything it asks for, this report included.
			request.Header.Set("Referer", reportedPageUrl)

			response, err := (&http.Client{}).Do(request)
			if err != nil {
				t.Fatalf("client do: %v", err)
			}
			defer func() { _ = response.Body.Close() }()

			if response.StatusCode >= http.StatusBadRequest {
				t.Fatalf("the report was refused with %d", response.StatusCode)
			}

			logged := buffer.String()
			if logged == "" {
				t.Fatal("expected the logger to have written something to check")
			}

			if strings.Contains(logged, reportingTestToken) {
				t.Errorf("the token reached the log: %s", logged)
			}

			// The page the report is about has to survive, or the report says nothing.
			if !strings.Contains(logged, "/verification") {
				t.Errorf("the reported page was lost from the log: %s", logged)
			}
		})
	}
}
