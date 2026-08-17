package endpoint

import (
	"bytes"
	"io"
	"strings"
	"testing"

	altshiftBrotli "github.com/altshiftab/utils_go/pkg/brotli"
)

func TestAddContentEncodingDataBrotli(t *testing.T) {
	t.Parallel()

	data := []byte("<html><body>" + strings.Repeat("<div class=\"item\">repetitive markup</div>", 200) + "</body></html>")
	htmlEndpoint, err := NewFromDataPath("/index.html", data, "Sun, 02 Aug 2026 12:49:39 GMT", true, false)
	if err != nil {
		t.Fatalf("new from data path: %v", err)
	}
	if htmlEndpoint == nil || htmlEndpoint.StaticContent == nil {
		t.Fatal("nil endpoint or static content")
	}

	contentEncodingToData := htmlEndpoint.StaticContent.ContentEncodingToData
	for _, contentEncoding := range []string{"gzip", "br"} {
		if _, ok := contentEncodingToData[contentEncoding]; !ok {
			t.Fatalf("expected a %s variant, got %v", contentEncoding, contentEncodingToData)
		}
	}

	brotliData := contentEncodingToData["br"]
	if brotliData == nil {
		t.Fatal("nil brotli static content data")
	}
	if len(brotliData.Data) >= len(data) {
		t.Error("expected the brotli variant to be smaller than the original")
	}

	decompressed, err := io.ReadAll(altshiftBrotli.NewReader(bytes.NewReader(brotliData.Data)))
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if !bytes.Equal(decompressed, data) {
		t.Error("expected the decompressed brotli variant to equal the original")
	}

	var hasContentEncodingHeader bool
	for _, header := range brotliData.Headers {
		if header != nil && header.Name == "Content-Encoding" && header.Value == "br" {
			hasContentEncodingHeader = true
		}
	}
	if !hasContentEncodingHeader {
		t.Errorf("expected a Content-Encoding br header, got %v", brotliData.Headers)
	}
}
