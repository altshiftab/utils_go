package dockerarchive

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

// tarEntry describes one entry of a layer archive built in a test.
type tarEntry struct {
	name     string
	typeflag byte
	content  string
	linkname string
}

func file(name, content string) tarEntry {
	return tarEntry{name: name, typeflag: tar.TypeReg, content: content}
}
func dir(name string) tarEntry { return tarEntry{name: name, typeflag: tar.TypeDir} }
func symlink(name, target string) tarEntry {
	return tarEntry{name: name, typeflag: tar.TypeSymlink, linkname: target}
}
func hardlink(name, target string) tarEntry {
	return tarEntry{name: name, typeflag: tar.TypeLink, linkname: target}
}

func writeTar(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Typeflag: entry.typeflag, Mode: 0o644, Linkname: entry.linkname}
		if entry.typeflag == tar.TypeReg {
			header.Size = int64(len(entry.content))
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatalf("write header %s: %v", entry.name, err)
		}
		if entry.typeflag == tar.TypeReg {
			if _, err := writer.Write([]byte(entry.content)); err != nil {
				t.Fatalf("write %s: %v", entry.name, err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	return buffer.Bytes()
}

func gzipped(t *testing.T, data []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(data); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buffer.Bytes()
}

// archiveEntry is one entry of the outer docker-archive tar.
type archiveEntry struct {
	name     string
	data     []byte
	linkname string
}

// writeArchive writes a docker archive: the given entries first (layers and configs, in whatever order), the manifest
// last, as podman writes it.
func writeArchive(t *testing.T, entries []archiveEntry, manifest string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	write := func(entry archiveEntry) {
		header := &tar.Header{Name: entry.name, Mode: 0o444}
		if entry.linkname != "" {
			header.Typeflag = tar.TypeSymlink
			header.Linkname = entry.linkname
		} else {
			header.Typeflag = tar.TypeReg
			header.Size = int64(len(entry.data))
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatalf("write header %s: %v", entry.name, err)
		}
		if entry.linkname == "" {
			if _, err := writer.Write(entry.data); err != nil {
				t.Fatalf("write %s: %v", entry.name, err)
			}
		}
	}
	for _, entry := range entries {
		write(entry)
	}
	if manifest != "" {
		write(archiveEntry{name: "manifest.json", data: []byte(manifest)})
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return buffer.Bytes()
}

// captureText keeps the content of every regular file, so tests can see the resolved view.
func captureText(_ string, _ *tar.Header, reader *bufio.Reader) (any, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// captureNotScripts keeps every regular file except shell scripts, standing in for a capture that declines a file.
func captureNotScripts(_ string, _ *tar.Header, reader *bufio.Reader) (any, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(string(data), "#!/") {
		return nil, nil
	}
	return string(data), nil
}

func resolvedContents(image *Image) map[string]string {
	contents := make(map[string]string, len(image.Files))
	for filePath, file := range image.Files {
		contents[filePath] = file.Payload.(string) //nolint:errcheck // test payloads are strings by construction
	}
	return contents
}

func TestRead(t *testing.T) {
	t.Parallel()

	const config = "abc123.json"

	testCases := []struct {
		name      string
		layers    [][]tarEntry
		gzipLast  bool
		manifest  string
		reference string
		capture   Capture
		expected  map[string]string
		layer     map[string]int
		id        string
		err       error
	}{
		{
			name: "upper layer replaces and adds",
			layers: [][]tarEntry{
				{dir("etc"), file("etc/os-release", "ID=alpine"), file("etc/keep", "keep")},
				{file("etc/os-release", "ID=alpine\nVERSION_ID=3.24.1")},
			},
			expected: map[string]string{"etc/os-release": "ID=alpine\nVERSION_ID=3.24.1", "etc/keep": "keep"},
			layer:    map[string]int{"etc/os-release": 1, "etc/keep": 0},
			id:       "sha256:abc123",
		},
		{
			name: "file whiteout",
			layers: [][]tarEntry{
				{file("usr/bin/app", "old"), file("usr/bin/other", "x")},
				{file("usr/bin/.wh.app", "")},
			},
			expected: map[string]string{"usr/bin/other": "x"},
		},
		{
			name: "directory whiteout removes the tree",
			layers: [][]tarEntry{
				{file("var/lib/apk/db/installed", "P:a"), file("var/lib/apk/db/lock", ""), file("var/log/x", "x")},
				{file("var/lib/.wh.apk", "")},
			},
			expected: map[string]string{"var/log/x": "x"},
		},
		{
			name: "opaque directory hides lower contents but keeps its own",
			layers: [][]tarEntry{
				{file("opt/a", "a"), file("opt/sub/b", "b"), file("etc/c", "c")},
				{file("opt/.wh..wh..opq", ""), file("opt/new", "new")},
			},
			expected: map[string]string{"etc/c": "c", "opt/new": "new"},
		},
		{
			name: "whiteout applies to lower layers only, then the same layer's file lands",
			layers: [][]tarEntry{
				{file("etc/os-release", "old")},
				{file("etc/.wh.os-release", ""), file("etc/os-release", "new")},
			},
			expected: map[string]string{"etc/os-release": "new"},
		},
		{
			name: "upper layer replacing a captured file with a script, a symlink or a directory removes it",
			layers: [][]tarEntry{
				{file("usr/bin/app", "\x7fELF"), file("etc/os-release", "ID=old"), file("opt/tool", "\x7fELF"), file("var/keep", "k")},
				{file("usr/bin/app", "#!/bin/sh"), symlink("etc/os-release", "../usr/lib/os-release"), dir("opt/tool")},
			},
			capture:  captureNotScripts,
			expected: map[string]string{"var/keep": "k"},
		},
		{
			name: "file replacing a directory removes what was beneath it",
			layers: [][]tarEntry{
				{dir("data"), file("data/a", "a"), file("data/b", "b")},
				{file("data", "now a file")},
			},
			expected: map[string]string{"data": "now a file"},
		},
		{
			name: "hard link aliases a captured file",
			layers: [][]tarEntry{
				{file("bin/busybox", "ELF"), hardlink("bin/sh", "bin/busybox"), hardlink("bin/ash", "bin/sh")},
			},
			expected: map[string]string{"bin/busybox": "ELF", "bin/sh": "ELF", "bin/ash": "ELF"},
		},
		{
			name: "symlinks and directories are not files",
			layers: [][]tarEntry{
				{dir("etc"), symlink("etc/os-release", "../usr/lib/os-release"), file("usr/lib/os-release", "ID=x")},
			},
			expected: map[string]string{"usr/lib/os-release": "ID=x"},
		},
		{
			name: "names with ./ prefix and leading slash are normalized",
			layers: [][]tarEntry{
				{file("./etc/a", "a"), file("/etc/b", "b")},
			},
			expected: map[string]string{"etc/a": "a", "etc/b": "b"},
		},
		{
			name: "gzip compressed layer is accepted",
			layers: [][]tarEntry{
				{file("etc/a", "a")},
			},
			gzipLast: true,
			expected: map[string]string{"etc/a": "a"},
		},
		{
			name:     "no manifest",
			layers:   [][]tarEntry{{file("etc/a", "a")}},
			manifest: "-",
			err:      ErrNoManifest,
		},
		{
			name:     "layer missing",
			layers:   [][]tarEntry{{file("etc/a", "a")}},
			manifest: `[{"Config":"abc123.json","RepoTags":["localhost/app:latest"],"Layers":["missing.tar"]}]`,
			err:      ErrLayerMissing,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Layers are stored under "<diffid>.tar" and referenced through "<id>/layer.tar" symlinks, written in
			// reverse order to make sure the manifest, not the stream order, decides.
			var entries []archiveEntry
			var layerNames []string
			for i, layer := range testCase.layers {
				data := writeTar(t, layer)
				if testCase.gzipLast && i == len(testCase.layers)-1 {
					data = gzipped(t, data)
				}
				diffName := strings.Repeat("a", 3) + string(rune('0'+i)) + ".tar"
				layerDir := "layer" + string(rune('0'+i))
				entries = append(entries,
					archiveEntry{name: diffName, data: data},
					archiveEntry{name: layerDir + "/layer.tar", linkname: "../" + diffName},
					archiveEntry{name: layerDir + "/VERSION", data: []byte("1.0")},
				)
				layerNames = append(layerNames, `"`+layerDir+`/layer.tar"`)
			}
			slices.Reverse(entries)
			entries = append(entries, archiveEntry{name: config, data: []byte(`{"architecture":"amd64"}`)})

			manifest := testCase.manifest
			switch manifest {
			case "":
				manifest = `[{"Config":"` + config + `","RepoTags":["localhost/app:latest"],"Layers":[` + strings.Join(layerNames, ",") + `]}]`
			case "-":
				manifest = ""
			}

			capture := testCase.capture
			if capture == nil {
				capture = captureText
			}
			image, err := Read(bytes.NewReader(writeArchive(t, entries, manifest)), testCase.reference, capture)
			if testCase.err != nil {
				if !errors.Is(err, testCase.err) {
					t.Fatalf("expected error %v, got %v", testCase.err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			contents := resolvedContents(image)
			if len(contents) != len(testCase.expected) {
				t.Fatalf("expected files %v, got %v", testCase.expected, contents)
			}
			for filePath, expected := range testCase.expected {
				if contents[filePath] != expected {
					t.Errorf("%s: expected %q, got %q", filePath, expected, contents[filePath])
				}
			}
			for filePath, expected := range testCase.layer {
				if image.Files[filePath].Layer != expected {
					t.Errorf("%s: expected layer %d, got %d", filePath, expected, image.Files[filePath].Layer)
				}
			}
			if testCase.id != "" && image.Id != testCase.id {
				t.Errorf("expected id %q, got %q", testCase.id, image.Id)
			}
			if !slices.Equal(image.RepoTags, []string{"localhost/app:latest"}) {
				t.Errorf("unexpected repo tags: %v", image.RepoTags)
			}
			if len(image.Layers) != len(testCase.layers) {
				t.Errorf("expected %d layers, got %v", len(testCase.layers), image.Layers)
			}
		})
	}
}

func TestReadEmptyLayer(t *testing.T) {
	t.Parallel()

	// An empty layer is a tar holding only the end-of-archive marker: 1024 zero bytes.
	layer := writeTar(t, []tarEntry{file("etc/a", "a")})
	entries := []archiveEntry{
		{name: "aaa.tar", data: layer},
		{name: "empty.tar", data: make([]byte, 1024)},
		{name: "cfg.json", data: []byte(`{}`)},
	}
	manifest := `[{"Config":"cfg.json","RepoTags":["x:y"],"Layers":["aaa.tar","empty.tar"]}]`

	image, err := Read(bytes.NewReader(writeArchive(t, entries, manifest)), "", captureText)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolvedContents(image)["etc/a"] != "a" || len(image.Layers) != 2 || !slices.Equal(image.LayerDigests, []string{"sha256:aaa", "sha256:empty"}) {
		t.Errorf("unexpected image: %+v", image)
	}
}

func TestReadLayerNamedWithoutTarSuffix(t *testing.T) {
	t.Parallel()

	layer := writeTar(t, []tarEntry{file("etc/a", "a")})
	entries := []archiveEntry{{name: "blobs/sha256/abcdef", data: layer}, {name: "blobs/sha256/cfg", data: []byte(`{"rootfs":{"diff_ids":["sha256:abcdef"]}}`)}}
	manifest := `[{"Config":"blobs/sha256/cfg","RepoTags":["x:y"],"Layers":["blobs/sha256/abcdef"]}]`

	image, err := Read(bytes.NewReader(writeArchive(t, entries, manifest)), "", captureText)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolvedContents(image)["etc/a"] != "a" || !slices.Equal(image.LayerDigests, []string{"sha256:abcdef"}) {
		t.Errorf("unexpected image: %+v", image)
	}
}

func TestReadSelectsImageByReference(t *testing.T) {
	t.Parallel()

	layerA := writeTar(t, []tarEntry{file("etc/name", "a")})
	layerB := writeTar(t, []tarEntry{file("etc/name", "b")})
	entries := []archiveEntry{
		{name: "aaa.tar", data: layerA},
		{name: "bbb.tar", data: layerB},
		{name: "cfga.json", data: []byte(`{}`)},
		{name: "cfgb.json", data: []byte(`{}`)},
	}
	manifest := `[
		{"Config":"cfga.json","RepoTags":["docker.io/library/a:1"],"Layers":["aaa.tar"]},
		{"Config":"cfgb.json","RepoTags":["docker.io/library/b:2"],"Layers":["bbb.tar"]}
	]`

	testCases := []struct {
		name      string
		reference string
		expected  string
		err       error
	}{
		{name: "full reference", reference: "docker.io/library/b:2", expected: "b"},
		{name: "short reference", reference: "a:1", expected: "a"},
		{name: "no reference", reference: "", err: ErrAmbiguousManifest},
		{name: "unknown reference", reference: "c:3", err: ErrAmbiguousManifest},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			image, err := Read(bytes.NewReader(writeArchive(t, entries, manifest)), testCase.reference, captureText)
			if testCase.err != nil {
				if !errors.Is(err, testCase.err) {
					t.Fatalf("expected error %v, got %v", testCase.err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := resolvedContents(image)["etc/name"]; got != testCase.expected {
				t.Errorf("expected image %q, got %q", testCase.expected, got)
			}
		})
	}
}

func TestReadCaptureSeesHeaderAndPeekableReader(t *testing.T) {
	t.Parallel()

	layer := writeTar(t, []tarEntry{file("usr/bin/app", "\x7fELF-payload"), file("etc/skip", "nope")})
	entries := []archiveEntry{{name: "aaa.tar", data: layer}, {name: "cfg.json", data: []byte(`{}`)}}
	manifest := `[{"Config":"cfg.json","RepoTags":["x:y"],"Layers":["aaa.tar"]}]`

	image, err := Read(bytes.NewReader(writeArchive(t, entries, manifest)), "", func(filePath string, header *tar.Header, reader *bufio.Reader) (any, error) {
		magic, err := reader.Peek(4)
		if err != nil {
			return nil, err
		}
		if string(magic) != "\x7fELF" {
			return nil, nil
		}
		if header.Size != int64(len("\x7fELF-payload")) {
			t.Errorf("unexpected header size %d", header.Size)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		return len(data), nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(image.Files) != 1 || image.Files["usr/bin/app"] == nil || image.Files["usr/bin/app"].Payload != len("\x7fELF-payload") {
		t.Errorf("unexpected files: %+v", image.Files)
	}
}

var errCaptureFailed = errors.New("capture failed")

func TestReadCaptureErrorPropagates(t *testing.T) {
	t.Parallel()

	layer := writeTar(t, []tarEntry{file("etc/a", "a")})
	entries := []archiveEntry{{name: "aaa.tar", data: layer}, {name: "cfg.json", data: []byte(`{}`)}}
	manifest := `[{"Config":"cfg.json","RepoTags":["x:y"],"Layers":["aaa.tar"]}]`
	captureErr := errCaptureFailed

	_, err := Read(bytes.NewReader(writeArchive(t, entries, manifest)), "", func(string, *tar.Header, *bufio.Reader) (any, error) {
		return nil, captureErr
	})
	if !errors.Is(err, captureErr) {
		t.Fatalf("expected the capture error, got %v", err)
	}
}

func TestNormalizePath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "plain", input: "etc/os-release", expected: "etc/os-release"},
		{name: "dot slash", input: "./etc/os-release", expected: "etc/os-release"},
		{name: "leading slash", input: "/etc/os-release", expected: "etc/os-release"},
		{name: "trailing slash", input: "etc/", expected: "etc"},
		{name: "root", input: "./", expected: ""},
		{name: "dot", input: ".", expected: ""},
		{name: "double slashes and dots", input: "usr//lib/../bin/./x", expected: "usr/bin/x"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizePath(testCase.input); got != testCase.expected {
				t.Errorf("expected %q, got %q", testCase.expected, got)
			}
		})
	}
}
