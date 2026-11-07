package drive

import (
	"context"
	"net/url"
	"strconv"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/list_comments_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/types/comment_list"
)

// commentFields are the fields of a page of comments the client reads, which Drive needs named: it refuses a listing
// of comments without them.
const commentFields = "nextPageToken,comments(id,createdTime,modifiedTime,author(displayName,emailAddress,me),content," +
	"quotedFileContent,anchor,resolved,deleted,replies(id,createdTime,modifiedTime,author(displayName,emailAddress,me)," +
	"content,action,deleted))"

// ListCommentsPage retrieves one page of a file's comments, each with its replies, beginning at the options' page
// token.
func (c *Client) ListCommentsPage(
	ctx context.Context,
	fileId string,
	options ...list_comments_config.Option,
) (*comment_list.CommentList, error) {
	if fileId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("file id"))
	}

	listCommentsConfig := list_comments_config.New(options...)

	fields := listCommentsConfig.Fields
	if fields == "" {
		fields = commentFields
	}
	// Not the shared query: a listing of comments takes no supportsAllDrives, as it reaches a file by its id alone.
	query := url.Values{"fields": {fields}}
	if listCommentsConfig.PageSize > 0 {
		query.Set("pageSize", strconv.Itoa(listCommentsConfig.PageSize))
	}
	if listCommentsConfig.PageToken != "" {
		query.Set("pageToken", listCommentsConfig.PageToken)
	}
	if listCommentsConfig.IncludeDeleted {
		query.Set("includeDeleted", "true")
	}

	return rest.GetJson[comment_list.CommentList](
		ctx,
		c.urlString(query, append(filePath(fileId), "comments")...),
		c.fetchOptions(listCommentsConfig.FetchOptions),
	)
}
