// Package blob_storage is a client for block blobs in Azure Blob Storage, authorized with Entra
// tokens alone: it signs nothing with an account key and makes no shared access signatures, so
// that it works in accounts with shared-key access turned off.
//
// It covers what writing and checking backups takes: uploading a blob block by block, reading
// one back, and listing a container with its versions and metadata.
//
// A failed request carries its body in the error's HTTP context, a block of up to the upload's
// block size for Put Block: services logging errors should mask the request bodies of URLs with
// comp=block.
package blob_storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/azure/blob_storage/blob_storage_config"
	"github.com/altshiftab/utils_go/pkg/cloud/azure/blob_storage/list_blobs_config"
	"github.com/altshiftab/utils_go/pkg/cloud/azure/blob_storage/types/blob_list"
	"github.com/altshiftab/utils_go/pkg/cloud/azure/blob_storage/upload_config"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

const (
	// ApiVersion is the Blob service version every request is made under.
	ApiVersion = "2024-11-04"
	// DomainSuffix is what an account's blob endpoint is a subdomain of.
	DomainSuffix = "blob.core.windows.net"

	versionHeaderName     = "x-ms-version"
	dateHeaderName        = "x-ms-date"
	errorCodeHeaderName   = "x-ms-error-code"
	versionIdHeaderName   = "x-ms-version-id"
	metadataHeaderPrefix  = "x-ms-meta-"
	blobContentTypeHeader = "x-ms-blob-content-type"

	errorCodeBlobAlreadyExists = "BlobAlreadyExists"
	errorCodeConditionNotMet   = "ConditionNotMet"
)

// ErrBlobExists is an upload with IfAbsent to a name a current version already holds.
var ErrBlobExists = errors.New("the blob already exists")

// ServiceError is a request the service refused, with the error code it gave.
type ServiceError struct {
	StatusCode int
	// Code is the service's x-ms-error-code, as BlobAlreadyExists or BlobImmutableDueToPolicy.
	Code string
}

func (e *ServiceError) Error() string {
	return fmt.Sprintf("blob storage: status %d, code %q", e.StatusCode, e.Code)
}

type Client struct {
	baseUrl *url.URL
	config  *blob_storage_config.Config
}

func validAccountName(account string) bool {
	if len(account) < 3 || len(account) > 24 {
		return false
	}
	for i := range len(account) {
		if c := account[i]; (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}

	return true
}

// NewClient returns a client of the storage account's blob endpoint.
func NewClient(account string, options ...blob_storage_config.Option) (*Client, error) {
	if account == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("account"))
	}
	if !validAccountName(account) {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: an account name is 3 to 24 lowercase letters and digits: %q", altshiftErrors.ErrValidationError, account),
		)
	}

	config := blob_storage_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = &url.URL{Scheme: "https", Host: account + "." + DomainSuffix}
	}

	return &Client{baseUrl: baseUrl, config: config}, nil
}

// blobUrl is the URL of a blob. A "/" in its name stays a path separator, and everything else
// that needs it is escaped as the URL is written.
func (c *Client) blobUrl(container string, blob string, query url.Values) string {
	u := *c.baseUrl
	u.Path = strings.TrimSuffix(c.baseUrl.Path, "/") + "/" + container + "/" + blob
	u.RawPath = ""
	u.RawQuery = query.Encode()

	return u.String()
}

func (c *Client) containerUrl(container string, query url.Values) string {
	u := *c.baseUrl
	u.Path = strings.TrimSuffix(u.Path, "/") + "/" + container
	u.RawPath = ""
	u.RawQuery = query.Encode()

	return u.String()
}

// requestOptions are the client's fetch options, then the request's own: its method, its body,
// and the headers every request carries beside the ones given.
func (c *Client) requestOptions(method string, body []byte, headers map[string]string) []fetch_config.Option {
	allHeaders := map[string]string{
		versionHeaderName: ApiVersion,
		dateHeaderName:    time.Now().UTC().Format(http.TimeFormat),
	}
	maps.Copy(allHeaders, headers)

	options := append(slices.Clip(c.config.FetchOptions), fetch_config.WithMethod(method), fetch_config.WithHeaders(allHeaders))
	if body != nil {
		options = append(options, fetch_config.WithBody(body))
	}

	return options
}

// serviceError turns a refused request's response into a ServiceError beside the fetch error,
// so that a caller can tell the service's reasons apart.
func serviceError(response *http.Response, err error) error {
	if response == nil {
		return err
	}
	if _, ok := errors.AsType[*altshiftHttpErrors.Non2xxStatusCodeError](err); !ok {
		return err
	}

	return errors.Join(err, &ServiceError{StatusCode: response.StatusCode, Code: response.Header.Get(errorCodeHeaderName)})
}

// blockId is the id of the block at index of one upload. The service keeps uncommitted blocks by
// blob name and id, so two uploads to one name that numbered their blocks alike would overwrite
// each other's: each upload's ids carry a random prefix of its own. Every id of a blob must be as
// long as the others, so the index is written at a fixed width.
func blockId(upload string, index int) string {
	return base64.StdEncoding.EncodeToString(fmt.Appendf(nil, "%s-%06d", upload, index))
}

// fill reads from src until buffer is full or src ends, and returns how much it read. Only a plain
// io.EOF is src ending: any other error, one that wraps io.EOF or io.ErrUnexpectedEOF included,
// is src failing, whatever was read before it.
func fill(src io.Reader, buffer []byte) (int, bool, error) {
	n := 0
	for n < len(buffer) {
		read, err := src.Read(buffer[n:])
		n += read
		if err == io.EOF { //nolint:errorlint // Only the reader's own end, not an error that wraps it.
			return n, true, nil
		}
		if err != nil {
			return n, false, err
		}
	}

	return n, false, nil
}

// PutBlock stages a block of a blob, to be committed by PutBlockList.
func (c *Client) PutBlock(ctx context.Context, container string, blob string, id string, data []byte) error {
	if container == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("container"))
	}
	if blob == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("blob"))
	}
	if id == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("block id"))
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context err: %w", err)
	}

	urlString := c.blobUrl(container, blob, url.Values{"comp": {"block"}, "blockid": {id}})
	// Never nil: an empty block is sent as an empty body rather than none.
	body := data
	if body == nil {
		body = []byte{}
	}
	response, _, err := altshiftHttpUtils.Fetch(ctx, urlString, c.requestOptions(http.MethodPut, body, nil)...)
	if err != nil {
		return altshiftErrors.New(fmt.Errorf("fetch: %w", serviceError(response, err)), container, blob)
	}

	return nil
}

type blockList struct {
	XMLName xml.Name `xml:"BlockList"`
	Latest  []string `xml:"Latest"`
}

// Commit is a blob as committed.
type Commit struct {
	ETag string
	// VersionId names the version the commit made, where versioning is on.
	VersionId string
}

// PutBlockList commits the staged blocks, in order, as the blob's new current version.
func (c *Client) PutBlockList(
	ctx context.Context,
	container string,
	blob string,
	ids []string,
	options ...upload_config.Option,
) (*Commit, error) {
	if container == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("container"))
	}
	if blob == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("blob"))
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	config := upload_config.New(options...)

	body, err := xml.Marshal(&blockList{Latest: ids})
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("xml marshal (block list): %w", err), container, blob)
	}

	headers := map[string]string{"Content-Type": "application/xml"}
	if config.IfAbsent {
		headers["If-None-Match"] = "*"
	}
	if config.ContentType != "" {
		headers[blobContentTypeHeader] = config.ContentType
	}
	for name, value := range config.Metadata {
		headers[metadataHeaderPrefix+name] = value
	}

	urlString := c.blobUrl(container, blob, url.Values{"comp": {"blocklist"}})
	response, _, err := altshiftHttpUtils.Fetch(ctx, urlString, c.requestOptions(http.MethodPut, append([]byte(xml.Header), body...), headers)...)
	if err != nil {
		wrapped := serviceError(response, err)
		if serviceErr, ok := errors.AsType[*ServiceError](wrapped); ok && config.IfAbsent &&
			(serviceErr.Code == errorCodeBlobAlreadyExists || serviceErr.Code == errorCodeConditionNotMet) {
			return nil, altshiftErrors.New(fmt.Errorf("%w: %w", ErrBlobExists, wrapped), container, blob)
		}
		return nil, altshiftErrors.New(fmt.Errorf("fetch: %w", wrapped), container, blob)
	}
	if response == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("http response"), container, blob)
	}

	return &Commit{ETag: response.Header.Get("ETag"), VersionId: response.Header.Get(versionIdHeaderName)}, nil
}

// Upload writes what src holds as a blob, a block at a time, holding one block in memory, and
// commits it once src is exhausted. A failure before the commit leaves the name as it was: the
// staged blocks are discarded by the service after a week.
func (c *Client) Upload(
	ctx context.Context,
	container string,
	blob string,
	src io.Reader,
	options ...upload_config.Option,
) (*Commit, error) {
	if src == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("src"))
	}

	if container == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("container"))
	}
	if blob == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("blob"))
	}

	config := upload_config.New(options...)
	if config.BlockSize < 1 || int64(config.BlockSize) > upload_config.MaxBlockSize {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: block size %d is out of range", altshiftErrors.ErrValidationError, config.BlockSize),
		)
	}

	uploadId := make([]byte, 8)
	if _, err := rand.Read(uploadId); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("rand read (upload id): %w", err))
	}
	upload := hex.EncodeToString(uploadId)

	buffer := make([]byte, config.BlockSize)
	var ids []string
	for {
		n, ended, readErr := fill(src, buffer)
		// A source that failed is not committed, nor is what it gave before failing staged: the
		// blob would be the start of something presented as all of it.
		if readErr != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("read: %w", readErr), container, blob)
		}
		if n > 0 {
			if len(ids) == upload_config.MaxBlocks {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: more than %d blocks of %d bytes", altshiftErrors.ErrValidationError, upload_config.MaxBlocks, config.BlockSize),
					container, blob,
				)
			}
			id := blockId(upload, len(ids))
			if err := c.PutBlock(ctx, container, blob, id, buffer[:n]); err != nil {
				return nil, fmt.Errorf("put block: %w", err)
			}
			ids = append(ids, id)
		}
		if ended {
			break
		}
	}

	commit, err := c.PutBlockList(ctx, container, blob, ids, options...)
	if err != nil {
		return nil, fmt.Errorf("put block list: %w", err)
	}

	return commit, nil
}

// Download is a blob being read.
type Download struct {
	// Body is the blob's content; the caller closes it.
	Body          io.ReadCloser
	ContentLength int64
	ContentType   string
	ETag          string
	VersionId     string
	Metadata      map[string]string
}

// GetBlob opens a blob for reading: its current version, or the version named.
func (c *Client) GetBlob(ctx context.Context, container string, blob string, versionId string) (*Download, error) {
	if container == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("container"))
	}
	if blob == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("blob"))
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	query := url.Values{}
	if versionId != "" {
		query.Set("versionid", versionId)
	}

	urlString := c.blobUrl(container, blob, query)
	options := append(c.requestOptions(http.MethodGet, nil, nil), fetch_config.WithSkipReadResponseBody(true))
	response, _, err := altshiftHttpUtils.Fetch(ctx, urlString, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch: %w", serviceError(response, err)), container, blob)
	}
	if response == nil || response.Body == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("http response body"), container, blob)
	}

	metadata := make(map[string]string)
	for name, values := range response.Header {
		if rest, found := strings.CutPrefix(strings.ToLower(name), metadataHeaderPrefix); found && len(values) > 0 {
			metadata[rest] = values[0]
		}
	}

	return &Download{
		Body:          response.Body,
		ContentLength: response.ContentLength,
		ContentType:   response.Header.Get("Content-Type"),
		ETag:          response.Header.Get("ETag"),
		VersionId:     response.Header.Get(versionIdHeaderName),
		Metadata:      metadata,
	}, nil
}

// ListBlobs lists one page of a container's blobs.
func (c *Client) ListBlobs(ctx context.Context, container string, options ...list_blobs_config.Option) (*blob_list.EnumerationResults, error) {
	if container == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("container"))
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	config := list_blobs_config.New(options...)

	query := url.Values{"restype": {"container"}, "comp": {"list"}}
	if config.Prefix != "" {
		query.Set("prefix", config.Prefix)
	}
	var include []string
	if config.IncludeVersions {
		include = append(include, "versions")
	}
	if config.IncludeMetadata {
		include = append(include, "metadata")
	}
	if len(include) > 0 {
		query.Set("include", strings.Join(include, ","))
	}
	if config.Marker != "" {
		query.Set("marker", config.Marker)
	}
	if config.MaxResults > 0 {
		query.Set("maxresults", strconv.Itoa(config.MaxResults))
	}

	urlString := c.containerUrl(container, query)
	response, body, err := altshiftHttpUtils.Fetch(ctx, urlString, c.requestOptions(http.MethodGet, nil, nil)...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch: %w", serviceError(response, err)), container)
	}

	var results blob_list.EnumerationResults
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&results); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("xml decode (enumeration results): %w", err), container)
	}

	return &results, nil
}

// ListAllBlobs lists every page of a container's blobs, following the markers.
func (c *Client) ListAllBlobs(ctx context.Context, container string, options ...list_blobs_config.Option) ([]*blob_list.Blob, error) {
	var (
		blobs  []*blob_list.Blob
		marker string
	)
	for {
		page, err := c.ListBlobs(ctx, container, append(slices.Clip(options), list_blobs_config.WithMarker(marker))...)
		if err != nil {
			return nil, fmt.Errorf("list blobs: %w", err)
		}
		if page.Blobs != nil {
			blobs = append(blobs, page.Blobs.Blob...)
		}
		if page.NextMarker == "" {
			return blobs, nil
		}
		marker = page.NextMarker
	}
}
