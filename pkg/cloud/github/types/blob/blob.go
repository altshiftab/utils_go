// Package blob holds a file's contents as the GitHub API reports them.
package blob

import (
	"encoding/base64"
	"fmt"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

// Base64Encoding is what the API names the encoding it sends contents in.
const Base64Encoding = "base64"

type Blob struct {
	Sha      string `json:"sha,omitzero"`
	Url      string `json:"url,omitzero"`
	Content  string `json:"content,omitzero"`
	Encoding string `json:"encoding,omitzero"`
	Size     int    `json:"size,omitzero"`
}

// Data returns the file's contents.
func (blob *Blob) Data() ([]byte, error) {
	if blob == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("blob"))
	}

	// An encoding that is not the one expected would otherwise be decoded as
	// though it were, producing bytes that are not the file.
	if blob.Encoding != "" && blob.Encoding != Base64Encoding {
		return nil, altshiftErrors.NewWithTrace(ErrUnexpectedEncoding, blob.Encoding)
	}

	// The API wraps encoded contents across lines, which the decoder rejects.
	content := strings.NewReplacer("\n", "", "\r", "").Replace(blob.Content)

	data, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("base64 decode string: %w", err), blob.Content)
	}

	return data, nil
}
