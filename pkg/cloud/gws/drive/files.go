package drive

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/get_file_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/list_files_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/types/about"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/types/file"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/types/file_list"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/drive/update_file_config"
)

// MaxSimpleUploadBytes is the most content a create or an update sends in one
// request, which is what Drive takes that way; more goes by a resumable upload,
// which this client does not make.
const MaxSimpleUploadBytes = 5 * 1024 * 1024

func filePath(fileId string) []string {
	return []string{"files", fileId}
}

func (c *Client) uploadUrlString(query url.Values, segments ...string) string {
	return rest.Url(c.uploadBaseUrl, query, segments...)
}

// fieldsQuery is the query every call answering with a file starts from.
func (c *Client) fieldsQuery(fields string) url.Values {
	query := c.newQuery()
	if fields != "" {
		query.Set("fields", fields)
	}
	return query
}

// ListFilesPage retrieves one page of the files matching the options, beginning
// at the options' page token.
func (c *Client) ListFilesPage(ctx context.Context, options ...list_files_config.Option) (*file_list.FileList, error) {
	listFilesConfig := list_files_config.New(options...)

	query := c.fieldsQuery(listFilesConfig.Fields)
	if listFilesConfig.Query != "" {
		query.Set("q", listFilesConfig.Query)
	}
	if listFilesConfig.PageSize > 0 {
		query.Set("pageSize", strconv.Itoa(listFilesConfig.PageSize))
	}
	if listFilesConfig.PageToken != "" {
		query.Set("pageToken", listFilesConfig.PageToken)
	}
	if listFilesConfig.OrderBy != "" {
		query.Set("orderBy", listFilesConfig.OrderBy)
	}
	if listFilesConfig.Corpora != "" {
		query.Set("corpora", listFilesConfig.Corpora)
	}
	if listFilesConfig.IncludeItemsFromAllDrives {
		query.Set("includeItemsFromAllDrives", "true")
	}

	return rest.GetJson[file_list.FileList](
		ctx,
		c.urlString(query, "files"),
		c.fetchOptions(listFilesConfig.FetchOptions),
	)
}

// GetFile retrieves a file's metadata.
func (c *Client) GetFile(ctx context.Context, fileId string, options ...get_file_config.Option) (*file.File, error) {
	if fileId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("file id"))
	}

	getFileConfig := get_file_config.New(options...)

	return rest.GetJson[file.File](
		ctx,
		c.urlString(c.fieldsQuery(getFileConfig.Fields), filePath(fileId)...),
		c.fetchOptions(getFileConfig.FetchOptions),
	)
}

// GetAbout is what Drive says of the user the request is made as: their name and address, as Drive shows them.
func (c *Client) GetAbout(ctx context.Context, options ...fetch_config.Option) (*about.About, error) {
	return rest.GetJson[about.About](ctx, c.urlString(url.Values{"fields": {"user(displayName,emailAddress)"}}, "about"), c.fetchOptions(options))
}

// DownloadFile retrieves a file's content, with the content type Drive serves it
// as. A file Drive keeps in a form of its own -- a Doc, a Sheet -- has no content
// to download and is refused by Drive; ExportFile reads one.
//
// The content is read whole into memory, so a caller wanting a bound reads the
// file's size first.
func (c *Client) DownloadFile(ctx context.Context, fileId string, options ...fetch_config.Option) ([]byte, string, error) {
	if fileId == "" {
		return nil, "", altshiftErrors.NewWithTrace(empty_error.New("file id"))
	}

	query := c.newQuery()
	query.Set("alt", "media")

	return rest.GetBytes(ctx, c.urlString(query, filePath(fileId)...), c.fetchOptions(options))
}

// ExportFile retrieves a file Drive keeps in a form of its own converted to the
// MIME type given: a Doc as text/plain, text/markdown or a PDF, a Sheet as
// text/csv (its first sheet alone). Drive refuses an export of more than ten
// megabytes.
func (c *Client) ExportFile(ctx context.Context, fileId string, mimeType string, options ...fetch_config.Option) ([]byte, error) {
	if fileId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("file id"))
	}
	if mimeType == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("mime type"))
	}

	query := url.Values{}
	query.Set("mimeType", mimeType)

	content, _, err := rest.GetBytes(ctx, c.urlString(query, append(filePath(fileId), "export")...), c.fetchOptions(options))
	if err != nil {
		return nil, err
	}

	return content, nil
}

// multipartBody is a file's metadata and content as one multipart/related
// body, which is how Drive takes the two in one request, and its content type.
func multipartBody(f *file.File, content []byte, contentType string) ([]byte, string, error) {
	metadata, err := json.Marshal(f)
	if err != nil {
		return nil, "", altshiftErrors.NewWithTrace(fmt.Errorf("json marshal (file): %w", err))
	}

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	metadataPart, err := writer.CreatePart(textproto.MIMEHeader{"Content-Type": {"application/json; charset=UTF-8"}})
	if err != nil {
		return nil, "", altshiftErrors.NewWithTrace(fmt.Errorf("multipart writer create part (metadata): %w", err))
	}
	if _, err := metadataPart.Write(metadata); err != nil {
		return nil, "", altshiftErrors.NewWithTrace(fmt.Errorf("multipart part write (metadata): %w", err))
	}

	contentPart, err := writer.CreatePart(textproto.MIMEHeader{"Content-Type": {contentType}})
	if err != nil {
		return nil, "", altshiftErrors.NewWithTrace(fmt.Errorf("multipart writer create part (content): %w", err), contentType)
	}
	if _, err := contentPart.Write(content); err != nil {
		return nil, "", altshiftErrors.NewWithTrace(fmt.Errorf("multipart part write (content): %w", err))
	}

	if err := writer.Close(); err != nil {
		return nil, "", altshiftErrors.NewWithTrace(fmt.Errorf("multipart writer close: %w", err))
	}

	return buffer.Bytes(), "multipart/related; boundary=" + writer.Boundary(), nil
}

// checkContent refuses content this client cannot send: none at all is the
// metadata-only call's to make, and more than a simple upload takes is refused
// by Drive.
func checkContent(content []byte, contentType string) error {
	if len(content) > MaxSimpleUploadBytes {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: content of %d bytes, more than %d", altshiftErrors.ErrValidationError, len(content), MaxSimpleUploadBytes),
		)
	}
	if contentType == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("content type"))
	}
	return nil
}

// CreateFile creates a file from its metadata, and its content when content is
// given. A file with no content is how a folder is made (MimeType
// file.MimeTypeFolder), or an empty Doc.
//
// A MimeType of one of Drive's own kinds with content of another converts the
// content: text/plain or text/html made a Doc, text/csv a Sheet.
func (c *Client) CreateFile(
	ctx context.Context,
	f *file.File,
	content []byte,
	contentType string,
	options ...get_file_config.Option,
) (*file.File, error) {
	if f == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("file"))
	}
	if f.Name == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("file name"))
	}

	getFileConfig := get_file_config.New(options...)
	query := c.fieldsQuery(getFileConfig.Fields)

	if content == nil {
		return rest.SendJson[file.File](
			ctx,
			http.MethodPost,
			c.urlString(query, "files"),
			f,
			c.fetchOptions(getFileConfig.FetchOptions),
		)
	}

	if err := checkContent(content, contentType); err != nil {
		return nil, err
	}
	body, bodyContentType, err := multipartBody(f, content, contentType)
	if err != nil {
		return nil, err
	}
	query.Set("uploadType", "multipart")

	return rest.SendBytes[file.File](
		ctx,
		http.MethodPost,
		c.uploadUrlString(query, "files"),
		body,
		bodyContentType,
		c.fetchOptions(getFileConfig.FetchOptions),
	)
}

// updateQuery is an update's query: the fields to return, and the parents to
// move the file between.
func (c *Client) updateQuery(updateFileConfig *update_file_config.Config) url.Values {
	query := c.fieldsQuery(updateFileConfig.Fields)
	if len(updateFileConfig.AddParents) != 0 {
		query.Set("addParents", strings.Join(updateFileConfig.AddParents, ","))
	}
	if len(updateFileConfig.RemoveParents) != 0 {
		query.Set("removeParents", strings.Join(updateFileConfig.RemoveParents, ","))
	}
	return query
}

// UpdateFile changes the metadata fields f sets, leaving the rest: it renames a
// file, trashes it or takes it out of the trash, and, with the options' parents,
// moves it.
func (c *Client) UpdateFile(
	ctx context.Context,
	fileId string,
	f *file.File,
	options ...update_file_config.Option,
) (*file.File, error) {
	if fileId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("file id"))
	}
	if f == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("file"))
	}

	updateFileConfig := update_file_config.New(options...)

	return rest.SendJson[file.File](
		ctx,
		http.MethodPatch,
		c.urlString(c.updateQuery(updateFileConfig), filePath(fileId)...),
		f,
		c.fetchOptions(updateFileConfig.FetchOptions),
	)
}

// UpdateFileContent replaces a file's content, keeping its id, its name and its
// place: Drive keeps what it held before as a revision. Content of a type other
// than a Doc's or a Sheet's own is converted into one, as on creation.
func (c *Client) UpdateFileContent(
	ctx context.Context,
	fileId string,
	content []byte,
	contentType string,
	options ...update_file_config.Option,
) (*file.File, error) {
	if fileId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("file id"))
	}
	if content == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("content"))
	}
	if err := checkContent(content, contentType); err != nil {
		return nil, err
	}

	updateFileConfig := update_file_config.New(options...)
	query := c.updateQuery(updateFileConfig)
	query.Set("uploadType", "media")

	return rest.SendBytes[file.File](
		ctx,
		http.MethodPatch,
		c.uploadUrlString(query, filePath(fileId)...),
		content,
		contentType,
		c.fetchOptions(updateFileConfig.FetchOptions),
	)
}

// CopyFile copies a file, with the metadata f sets in place of the original's:
// a name, or parents to put the copy elsewhere. A folder cannot be copied.
func (c *Client) CopyFile(
	ctx context.Context,
	fileId string,
	f *file.File,
	options ...get_file_config.Option,
) (*file.File, error) {
	if fileId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("file id"))
	}
	if f == nil {
		f = &file.File{}
	}

	getFileConfig := get_file_config.New(options...)

	return rest.SendJson[file.File](
		ctx,
		http.MethodPost,
		c.urlString(c.fieldsQuery(getFileConfig.Fields), append(filePath(fileId), "copy")...),
		f,
		c.fetchOptions(getFileConfig.FetchOptions),
	)
}

// DeleteFile deletes a file for good, past the trash: what UpdateFile with Trashed does is what a user would want of
// their own file, and this is for a file a caller made for itself and has no more use for. A folder's files go with
// it.
func (c *Client) DeleteFile(ctx context.Context, fileId string, options ...fetch_config.Option) error {
	if fileId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("file id"))
	}

	return rest.Do(ctx, http.MethodDelete, c.urlString(c.newQuery(), filePath(fileId)...), c.fetchOptions(options))
}
