package tar

import (
	"archive/tar"
	"bytes"
	"testing"

	altshiftTarTypes "github.com/altshiftab/utils_go/pkg/tar/types"
)

func writeTar(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return buf.Bytes()
}

func TestMakeArchiveFromReader_Nil(t *testing.T) {
	t.Parallel()
	got, err := MakeArchiveFromReader(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil archive, got %v", got)
	}
}

func TestMakeArchiveFromReader_Multiple(t *testing.T) {
	t.Parallel()
	data := writeTar(t, map[string][]byte{
		"foo.txt": []byte("hello"),
		"bar.txt": []byte("world"),
	})

	archive, err := MakeArchiveFromReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(archive) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(archive))
	}

	if foo := archive["foo.txt"]; foo == nil || !bytes.Equal(foo.Content, []byte("hello")) {
		t.Fatalf("foo.txt mismatch: %+v", foo)
	}
	if bar := archive["bar.txt"]; bar == nil || !bytes.Equal(bar.Content, []byte("world")) {
		t.Fatalf("bar.txt mismatch: %+v", bar)
	}
}

func TestMakeArchiveFromData_Empty(t *testing.T) {
	t.Parallel()
	got, err := MakeArchiveFromData(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestMakeArchiveFromData_RoundTrip(t *testing.T) {
	t.Parallel()
	data := writeTar(t, map[string][]byte{"a": []byte("a-content")})
	archive, err := MakeArchiveFromData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry := archive["a"]; entry == nil || !bytes.Equal(entry.Content, []byte("a-content")) {
		t.Fatalf("entry mismatch: %+v", entry)
	}
}

func TestMakeArchiveFromData_Garbage(t *testing.T) {
	t.Parallel()
	if _, err := MakeArchiveFromData([]byte("not a tar")); err == nil {
		t.Fatal("expected error on garbage input")
	}
}

func TestMakeArchive_Filtering(t *testing.T) {
	t.Parallel()

	regular := &altshiftTarTypes.Entry{
		Header:  &tar.Header{Name: "file.txt", Typeflag: tar.TypeReg},
		Content: []byte("content"),
	}
	dir := &altshiftTarTypes.Entry{
		Header: &tar.Header{Name: "dir/", Typeflag: tar.TypeDir},
	}
	symlink := &altshiftTarTypes.Entry{
		Header: &tar.Header{Name: "link", Typeflag: tar.TypeSymlink},
	}
	noName := &altshiftTarTypes.Entry{
		Header: &tar.Header{Name: "", Typeflag: tar.TypeReg},
	}
	noHeader := &altshiftTarTypes.Entry{}

	archive := MakeArchive(regular, dir, symlink, noName, noHeader, nil)

	if len(archive) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(archive))
	}
	if _, ok := archive["file.txt"]; !ok {
		t.Fatal("expected file.txt in archive")
	}
	if _, ok := archive["dir/"]; !ok {
		t.Fatal("expected dir/ in archive")
	}
}
