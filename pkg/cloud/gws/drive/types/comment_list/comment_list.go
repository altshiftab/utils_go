package comment_list

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/types/comment"
)

// CommentList is one page of a file's comments.
type CommentList struct {
	Comments      []*comment.Comment `json:"comments,omitzero"`
	NextPageToken string             `json:"nextPageToken,omitzero"`
}
