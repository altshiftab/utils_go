package types

import (
	"archive/tar"
	"bytes"
	"fmt"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"io"
	"path/filepath"
	"strings"
)

type Entry struct {
	Header  *tar.Header
	Content []byte
}

type Archive map[string]*Entry

func (archive Archive) Bytes() ([]byte, error) {
	if len(archive) == 0 {
		return nil, nil
	}

	var outputBuffer bytes.Buffer
	tarWriter := tar.NewWriter(&outputBuffer)
	defer tarWriter.Close()

	for _, entry := range archive {
		if entry == nil {
			continue
		}

		header := entry.Header
		if header == nil {
			continue
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, &motmedelErrors.Error{
				Message: "An error occurred when writing a tar header.",
				Cause:   err,
				Input:   header,
			}
		}

		content := entry.Content
		if len(content) == 0 {
			continue
		}

		if _, err := io.Copy(tarWriter, bytes.NewReader(content)); err != nil {
			return nil, &motmedelErrors.Error{
				Message: "An error occurred when writing tar header file content.",
				Cause:   err,
			}
		}
	}

	if err := tarWriter.Close(); err != nil {
		return nil, &motmedelErrors.Error{
			Message: "An error occurred when flushing the tar writer.",
			Cause:   err,
		}
	}

	return outputBuffer.Bytes(), nil
}

func (archive Archive) Filter(patterns ...string) (Archive, error) {
	newArchive := make(Archive)

	for path, entry := range archive {
		var ignored bool

		for _, pattern := range patterns {
			if pattern == "" {
				continue
			}

			var isNegatePattern bool

			if strings.HasPrefix(pattern, "!") {
				isNegatePattern = true
				pattern = pattern[1:]
			}

			patternMatches, err := filepath.Match(pattern, path)
			if err != nil {
				return nil, &motmedelErrors.Error{
					Message: "An error occurred when matching a path.",
					Cause:   err,
					Input:   []any{pattern, path},
				}
			}

			if patternMatches {
				ignored = !isNegatePattern
			}
		}

		if !ignored {
			newArchive[path] = entry
		}
	}

	return newArchive, nil
}

func (archive Archive) SetDirectory(directory string) (Archive, bool) {
	if len(archive) == 0 {
		return nil, false
	}

	var ok bool

	slashSuffixedDirectoryPath := fmt.Sprintf("%s/", directory)

	newArchive := make(Archive)

	for path, entry := range archive {
		if entry == nil {
			continue
		}

		header := entry.Header
		if header == nil {
			continue
		}

		typeFlag := header.Typeflag
		if typeFlag != tar.TypeReg && typeFlag != tar.TypeDir {
			continue
		}

		if strings.HasPrefix(path, slashSuffixedDirectoryPath) {
			ok = true

			if path != slashSuffixedDirectoryPath {
				newName := path[len(slashSuffixedDirectoryPath):]
				entry.Header.Name = newName

				newArchive[newName] = entry
			}
		}
	}

	return newArchive, ok
}

func (archive Archive) AddBasicFile(path string, content []byte) {
	cleanPath := filepath.Clean(path)
	archive[cleanPath] = &Entry{
		Header:  &tar.Header{Name: cleanPath, Size: int64(len(content))},
		Content: content,
	}
}
