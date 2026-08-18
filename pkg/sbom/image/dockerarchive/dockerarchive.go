// Package dockerarchive reads container images in the docker-archive format (what `podman save --format
// docker-archive` and `docker save` write) as a stream, resolving the layers into the final root filesystem view of
// the files a caller chose to capture. It knows nothing about what the files mean.
package dockerarchive

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

var (
	ErrNoManifest        = errors.New("no manifest in archive")
	ErrAmbiguousManifest = errors.New("archive holds several images and none matches the reference")
	ErrLayerMissing      = errors.New("layer named by the manifest missing from archive")
	ErrUnsupportedLayer  = errors.New("unsupported layer compression")
	ErrEntryTooLarge     = errors.New("archive entry too large")
)

const (
	// maxJsonSize bounds the manifest and config files kept in memory.
	maxJsonSize = 16 << 20

	whiteoutPrefix = ".wh."
	opaqueWhiteout = ".wh..wh..opq"
)

// Capture is called for every regular file in every layer, with the reader positioned at the file's content (it
// ends at the file's end). It returns the payload to keep for the file, or nil to keep nothing. Files that are later
// deleted or replaced by an upper layer do not reach the resolved image.
type Capture func(filePath string, header *tar.Header, reader *bufio.Reader) (any, error)

// File is a captured file as it exists in the final image.
type File struct {
	// Path is the path inside the image, without a leading slash.
	Path string
	// Layer is the index, in the manifest's order, of the layer the file came from.
	Layer   int
	Payload any
}

// Image is the resolved view of one image in the archive.
type Image struct {
	// Id is the image ID, "sha256:<digest of the config>".
	Id       string
	RepoTags []string
	// Layers are the layer archive names in the manifest's order (lowest first).
	Layers []string
	// Files maps each captured path to the file as the top-most layer left it.
	Files map[string]*File
}

type manifestEntry struct {
	Config   string   `json:"Config"`
	RepoTags []string `json:"RepoTags"`
	Layers   []string `json:"Layers"`
}

// layerRecord is what one layer contributes: the captured files and the deletions it applies to the layers below.
type layerRecord struct {
	files     map[string]any
	hardlinks map[string]string
	// whiteouts are the paths (files or whole directories) the layer deletes.
	whiteouts []string
	// opaqueDirs are the directories whose lower-layer contents the layer hides.
	opaqueDirs []string
}

// Read reads a docker-archive stream and resolves the image it holds. When the archive holds several images, the one
// whose repo tags name the reference is chosen; an empty reference requires the archive to hold exactly one image.
func Read(reader io.Reader, reference string, capture Capture) (*Image, error) {
	if reader == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("reader"))
	}
	if capture == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("capture"))
	}

	// The manifest comes last in the stream and layers are not stored in layer order, so everything is recorded
	// first and resolved once the manifest is known.
	records := make(map[string]*layerRecord)
	jsons := make(map[string][]byte)
	links := make(map[string]string)

	entryReader := bufio.NewReaderSize(nil, 64*1024)
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("tar next: %w", err))
		}

		name := normalizePath(header.Name)
		if name == "" {
			continue
		}

		switch header.Typeflag {
		case tar.TypeReg:
			switch {
			case strings.HasSuffix(name, ".tar"):
				entryReader.Reset(tarReader)
				record, err := readLayer(entryReader, capture)
				if err != nil {
					return nil, altshiftErrors.New(fmt.Errorf("read layer (%s): %w", name, err), name)
				}
				records[name] = record
			case strings.HasSuffix(name, ".json"):
				if header.Size > maxJsonSize {
					return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s (%d bytes)", ErrEntryTooLarge, name, header.Size), name, header.Size)
				}
				// The archive is only ever read into memory, never extracted, so entry names cannot traverse anywhere.
				data, err := io.ReadAll(io.LimitReader(tarReader, maxJsonSize+1)) //nolint:gosec // G305: read into memory, not written to a path
				if err != nil {
					return nil, altshiftErrors.NewWithTrace(fmt.Errorf("read all (%s): %w", name, err), name)
				}
				jsons[name] = data
			}
		case tar.TypeSymlink:
			// Only recorded to look layers up by their alias; nothing is written to disk.
			links[name] = normalizePath(path.Join(path.Dir(name), header.Linkname)) //nolint:gosec // G305: name resolution in memory, no extraction
		}
	}

	manifestData, ok := jsons["manifest.json"]
	if !ok {
		return nil, altshiftErrors.NewWithTrace(ErrNoManifest)
	}
	var manifest []*manifestEntry
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal (manifest): %w", err), manifestData)
	}
	entry, err := selectManifestEntry(manifest, reference)
	if err != nil {
		return nil, err
	}

	image := &Image{
		Id:       "sha256:" + strings.TrimSuffix(path.Base(entry.Config), ".json"),
		RepoTags: entry.RepoTags,
		Layers:   entry.Layers,
		Files:    make(map[string]*File),
	}

	for i, layerName := range entry.Layers {
		name := normalizePath(layerName)
		record, ok := records[name]
		if !ok {
			// Layers may be referenced through "<id>/layer.tar" symlinks to the "<diffid>.tar" files.
			record, ok = records[links[name]]
		}
		if !ok || record == nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s", ErrLayerMissing, layerName), layerName)
		}
		applyLayer(image.Files, record, i)
	}

	return image, nil
}

// readLayer walks one layer archive, capturing files and recording deletions.
func readLayer(reader *bufio.Reader, capture Capture) (*layerRecord, error) {
	magic, err := reader.Peek(4)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("peek: %w", err))
	}
	var layerReader io.Reader = reader
	switch {
	case len(magic) >= 2 && magic[0] == 0x1f && magic[1] == 0x8b:
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("gzip new reader: %w", err))
		}
		defer gzipReader.Close()
		layerReader = gzipReader
	case len(magic) >= 4 && bytes.Equal(magic, []byte{0x28, 0xb5, 0x2f, 0xfd}):
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: zstd", ErrUnsupportedLayer))
	}

	record := &layerRecord{files: make(map[string]any), hardlinks: make(map[string]string)}
	entryReader := bufio.NewReaderSize(nil, 64*1024)
	tarReader := tar.NewReader(layerReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("tar next: %w", err))
		}

		filePath := normalizePath(header.Name)
		if filePath == "" {
			continue
		}
		dir, base := path.Split(filePath)
		dir = strings.TrimSuffix(dir, "/")

		switch {
		case base == opaqueWhiteout:
			record.opaqueDirs = append(record.opaqueDirs, dir)
			continue
		case strings.HasPrefix(base, whiteoutPrefix):
			record.whiteouts = append(record.whiteouts, path.Join(dir, strings.TrimPrefix(base, whiteoutPrefix)))
			continue
		}

		switch header.Typeflag {
		case tar.TypeReg:
			entryReader.Reset(tarReader)
			payload, err := capture(filePath, header, entryReader)
			if err != nil {
				return nil, altshiftErrors.New(fmt.Errorf("capture (%s): %w", filePath, err), filePath)
			}
			if payload != nil {
				record.files[filePath] = payload
			}
		case tar.TypeLink:
			record.hardlinks[filePath] = normalizePath(header.Linkname)
		}
	}

	// A hard link to a captured file is the same file under another name.
	for link, target := range record.hardlinks {
		for range len(record.hardlinks) + 1 {
			if payload, ok := record.files[target]; ok {
				record.files[link] = payload
				break
			}
			next, ok := record.hardlinks[target]
			if !ok {
				break
			}
			target = next
		}
	}

	return record, nil
}

// applyLayer folds a layer into the resolved view: its deletions hide what lower layers put there, then its files
// take their place.
func applyLayer(files map[string]*File, record *layerRecord, layer int) {
	for _, whiteout := range record.whiteouts {
		delete(files, whiteout)
		deletePrefix(files, whiteout+"/")
	}
	for _, dir := range record.opaqueDirs {
		if dir == "" {
			clear(files)
			continue
		}
		deletePrefix(files, dir+"/")
	}
	for filePath, payload := range record.files {
		files[filePath] = &File{Path: filePath, Layer: layer, Payload: payload}
	}
}

func deletePrefix(files map[string]*File, prefix string) {
	for filePath := range files {
		if strings.HasPrefix(filePath, prefix) {
			delete(files, filePath)
		}
	}
}

func selectManifestEntry(manifest []*manifestEntry, reference string) (*manifestEntry, error) {
	var entries []*manifestEntry
	for _, entry := range manifest {
		if entry != nil {
			entries = append(entries, entry)
		}
	}
	switch {
	case len(entries) == 0:
		return nil, altshiftErrors.NewWithTrace(empty_error.New("manifest"))
	case len(entries) == 1:
		return entries[0], nil
	}

	for _, entry := range entries {
		for _, tag := range entry.RepoTags {
			// Tags are stored fully qualified ("docker.io/library/alpine:3.24"); the reference may be short.
			if reference != "" && (tag == reference || strings.HasSuffix(tag, "/"+reference)) {
				return entry, nil
			}
		}
	}

	return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %q", ErrAmbiguousManifest, reference), reference)
}

// normalizePath turns a tar entry name into the clean, slash-less-prefixed path used throughout ("./etc/os-release"
// -> "etc/os-release"); the root itself becomes "".
func normalizePath(name string) string {
	name = strings.TrimPrefix(name, "/")
	name = strings.TrimPrefix(name, "./")
	name = path.Clean(name)
	if name == "." || name == "/" {
		return ""
	}
	return strings.TrimPrefix(name, "/")
}
