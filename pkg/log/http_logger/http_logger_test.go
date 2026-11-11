package http_logger

import (
	"bufio"
	"bytes"
	"encoding/json/v2"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	altshiftContext "github.com/altshiftab/utils_go/pkg/context"
	altshiftHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	"github.com/altshiftab/utils_go/pkg/http/types/http_context_extractor"
	"github.com/altshiftab/utils_go/pkg/http/types/http_context_extractor/http_context_extractor_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
	"github.com/altshiftab/utils_go/pkg/log/entry_size_guard"
	"github.com/altshiftab/utils_go/pkg/log/http_logger/http_logger_config"
	"github.com/altshiftab/utils_go/pkg/schema"
)

func logRecord(t *testing.T, buffer *bytes.Buffer) map[string]any {
	t.Helper()

	var record map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &record); err != nil {
		t.Fatalf("json unmarshal log record: %v (%q)", err, buffer.String())
	}

	return record
}

// TestNew verifies that the logger writes JSON as the standard library names it, nothing being
// asked of it beyond that.
func TestNew(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer

	logger := New(http_logger_config.WithWriter(&buffer))
	if logger == nil || logger.Logger == nil {
		t.Fatal("nil logger")
	}

	logger.Info("test message")

	record := logRecord(t, &buffer)

	if record["msg"] != "test message" {
		t.Errorf("msg: got %v in %v", record["msg"], record)
	}
	if record["level"] != "INFO" {
		t.Errorf("level: got %v in %v", record["level"], record)
	}

	// What Cloud Logging reads is not written unless it is asked for.
	for _, key := range []string{"severity", "message"} {
		if value, found := record[key]; found {
			t.Errorf("%s: got %v, want nothing outside gcp mode", key, value)
		}
	}
}

// TestNewWithGcp verifies that the severity and the message are written under the names Cloud
// Logging reads them under, which is what the mode is for.
func TestNewWithGcp(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer

	logger := New(http_logger_config.WithWriter(&buffer), http_logger_config.WithGcp(true))
	if logger == nil || logger.Logger == nil {
		t.Fatal("nil logger")
	}

	logger.Info("test message")

	record := logRecord(t, &buffer)

	if record["message"] != "test message" {
		t.Errorf("message: got %v in %v", record["message"], record)
	}
	if record["severity"] != "INFO" {
		t.Errorf("severity: got %v in %v", record["severity"], record)
	}
}

// TestNewWithGcpGuardsTheEntrySize verifies that an entry too large for Cloud Logging to accept is
// reduced in gcp mode, and left alone otherwise -- nothing else rejecting it.
func TestNewWithGcpGuardsTheEntrySize(t *testing.T) {
	t.Parallel()

	oversized := strings.Repeat("a", entry_size_guard.DefaultEntryLimit)

	testCases := []struct {
		name              string
		gcp               bool
		expectedTruncated bool
	}{
		{name: "gcp", gcp: true, expectedTruncated: true},
		{name: "not gcp"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buffer bytes.Buffer

			logger := New(
				http_logger_config.WithWriter(&buffer),
				http_logger_config.WithGcp(testCase.gcp),
			)

			logger.Info("test message", slog.String("padding", oversized))

			truncated := strings.Contains(buffer.String(), entry_size_guard.TruncatedKey)
			if truncated != testCase.expectedTruncated {
				t.Errorf("truncated: got %t, want %t", truncated, testCase.expectedTruncated)
			}

			withinLimit := buffer.Len() <= entry_size_guard.DefaultEntryLimit
			if withinLimit != testCase.expectedTruncated {
				t.Errorf("within the limit: got %t (%d bytes)", withinLimit, buffer.Len())
			}
		})
	}
}

func TestNewLogLevelFilters(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer

	logger := New(
		http_logger_config.WithWriter(&buffer),
		http_logger_config.WithLogLevel(slog.LevelWarn),
	)

	logger.Info("filtered")
	if buffer.Len() != 0 {
		t.Errorf("the info record was written, though the level is warn: %q", buffer.String())
	}

	logger.Warn("emitted")
	if buffer.Len() == 0 {
		t.Error("the warn record was not written")
	}
}

// Cloud Logging reads the referrer from a field it builds itself, separately from the rest of the
// entry. A service that declares a query parameter secret would otherwise publish it there on every
// request made from the page whose address carries it.
func TestNewWithGcpMasksTheReferer(t *testing.T) {
	t.Parallel()

	const token = "zhhjetavkELn1GOhywaXHdOatMAmhMro6ksgHu2XP7o" //nolint:gosec // G101: a stand-in, not a token.
	const pageUrl = "https://example.com/verification?token=" + token

	request, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
		"GET /api/order/consented HTTP/1.1\r\nHost: example.com\r\nReferer: " + pageUrl + "\r\n\r\n",
	)))
	if err != nil {
		t.Fatalf("read request: %v", err)
	}

	extractor := http_context_extractor.New(
		http_context_extractor_config.WithMaskedUrlParams(
			&schema.Url{Path: "/verification", Query: "token"},
		),
	)

	var buffer bytes.Buffer
	logger := New(
		http_logger_config.WithWriter(&buffer),
		http_logger_config.WithGcp(true),
		http_logger_config.WithHttpContextExtractor(extractor),
	)

	logger.InfoContext(
		altshiftHttpContext.WithHttpContextValue(
			t.Context(),
			&altshiftHttpTypes.HttpContext{Request: request},
		),
		"An HTTP response was served.",
	)

	logged := buffer.String()
	if strings.Contains(logged, token) {
		t.Errorf("the token reached the log: %s", logged)
	}

	record := logRecord(t, &buffer)
	httpRequest, ok := record["httpRequest"].(map[string]any)
	if !ok {
		t.Fatalf("no httpRequest in the entry: %s", logged)
	}
	referer, _ := httpRequest["referer"].(string)
	if !strings.Contains(referer, "/verification") {
		t.Errorf("httpRequest.referer lost the referring page: %q", referer)
	}
}

// A name written into a URL's path is kept out of everything the service writes about a request
// for it: the debug entry of a request that went, with its URL in the fields, the message and the
// request line Cloud Logging reads, and the error of one that never reached its server, whose
// message Go writes the URL into. The extractor masks the path; the logger has the error's message
// masked by it too.
//
//nolint:paralleltest // The fetch writes its entry to the default logger, which this replaces.
func TestNewWithGcpMasksAUrlPath(t *testing.T) {
	const name = "Anna Andersson 2026-10-06.pptx"

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		writer.WriteHeader(http.StatusCreated)
	}))
	target := server.URL + "/v1.0/drives/d/items/f:/" + url.PathEscape(name) + ":/content"

	extractor := http_context_extractor.New(
		http_context_extractor_config.WithReplaceableMessages(http_logger_config.DefaultReplaceableMessages...),
		http_context_extractor_config.WithMaskedUrlPaths(&http_context_extractor_config.MaskedUrlPath{
			Pattern: regexp.MustCompile(`:/([^:]+):`),
		}),
	)

	var buffer lockedBuffer
	logger := New(
		http_logger_config.WithWriter(&buffer),
		http_logger_config.WithGcp(true),
		http_logger_config.WithLogLevel(slog.LevelDebug),
		http_logger_config.WithHttpContextExtractor(extractor),
	)

	// Fetched through the package's own fetch, which writes its debug entry to the default logger.
	previous := slog.Default()
	slog.SetDefault(logger.Logger)
	t.Cleanup(func() { slog.SetDefault(previous) })

	ctx := t.Context()
	put := func() error {
		_, _, err := altshiftHttpUtils.Fetch(
			ctx,
			target,
			fetch_config.WithMethod(http.MethodPut),
			fetch_config.WithBody([]byte("deck")),
			fetch_config.WithHttpClient(&http.Client{}),
		)
		return err
	}
	if err := put(); err != nil {
		t.Fatalf("put: %v", err)
	}

	server.Close()
	failed := put()
	if failed == nil {
		t.Fatal("expected an error once the server was gone")
	}
	if !strings.Contains(failed.Error(), url.PathEscape(name)) {
		t.Fatalf("the error does not carry the URL, so the test shows nothing: %v", failed)
	}
	logger.ErrorContext(altshiftContext.WithError(ctx, failed), "The deck could not be filed.")

	logged := buffer.String()
	if !strings.Contains(logged, "/v1.0/drives/d/items/f:/") {
		t.Fatalf("the log does not carry the exchange, so the test shows nothing: %s", logged)
	}
	for _, form := range []string{name, url.PathEscape(name), "Anna"} {
		if strings.Contains(logged, form) {
			t.Errorf("the log holds %q: %s", form, logged)
		}
	}
}

// lockedBuffer is a buffer two goroutines may write to: the fetch's entry may be written from the
// client's.
type lockedBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buffer.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buffer.String()
}
