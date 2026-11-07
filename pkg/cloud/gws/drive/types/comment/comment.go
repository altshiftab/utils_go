package comment

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/types/user"
)

// Actions a reply can take on its comment, besides saying something.
const (
	ActionResolve = "resolve"
	ActionReopen  = "reopen"
)

// QuotedFileContent is the passage of the file a comment is anchored to, as it read when the comment was made.
type QuotedFileContent struct {
	MimeType string `json:"mimeType,omitzero"`
	Value    string `json:"value,omitzero"`
}

// Reply is one reply to a comment. Action is ActionResolve or ActionReopen for one that resolved or reopened it, which
// may say nothing else.
type Reply struct {
	Id           string     `json:"id,omitzero"`
	CreatedTime  string     `json:"createdTime,omitzero"`
	ModifiedTime string     `json:"modifiedTime,omitzero"`
	Author       *user.User `json:"author,omitzero"`
	// Content is the reply as text; Drive gives it as HTML too, in htmlContent, which is not read.
	Content string `json:"content,omitzero"`
	Action  string `json:"action,omitzero"`
	Deleted bool   `json:"deleted,omitzero"`
}

// Comment is a comment on a file, with its replies.
type Comment struct {
	Id           string     `json:"id,omitzero"`
	CreatedTime  string     `json:"createdTime,omitzero"`
	ModifiedTime string     `json:"modifiedTime,omitzero"`
	Author       *user.User `json:"author,omitzero"`
	Content      string     `json:"content,omitzero"`
	// QuotedFileContent is the passage the comment is on, where it is on one rather than on the file as a whole.
	QuotedFileContent *QuotedFileContent `json:"quotedFileContent,omitzero"`
	// Anchor is where in the file the comment is, in an opaque form of the editor's own.
	Anchor   string   `json:"anchor,omitzero"`
	Resolved bool     `json:"resolved,omitzero"`
	Deleted  bool     `json:"deleted,omitzero"`
	Replies  []*Reply `json:"replies,omitzero"`
}
