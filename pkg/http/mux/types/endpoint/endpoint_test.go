package endpoint

import (
	"archive/zip"
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint/static_content"
	motmedelHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

func cacheControlValue(endpoint *Endpoint) string {
	if endpoint == nil || endpoint.StaticContent == nil {
		return ""
	}
	for _, header := range endpoint.StaticContent.Headers {
		if header != nil && header.Name == "Cache-Control" {
			return header.Value
		}
	}
	return ""
}

func TestNewRobotsTxt(t *testing.T) {
	t.Parallel()

	t.Run("nil input", func(t *testing.T) {
		t.Parallel()
		if NewRobotsTxt(nil) != nil {
			t.Fatal("expected nil for nil input")
		}
	})

	t.Run("empty robots.txt", func(t *testing.T) {
		t.Parallel()
		if NewRobotsTxt(&motmedelHttpTypes.RobotsTxt{}) != nil {
			t.Fatal("expected nil for an empty robots.txt")
		}
	})

	t.Run("valid robots.txt", func(t *testing.T) {
		t.Parallel()
		endpoint := NewRobotsTxt(&motmedelHttpTypes.RobotsTxt{
			Groups: []*motmedelHttpTypes.RobotsTxtGroup{
				{UserAgents: []string{"*"}, Disallowed: []string{"/private"}},
			},
		})
		if endpoint == nil {
			t.Fatal("expected an endpoint")
		}
		if endpoint.Path != "/robots.txt" || endpoint.Method != http.MethodGet || !endpoint.Public {
			t.Fatalf("unexpected endpoint: %#v", endpoint)
		}
		if endpoint.StaticContent == nil || len(endpoint.StaticContent.Data) == 0 {
			t.Fatal("expected static content with data")
		}
	})
}

func TestStaticContentParameter_HeaderEntries(t *testing.T) {
	t.Parallel()

	parameter := &StaticContentParameter{ContentType: "text/html", CacheControl: "no-cache"}
	entries := parameter.HeaderEntries(`"etag"`, "Mon, 01 Jan 2000 00:00:00 GMT")

	got := map[string]string{}
	for _, entry := range entries {
		got[entry.Name] = entry.Value
	}
	for name, want := range map[string]string{
		"Content-Type":  "text/html",
		"Cache-Control": "no-cache",
		"ETag":          `"etag"`,
		"Last-Modified": "Mon, 01 Jan 2000 00:00:00 GMT",
	} {
		if got[name] != want {
			t.Errorf("header %q = %q, want %q", name, got[name], want)
		}
	}
}

func TestAddContentEncodingData(t *testing.T) {
	t.Parallel()

	t.Run("nil is a no-op", func(t *testing.T) {
		t.Parallel()
		if err := AddContentEncodingData(nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("compressible data gets a gzip encoding", func(t *testing.T) {
		t.Parallel()
		staticContent := &static_content.StaticContent{
			StaticContentData: static_content.StaticContentData{
				Data: bytes.Repeat([]byte("a"), 2048),
			},
		}
		if err := AddContentEncodingData(staticContent); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := staticContent.ContentEncodingToData["gzip"]; !ok {
			t.Fatal("expected a gzip content encoding")
		}

		var hasVary bool
		for _, header := range staticContent.Headers {
			if header.Name == "Vary" && header.Value == "Accept-Encoding" {
				hasVary = true
			}
		}
		if !hasVary {
			t.Error("expected a Vary header")
		}
	})
}

func TestNewFromDataPath(t *testing.T) {
	t.Parallel()

	t.Run("empty path", func(t *testing.T) {
		t.Parallel()
		endpoint, err := NewFromDataPath("", nil, "", false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if endpoint != nil {
			t.Fatalf("expected a nil endpoint, got %#v", endpoint)
		}
	})

	t.Run("unsupported extension", func(t *testing.T) {
		t.Parallel()
		if _, err := NewFromDataPath("/file.unsupported", []byte("x"), "", false, false); err == nil {
			t.Fatal("expected an error for an unsupported extension")
		}
	})

	t.Run("adds a leading slash and stays public", func(t *testing.T) {
		t.Parallel()
		endpoint, err := NewFromDataPath("style.css", []byte("body{}"), "", false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if endpoint == nil || endpoint.Path != "/style.css" || !endpoint.Public {
			t.Fatalf("unexpected endpoint: %#v", endpoint)
		}
		if endpoint.StaticContent == nil || string(endpoint.StaticContent.Data) != "body{}" {
			t.Fatalf("unexpected static content: %#v", endpoint.StaticContent)
		}
	})

	t.Run("html extension is stripped", func(t *testing.T) {
		t.Parallel()
		endpoint, err := NewFromDataPath("/page.html", []byte("<html></html>"), "", false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if endpoint == nil || endpoint.Path != "/page" {
			t.Fatalf("expected path /page, got %#v", endpoint)
		}
	})

	t.Run("index becomes root", func(t *testing.T) {
		t.Parallel()
		endpoint, err := NewFromDataPath("index.html", []byte("<html></html>"), "", false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if endpoint == nil || endpoint.Path != "/" {
			t.Fatalf("expected path /, got %#v", endpoint)
		}
	})

	t.Run("private endpoint is not public", func(t *testing.T) {
		t.Parallel()
		endpoint, err := NewFromDataPath("app.js", []byte("x=1"), "", false, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if endpoint == nil || endpoint.Public {
			t.Fatalf("expected a private endpoint, got %#v", endpoint)
		}
	})

	t.Run("large compressible candidate is encoded", func(t *testing.T) {
		t.Parallel()
		endpoint, err := NewFromDataPath("big.svg", bytes.Repeat([]byte("a"), 2048), "", true, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if endpoint == nil || endpoint.StaticContent == nil {
			t.Fatalf("unexpected endpoint: %#v", endpoint)
		}
		if _, ok := endpoint.StaticContent.ContentEncodingToData["gzip"]; !ok {
			t.Error("expected a gzip content encoding for large compressible data")
		}
	})

	t.Run("visibility is not shared across calls", func(t *testing.T) {
		t.Parallel()

		publicEndpoint, err := NewFromDataPath("public.css", []byte("body{}"), "", false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		privateEndpoint, err := NewFromDataPath("private.css", []byte("body{}"), "", false, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got := cacheControlValue(publicEndpoint); !strings.Contains(got, "public") {
			t.Errorf("public endpoint Cache-Control = %q, want it to contain %q", got, "public")
		}
		if got := cacheControlValue(privateEndpoint); !strings.Contains(got, "private") {
			t.Errorf("private endpoint Cache-Control = %q, want it to contain %q", got, "private")
		}
	})
}

func TestNewFromDirectory(t *testing.T) {
	t.Parallel()

	t.Run("empty root", func(t *testing.T) {
		t.Parallel()
		endpoints, err := NewFromDirectory("", false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if endpoints != nil {
			t.Fatalf("expected nil, got %#v", endpoints)
		}
	})

	t.Run("loads files from a directory", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		// Two files of the same extension exercise NewFromDirectory's concurrent
		// NewFromDataPath calls (previously a data race on the shared parameter).
		if err := os.WriteFile(filepath.Join(dir, "a.css"), []byte("a{}"), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "b.css"), []byte("b{}"), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}

		endpoints, err := NewFromDirectory(dir, false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(endpoints) != 2 {
			t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
		}

		// The endpoints are loaded concurrently but returned sorted by path.
		var paths []string
		for _, endpoint := range endpoints {
			paths = append(paths, endpoint.Path)
		}
		expectedPaths := []string{"/a.css", "/b.css"}
		if !slices.Equal(paths, expectedPaths) {
			t.Fatalf("expected paths %v, got %v", expectedPaths, paths)
		}
	})
}

func TestNewFromZip(t *testing.T) {
	t.Parallel()

	t.Run("nil reader", func(t *testing.T) {
		t.Parallel()
		endpoints, err := NewFromZip(nil, false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if endpoints != nil {
			t.Fatalf("expected nil, got %#v", endpoints)
		}
	})

	t.Run("loads files from a zip", func(t *testing.T) {
		t.Parallel()

		var buffer bytes.Buffer
		zipWriter := zip.NewWriter(&buffer)
		for name, content := range map[string]string{"a.css": "a{}", "b.css": "b{}"} {
			fileWriter, err := zipWriter.Create(name)
			if err != nil {
				t.Fatalf("zip create: %v", err)
			}
			if _, err := fileWriter.Write([]byte(content)); err != nil {
				t.Fatalf("zip write: %v", err)
			}
		}
		if err := zipWriter.Close(); err != nil {
			t.Fatalf("zip close: %v", err)
		}

		reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
		if err != nil {
			t.Fatalf("zip new reader: %v", err)
		}

		endpoints, err := NewFromZip(reader, false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(endpoints) != 2 {
			t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
		}

		// The endpoints are loaded concurrently but returned sorted by path.
		var paths []string
		for _, endpoint := range endpoints {
			paths = append(paths, endpoint.Path)
		}
		expectedPaths := []string{"/a.css", "/b.css"}
		if !slices.Equal(paths, expectedPaths) {
			t.Fatalf("expected paths %v, got %v", expectedPaths, paths)
		}
	})
}

func TestCompareEndpoints(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		a        *Endpoint
		b        *Endpoint
		expected int
	}{
		{name: "both nil", a: nil, b: nil, expected: 0},
		{name: "nil sorts first", a: nil, b: &Endpoint{Path: "/a"}, expected: -1},
		{name: "nil sorts first (reversed)", a: &Endpoint{Path: "/a"}, b: nil, expected: 1},
		{name: "ordered by path", a: &Endpoint{Path: "/a"}, b: &Endpoint{Path: "/b"}, expected: -1},
		{
			name:     "equal paths ordered by method",
			a:        &Endpoint{Path: "/a", Method: http.MethodGet},
			b:        &Endpoint{Path: "/a", Method: http.MethodPost},
			expected: -1,
		},
		{
			name:     "equal",
			a:        &Endpoint{Path: "/a", Method: http.MethodGet},
			b:        &Endpoint{Path: "/a", Method: http.MethodGet},
			expected: 0,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if result := compareEndpoints(testCase.a, testCase.b); result != testCase.expected {
				t.Errorf("compareEndpoints() = %d, expected %d", result, testCase.expected)
			}
		})
	}
}

func TestDuplicate(t *testing.T) {
	t.Parallel()

	original := &Endpoint{Path: "/", Method: http.MethodGet, Public: true}

	testCases := []struct {
		name          string
		endpoint      *Endpoint
		paths         []string
		expectedPaths []string
	}{
		{
			name:          "one path",
			endpoint:      original,
			paths:         []string{"/about"},
			expectedPaths: []string{"/about"},
		},
		{
			name:          "several paths",
			endpoint:      original,
			paths:         []string{"/about", "/contact"},
			expectedPaths: []string{"/about", "/contact"},
		},
		{name: "no paths", endpoint: original},
		{name: "an empty path is skipped", endpoint: original, paths: []string{""}},
		{name: "no endpoint", paths: []string{"/about"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			duplicates := Duplicate(testCase.endpoint, testCase.paths...)

			if len(duplicates) != len(testCase.expectedPaths) {
				t.Fatalf("duplicates: got %d, want %d", len(duplicates), len(testCase.expectedPaths))
			}

			for i, expectedPath := range testCase.expectedPaths {
				duplicate := duplicates[i]
				if duplicate.Path != expectedPath {
					t.Errorf("path: got %q, want %q", duplicate.Path, expectedPath)
				}
				// The duplicate answers as the original does, at another path.
				if duplicate.Method != testCase.endpoint.Method || duplicate.Public != testCase.endpoint.Public {
					t.Errorf("duplicate: got %+v, want the original but for the path", duplicate)
				}
			}

			// The original is left as it was.
			if testCase.endpoint != nil && testCase.endpoint.Path != "/" {
				t.Errorf("the original was changed: %+v", testCase.endpoint)
			}
		})
	}
}
