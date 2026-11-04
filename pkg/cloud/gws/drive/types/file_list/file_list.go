package file_list

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/types/file"
)

// FileList is one page of a file listing.
type FileList struct {
	Files         []*file.File `json:"files,omitzero"`
	NextPageToken string       `json:"nextPageToken,omitzero"`
	// IncompleteSearch says Drive did not search everywhere it was asked to,
	// which happens when a listing spans every drive the user can reach: what
	// was found is not all there is.
	IncompleteSearch bool `json:"incompleteSearch,omitzero"`
}
