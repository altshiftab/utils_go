package types

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"testing"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// readArchiveBytes parses raw tar bytes into a name -> content map.
func readArchiveBytes(t *testing.T, data []byte) map[string][]byte {
	t.Helper()

	result := make(map[string][]byte)
	reader := tar.NewReader(bytes.NewReader(data))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tar reader Next: %v", err)
		}

		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read tar content: %v", err)
		}
		result[header.Name] = content
	}

	return result
}

func regEntry(name string, content []byte) *Entry {
	return &Entry{
		Header: &tar.Header{
			Name:     name,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
			Mode:     0o644,
		},
		Content: content,
	}
}

func TestArchiveBytesEmpty(t *testing.T) {
	t.Parallel()

	data, err := Archive{}.Bytes()
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}
	if data != nil {
		t.Errorf("Bytes() = %v, want nil", data)
	}
}

func TestArchiveBytesRoundTrip(t *testing.T) {
	t.Parallel()

	archive := Archive{
		"a.txt": regEntry("a.txt", []byte("hello")),
		"b.txt": regEntry("b.txt", []byte("world")),
		"empty": regEntry("empty", nil),
	}

	data, err := archive.Bytes()
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}

	got := readArchiveBytes(t, data)
	if len(got) != 3 {
		t.Fatalf("parsed %d entries, want 3", len(got))
	}
	if string(got["a.txt"]) != "hello" {
		t.Errorf("a.txt = %q, want %q", got["a.txt"], "hello")
	}
	if string(got["b.txt"]) != "world" {
		t.Errorf("b.txt = %q, want %q", got["b.txt"], "world")
	}
	if len(got["empty"]) != 0 {
		t.Errorf("empty = %q, want empty", got["empty"])
	}
}

func TestArchiveBytesSkipsNilAndHeaderless(t *testing.T) {
	t.Parallel()

	archive := Archive{
		"good":       regEntry("good", []byte("data")),
		"nil-entry":  nil,
		"nil-header": &Entry{Header: nil, Content: []byte("ignored")},
	}

	data, err := archive.Bytes()
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}

	got := readArchiveBytes(t, data)
	if len(got) != 1 {
		t.Fatalf("parsed %d entries, want 1", len(got))
	}
	if string(got["good"]) != "data" {
		t.Errorf("good = %q, want %q", got["good"], "data")
	}
}

func TestArchiveBytesInvalidHeader(t *testing.T) {
	t.Parallel()

	// Size smaller than the actual content triggers a write error from the tar writer.
	archive := Archive{
		"bad": {
			Header:  &tar.Header{Name: "bad", Size: 1, Typeflag: tar.TypeReg},
			Content: []byte("too long"),
		},
	}

	_, err := archive.Bytes()
	if err == nil {
		t.Fatal("Bytes() error = nil, want error")
	}
	if _, ok := errors.AsType[*motmedelErrors.Error](err); !ok {
		t.Errorf("error type = %T, want *motmedelErrors.Error", err)
	}
}

func TestArchiveFilter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		patterns     []string
		wantIncluded []string
	}{
		{
			name:         "no patterns keeps all",
			patterns:     nil,
			wantIncluded: []string{"a.txt", "b.log", "c.txt"},
		},
		{
			name:         "empty pattern ignored",
			patterns:     []string{""},
			wantIncluded: []string{"a.txt", "b.log", "c.txt"},
		},
		{
			name:         "exclude by glob",
			patterns:     []string{"*.txt"},
			wantIncluded: []string{"b.log"},
		},
		{
			name:         "negate re-includes",
			patterns:     []string{"*.txt", "!c.txt"},
			wantIncluded: []string{"b.log", "c.txt"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			archive := Archive{
				"a.txt": regEntry("a.txt", []byte("a")),
				"b.log": regEntry("b.log", []byte("b")),
				"c.txt": regEntry("c.txt", []byte("c")),
			}

			filtered, err := archive.Filter(testCase.patterns...)
			if err != nil {
				t.Fatalf("Filter() error = %v", err)
			}

			if len(filtered) != len(testCase.wantIncluded) {
				t.Fatalf("filtered has %d entries (%v), want %d", len(filtered), keys(filtered), len(testCase.wantIncluded))
			}
			for _, name := range testCase.wantIncluded {
				if _, ok := filtered[name]; !ok {
					t.Errorf("expected %q to be included, got %v", name, keys(filtered))
				}
			}
		})
	}
}

func TestArchiveFilterInvalidPattern(t *testing.T) {
	t.Parallel()

	archive := Archive{"a.txt": regEntry("a.txt", []byte("a"))}

	_, err := archive.Filter("[")
	if err == nil {
		t.Fatal("Filter() error = nil, want error")
	}
	if _, ok := errors.AsType[*motmedelErrors.Error](err); !ok {
		t.Errorf("error type = %T, want *motmedelErrors.Error", err)
	}
}

func TestArchiveSetDirectory(t *testing.T) {
	t.Parallel()

	t.Run("empty archive", func(t *testing.T) {
		t.Parallel()

		got, ok := Archive{}.SetDirectory("dir")
		if got != nil {
			t.Errorf("archive = %v, want nil", got)
		}
		if ok {
			t.Errorf("ok = true, want false")
		}
	})

	t.Run("renames entries under directory", func(t *testing.T) {
		t.Parallel()

		dirEntry := regEntry("dir/", nil)
		dirEntry.Header.Typeflag = tar.TypeDir

		archive := Archive{
			"dir/":            dirEntry,
			"dir/file.txt":    regEntry("dir/file.txt", []byte("x")),
			"dir/sub/nest.go": regEntry("dir/sub/nest.go", []byte("y")),
			"other.txt":       regEntry("other.txt", []byte("z")),
		}

		got, ok := archive.SetDirectory("dir")
		if !ok {
			t.Fatal("ok = false, want true")
		}

		// The directory entry itself ("dir/") is dropped; entries under it are renamed;
		// entries outside are excluded.
		if _, exists := got["file.txt"]; !exists {
			t.Errorf("expected renamed file.txt, got %v", keys(got))
		}
		if _, exists := got["sub/nest.go"]; !exists {
			t.Errorf("expected renamed sub/nest.go, got %v", keys(got))
		}
		if _, exists := got["other.txt"]; exists {
			t.Errorf("other.txt should be excluded, got %v", keys(got))
		}
		if _, exists := got["dir/"]; exists {
			t.Errorf("directory entry itself should be excluded, got %v", keys(got))
		}
		if len(got) != 2 {
			t.Errorf("got %d entries, want 2 (%v)", len(got), keys(got))
		}
		// The renamed entry's header Name is updated.
		if got["file.txt"].Header.Name != "file.txt" {
			t.Errorf("header Name = %q, want %q", got["file.txt"].Header.Name, "file.txt")
		}
	})

	t.Run("skips non-regular non-directory and nil entries", func(t *testing.T) {
		t.Parallel()

		symlink := regEntry("dir/link", nil)
		symlink.Header.Typeflag = tar.TypeSymlink

		archive := Archive{
			"dir/keep": regEntry("dir/keep", []byte("k")),
			"dir/link": symlink,
			"dir/nil":  nil,
			"dir/hdr":  {Header: nil},
		}

		got, ok := archive.SetDirectory("dir")
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if len(got) != 1 {
			t.Fatalf("got %d entries, want 1 (%v)", len(got), keys(got))
		}
		if _, exists := got["keep"]; !exists {
			t.Errorf("expected keep, got %v", keys(got))
		}
	})

	t.Run("no matching directory", func(t *testing.T) {
		t.Parallel()

		archive := Archive{"other/file": regEntry("other/file", []byte("x"))}
		got, ok := archive.SetDirectory("dir")
		if ok {
			t.Errorf("ok = true, want false")
		}
		if len(got) != 0 {
			t.Errorf("got %d entries, want 0", len(got))
		}
	})
}

func TestArchiveAddBasicFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		path     string
		content  []byte
		wantKey  string
		wantSize int64
	}{
		{name: "plain path", path: "file.txt", content: []byte("abc"), wantKey: "file.txt", wantSize: 3},
		{name: "unclean path", path: "./dir/../file.txt", content: []byte("hello"), wantKey: "file.txt", wantSize: 5},
		{name: "empty content", path: "empty.txt", content: nil, wantKey: "empty.txt", wantSize: 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			archive := make(Archive)
			archive.AddBasicFile(testCase.path, testCase.content)

			entry, ok := archive[testCase.wantKey]
			if !ok {
				t.Fatalf("key %q missing, keys = %v", testCase.wantKey, keys(archive))
			}
			if entry.Header.Name != testCase.wantKey {
				t.Errorf("header Name = %q, want %q", entry.Header.Name, testCase.wantKey)
			}
			if entry.Header.Size != testCase.wantSize {
				t.Errorf("header Size = %d, want %d", entry.Header.Size, testCase.wantSize)
			}
			if !bytes.Equal(entry.Content, testCase.content) {
				t.Errorf("content = %q, want %q", entry.Content, testCase.content)
			}
		})
	}
}

func TestArchiveAddBasicFileRoundTrip(t *testing.T) {
	t.Parallel()

	archive := make(Archive)
	archive.AddBasicFile("hello.txt", []byte("content"))

	data, err := archive.Bytes()
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}

	got := readArchiveBytes(t, data)
	if string(got["hello.txt"]) != "content" {
		t.Errorf("hello.txt = %q, want %q", got["hello.txt"], "content")
	}
}

func keys(archive Archive) []string {
	result := make([]string, 0, len(archive))
	for k := range archive {
		result = append(result, k)
	}
	return result
}
