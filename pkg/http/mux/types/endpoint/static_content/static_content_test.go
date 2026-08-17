package static_content

import (
	"bytes"
	"testing"

	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
)

func TestStaticContentData_Fields(t *testing.T) {
	t.Parallel()

	data := StaticContentData{
		Data:         []byte("payload"),
		Etag:         `"abc123"`,
		LastModified: "Mon, 02 Jan 2006 15:04:05 GMT",
		Headers: []*muxTypesResponse.HeaderEntry{
			{Name: "Cache-Control", Value: "max-age=3600"},
		},
	}

	if !bytes.Equal(data.Data, []byte("payload")) {
		t.Errorf("Data = %q, want %q", data.Data, "payload")
	}
	if data.Etag != `"abc123"` {
		t.Errorf("Etag = %q, want %q", data.Etag, `"abc123"`)
	}
	if data.LastModified != "Mon, 02 Jan 2006 15:04:05 GMT" {
		t.Errorf("LastModified = %q, unexpected", data.LastModified)
	}
	if len(data.Headers) != 1 || data.Headers[0].Name != "Cache-Control" {
		t.Errorf("Headers = %+v, unexpected", data.Headers)
	}
}

func TestStaticContent_EmbeddedFieldPromotion(t *testing.T) {
	t.Parallel()

	content := &StaticContent{
		StaticContentData: StaticContentData{
			Data:         []byte("identity"),
			Etag:         `"etag-identity"`,
			LastModified: "now",
		},
		ContentEncodingToData: map[string]*StaticContentData{
			"gzip": {
				Data: []byte("gzipped"),
				Etag: `"etag-gzip"`,
			},
		},
	}

	// Promoted fields from the embedded StaticContentData.
	if !bytes.Equal(content.Data, []byte("identity")) {
		t.Errorf("promoted Data = %q, want %q", content.Data, "identity")
	}
	if content.Etag != `"etag-identity"` {
		t.Errorf("promoted Etag = %q, want %q", content.Etag, `"etag-identity"`)
	}

	gzipData, ok := content.ContentEncodingToData["gzip"]
	if !ok {
		t.Fatal("gzip encoding not present in ContentEncodingToData")
	}
	if !bytes.Equal(gzipData.Data, []byte("gzipped")) {
		t.Errorf("gzip Data = %q, want %q", gzipData.Data, "gzipped")
	}
	if gzipData.Etag != `"etag-gzip"` {
		t.Errorf("gzip Etag = %q, want %q", gzipData.Etag, `"etag-gzip"`)
	}
}

func TestStaticContent_ZeroValue(t *testing.T) {
	t.Parallel()

	var content StaticContent
	if content.Data != nil {
		t.Error("zero value Data = non-nil, want nil")
	}
	if content.Etag != "" {
		t.Errorf("zero value Etag = %q, want empty", content.Etag)
	}
	if content.ContentEncodingToData != nil {
		t.Error("zero value ContentEncodingToData = non-nil, want nil")
	}
}
