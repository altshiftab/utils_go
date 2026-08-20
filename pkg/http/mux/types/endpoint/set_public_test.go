package endpoint

import (
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint/static_content"
	muxTypesResponse "github.com/altshiftab/utils_go/pkg/http/mux/types/response"
)

func cacheControlOf(data *static_content.StaticContentData) string {
	if data == nil {
		return ""
	}
	for _, header := range data.Headers {
		if header != nil && header.Name == "Cache-Control" {
			return header.Value
		}
	}
	return ""
}

func staticEndpoint(cacheControl string, encodings map[string]string) *Endpoint {
	makeData := func(value string) *static_content.StaticContentData {
		return &static_content.StaticContentData{
			Headers: []*muxTypesResponse.HeaderEntry{
				{Name: "Content-Type", Value: "text/css"},
				{Name: "Cache-Control", Value: value, Overwrite: true},
			},
		}
	}

	encoded := map[string]*static_content.StaticContentData{}
	for name, value := range encodings {
		encoded[name] = makeData(value)
	}

	return &Endpoint{
		Path:   "/styles/index.css",
		Public: true,
		StaticContent: &static_content.StaticContent{
			StaticContentData:     *makeData(cacheControl),
			ContentEncodingToData: encoded,
		},
	}
}

func TestEndpoint_SetPublic(t *testing.T) {
	t.Parallel()

	t.Run("gating rewrites every encoding, not just the identity body", func(t *testing.T) {
		t.Parallel()

		endpoint := staticEndpoint(
			"public, max-age=31356000, immutable",
			map[string]string{
				"br":   "public, max-age=31356000, immutable",
				"gzip": "public, max-age=31356000, immutable",
			},
		)

		endpoint.SetPublic(false)

		if endpoint.Public {
			t.Fatal("Public should be false")
		}

		want := "private, max-age=31356000, immutable"
		if got := cacheControlOf(&endpoint.StaticContent.StaticContentData); got != want {
			t.Fatalf("identity: got %q, want %q", got, want)
		}
		for name, data := range endpoint.StaticContent.ContentEncodingToData {
			if got := cacheControlOf(data); got != want {
				t.Fatalf("encoding %s: got %q, want %q", name, got, want)
			}
		}
	})

	t.Run("opening it up goes the other way", func(t *testing.T) {
		t.Parallel()

		endpoint := staticEndpoint("private, max-age=60", map[string]string{"br": "private, max-age=60"})
		endpoint.SetPublic(true)

		if !endpoint.Public {
			t.Fatal("Public should be true")
		}
		if got := cacheControlOf(&endpoint.StaticContent.StaticContentData); got != "public, max-age=60" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("a header stating neither is left alone", func(t *testing.T) {
		t.Parallel()

		endpoint := staticEndpoint("no-cache", nil)
		endpoint.SetPublic(false)

		if got := cacheControlOf(&endpoint.StaticContent.StaticContentData); got != "no-cache" {
			t.Fatalf("got %q, want %q", got, "no-cache")
		}
	})

	t.Run("an unparsable header is left alone rather than dropped", func(t *testing.T) {
		t.Parallel()

		endpoint := staticEndpoint("!!! not a header !!!", nil)
		endpoint.SetPublic(false)

		if got := cacheControlOf(&endpoint.StaticContent.StaticContentData); got != "!!! not a header !!!" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("no static content, and nil", func(t *testing.T) {
		t.Parallel()

		endpoint := &Endpoint{Path: "/api/thing"}
		endpoint.SetPublic(false)
		if endpoint.Public {
			t.Fatal("Public should be false")
		}

		var nilEndpoint *Endpoint
		nilEndpoint.SetPublic(true)
	})
}
