package file

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/types/user"
)

// The MIME types Drive gives its own kinds of file, which have no content of
// their own to download: a Google Doc, Sheet or Slides is read by exporting it to
// another type, and a folder or a shortcut not at all.
const (
	MimeTypeFolder       = "application/vnd.google-apps.folder"
	MimeTypeShortcut     = "application/vnd.google-apps.shortcut"
	MimeTypeDocument     = "application/vnd.google-apps.document"
	MimeTypeSpreadsheet  = "application/vnd.google-apps.spreadsheet"
	MimeTypePresentation = "application/vnd.google-apps.presentation"
	MimeTypeDrawing      = "application/vnd.google-apps.drawing"
	// MimeTypeGoogleAppsPrefix begins the MIME type of every kind of file Drive
	// keeps in a form of its own.
	MimeTypeGoogleAppsPrefix = "application/vnd.google-apps."
)

// File is a file in Drive, or a folder, which Drive keeps as a file too.
//
// Drive answers with few of these fields unless the request names the ones it
// wants, so a field left empty may have been not asked for rather than empty.
type File struct {
	Id          string   `json:"id,omitzero"`
	Name        string   `json:"name,omitzero"`
	MimeType    string   `json:"mimeType,omitzero"`
	Description string   `json:"description,omitzero"`
	Parents     []string `json:"parents,omitzero"`
	// Trashed and Starred are pointers so that an update can set them false,
	// which is how a file is taken out of the trash.
	Trashed *bool `json:"trashed,omitzero"`
	Starred *bool `json:"starred,omitzero"`
	// Size is the content's size in bytes; a file Drive keeps in a form of its
	// own has none.
	Size         int64  `json:"size,omitzero,string"`
	Md5Checksum  string `json:"md5Checksum,omitzero"`
	CreatedTime  string `json:"createdTime,omitzero"`
	ModifiedTime string `json:"modifiedTime,omitzero"`
	WebViewLink  string `json:"webViewLink,omitzero"`
	// DriveId is the shared drive the file is in, and empty for one in My Drive.
	DriveId           string            `json:"driveId,omitzero"`
	Version           int64             `json:"version,omitzero,string"`
	HeadRevisionId    string            `json:"headRevisionId,omitzero"`
	Owners            []*user.User      `json:"owners,omitzero"`
	LastModifyingUser *user.User        `json:"lastModifyingUser,omitzero"`
	ShortcutDetails   *ShortcutDetails  `json:"shortcutDetails,omitzero"`
	ExportLinks       map[string]string `json:"exportLinks,omitzero"`
	Capabilities      map[string]bool   `json:"capabilities,omitzero"`
}

// ShortcutDetails says what a shortcut points at.
type ShortcutDetails struct {
	TargetId       string `json:"targetId,omitzero"`
	TargetMimeType string `json:"targetMimeType,omitzero"`
}
