package tar

import (
	"archive/tar"
	"bytes"
	"errors"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelTarTypes "github.com/altshiftab/utils_go/pkg/tar/types"
	"io"
)

func MakeArchiveFromReader(reader io.Reader) (motmedelTarTypes.Archive, error) {
	if reader == nil {
		return nil, nil
	}

	archive := make(motmedelTarTypes.Archive)

	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, &motmedelErrors.Error{
				Message: "An error occurred when obtaining an entry in the tar archive.",
				Cause:   err,
			}
		}
		if header == nil {
			continue
		}

		content, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, &motmedelErrors.Error{
				Message: "An error occurred when reading header file content.",
				Cause:   err,
			}
		}

		archive[header.Name] = &motmedelTarTypes.Entry{Header: header, Content: content}
	}

	return archive, nil
}

func MakeArchiveFromData(data []byte) (motmedelTarTypes.Archive, error) {
	if len(data) == 0 {
		return nil, nil
	}

	archive, err := MakeArchiveFromReader(bytes.NewReader(data))
	if err != nil {
		return nil, &motmedelErrors.Error{
			Message: "An error occurred when making a tar map from a reader.",
			Cause:   err,
		}
	}

	return archive, nil
}

func MakeArchive(entries ...*motmedelTarTypes.Entry) motmedelTarTypes.Archive {
	archive := make(motmedelTarTypes.Archive)

	for _, entry := range entries {
		if entry == nil {
			continue
		}

		header := entry.Header
		if header == nil {
			continue
		}

		headerName := header.Name
		if headerName == "" {
			continue
		}

		switch header.Typeflag {
		case tar.TypeReg, tar.TypeDir:
			archive[headerName] = entry
		}
	}

	return archive
}
