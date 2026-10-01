// Package drive is a client for the Microsoft Graph drive API: the files and
// folders of OneDrive accounts and SharePoint document libraries. Every item
// operation addresses a drive by id; GetUserDrive, GetSiteDrive and
// ListSiteDrives resolve one.
package drive

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/internal/graph"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"

	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/create_folder_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/drive_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/move_item_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/types/conflict_behavior"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/types/drive_item"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/types/drive_item/folder"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/types/drive_item/item_reference"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/types/graph_drive"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/types/site"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/drive/upload_config"
)

// RootItemId addresses a drive's root folder wherever an item id is taken.
const RootItemId = "root"

const (
	// SimpleUploadLimit is the largest file UploadFile sends in one request;
	// larger files go through an upload session.
	SimpleUploadLimit = 4 << 20

	uploadChunkUnit        = 320 << 10
	defaultUploadChunkSize = 10 * uploadChunkUnit
	conflictBehaviorMember = "@microsoft.graph.conflictBehavior"
	octetStreamContentType = "application/octet-stream"
	cancelUploadTimeout    = 10 * time.Second
)

var (
	// ErrNotFolder is returned when an item a folder was expected at is a file.
	ErrNotFolder = errors.New("not a folder")
	// ErrUnexpectedUploadStatus is returned when Graph completes an upload
	// session before its last chunk was sent.
	ErrUnexpectedUploadStatus = errors.New("unexpected upload chunk status")
)

type Client struct {
	baseUrl *url.URL
	config  *drive_config.Config
}

func NewClient(options ...drive_config.Option) *Client {
	config := drive_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = graph.DefaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/v1.0"
	u.RawPath = ""
	return &Client{baseUrl: &u, config: config}
}

func (c *Client) fetchOptions(options []fetch_config.Option) []fetch_config.Option {
	return append(slices.Clip(c.config.FetchOptions), options...)
}

func drivePath(driveId string) *graph.PathBuilder {
	return new(graph.PathBuilder).Literal("/drives/").Escaped(driveId)
}

func itemPath(driveId string, itemId string) *graph.PathBuilder {
	return drivePath(driveId).Literal("/items/").Escaped(itemId)
}

// itemByPathPath addresses an item by its drive-relative path; an empty path
// is the root folder.
func itemByPathPath(driveId string, path string) *graph.PathBuilder {
	builder := drivePath(driveId).Literal("/root")
	if len(graph.PathSegments(path)) != 0 {
		builder.Literal(":/").EscapedPath(path).Literal(":")
	}
	return builder
}

// childPath addresses the child called name of the folder parentId, which need
// not exist yet.
func childPath(driveId string, parentId string, name string) *graph.PathBuilder {
	return itemPath(driveId, parentId).Literal(":/").Escaped(name).Literal(":")
}

// Drive and site resolution

// GetDrive retrieves the drive identified by driveId.
func (c *Client) GetDrive(ctx context.Context, driveId string, options ...fetch_config.Option) (*graph_drive.Drive, error) {
	if driveId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("drive id"))
	}

	return rest.GetJson[graph_drive.Drive](ctx, drivePath(driveId).Url(c.baseUrl, nil), c.fetchOptions(options))
}

// GetUserDrive retrieves the OneDrive of the user identified by userId (an
// object id or a user principal name).
func (c *Client) GetUserDrive(ctx context.Context, userId string, options ...fetch_config.Option) (*graph_drive.Drive, error) {
	if userId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("user id"))
	}

	urlString := new(graph.PathBuilder).Literal("/users/").Escaped(userId).Literal("/drive").Url(c.baseUrl, nil)
	return rest.GetJson[graph_drive.Drive](ctx, urlString, c.fetchOptions(options))
}

// GetSiteDrive retrieves the default document library of the SharePoint site
// identified by siteId.
func (c *Client) GetSiteDrive(ctx context.Context, siteId string, options ...fetch_config.Option) (*graph_drive.Drive, error) {
	if siteId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("site id"))
	}

	urlString := new(graph.PathBuilder).Literal("/sites/").Escaped(siteId).Literal("/drive").Url(c.baseUrl, nil)
	return rest.GetJson[graph_drive.Drive](ctx, urlString, c.fetchOptions(options))
}

// ListSiteDrives retrieves all document libraries of the SharePoint site
// identified by siteId.
func (c *Client) ListSiteDrives(ctx context.Context, siteId string, options ...fetch_config.Option) ([]*graph_drive.Drive, error) {
	if siteId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("site id"))
	}

	firstUrl := new(graph.PathBuilder).Literal("/sites/").Escaped(siteId).Literal("/drives").Url(c.baseUrl, nil)
	return rest.ListPaginated(ctx, graph.NextLinkOr(firstUrl), graph.Extract[*graph_drive.Drive], c.fetchOptions(options))
}

// GetSiteByPath retrieves the SharePoint site at sitePath (e.g. "sites/Finance")
// on hostname (e.g. "contoso.sharepoint.com"); an empty sitePath is the root site.
func (c *Client) GetSiteByPath(ctx context.Context, hostname string, sitePath string, options ...fetch_config.Option) (*site.Site, error) {
	if hostname == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("hostname"))
	}

	builder := new(graph.PathBuilder).Literal("/sites/").Escaped(hostname)
	if len(graph.PathSegments(sitePath)) != 0 {
		builder.Literal(":/").EscapedPath(sitePath)
	}
	return rest.GetJson[site.Site](ctx, builder.Url(c.baseUrl, nil), c.fetchOptions(options))
}

// Item operations

// GetItem retrieves the item identified by itemId.
func (c *Client) GetItem(ctx context.Context, driveId string, itemId string, options ...fetch_config.Option) (*drive_item.DriveItem, error) {
	if driveId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("drive id"))
	}
	if itemId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("item id"))
	}

	return rest.GetJson[drive_item.DriveItem](ctx, itemPath(driveId, itemId).Url(c.baseUrl, nil), c.fetchOptions(options))
}

// GetItemByPath retrieves the item at the drive-relative path (e.g.
// "Reports/2026/q3.xlsx"); an empty path is the root folder.
func (c *Client) GetItemByPath(ctx context.Context, driveId string, path string, options ...fetch_config.Option) (*drive_item.DriveItem, error) {
	if driveId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("drive id"))
	}

	return rest.GetJson[drive_item.DriveItem](ctx, itemByPathPath(driveId, path).Url(c.baseUrl, nil), c.fetchOptions(options))
}

// ListChildren retrieves all items in the folder identified by folderId
// (RootItemId for the root). Subfolders are the items with Folder set.
func (c *Client) ListChildren(ctx context.Context, driveId string, folderId string, options ...fetch_config.Option) ([]*drive_item.DriveItem, error) {
	if driveId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("drive id"))
	}
	if folderId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("folder id"))
	}

	firstUrl := itemPath(driveId, folderId).Literal("/children").Url(c.baseUrl, nil)
	return rest.ListPaginated(ctx, graph.NextLinkOr(firstUrl), graph.Extract[*drive_item.DriveItem], c.fetchOptions(options))
}

type createFolderRequest struct {
	Name             string                             `json:"name"`
	Folder           *folder.Folder                     `json:"folder"`
	ConflictBehavior conflict_behavior.ConflictBehavior `json:"@microsoft.graph.conflictBehavior,omitzero"`
}

// CreateFolder creates a folder called name in the folder identified by
// parentId (RootItemId for the root). By default an existing item of that name
// fails the call with 409 Conflict.
func (c *Client) CreateFolder(
	ctx context.Context,
	driveId string,
	parentId string,
	name string,
	options ...create_folder_config.Option,
) (*drive_item.DriveItem, error) {
	if driveId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("drive id"))
	}
	if parentId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("parent id"))
	}
	if name == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("name"))
	}

	createFolderConfig := create_folder_config.New(options...)

	return rest.SendJson[drive_item.DriveItem](
		ctx,
		http.MethodPost,
		itemPath(driveId, parentId).Literal("/children").Url(c.baseUrl, nil),
		&createFolderRequest{Name: name, Folder: &folder.Folder{}, ConflictBehavior: createFolderConfig.ConflictBehavior},
		c.fetchOptions(createFolderConfig.FetchOptions),
	)
}

// EnsureFolderPath returns the folder at the drive-relative path, creating it
// and any missing ancestors. A folder created concurrently by someone else is
// used as found.
func (c *Client) EnsureFolderPath(ctx context.Context, driveId string, path string, options ...fetch_config.Option) (*drive_item.DriveItem, error) {
	if driveId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("drive id"))
	}

	// The common case is a path that already exists, which costs one request.
	item, err := c.GetItemByPath(ctx, driveId, path, options...)
	if err == nil {
		if item.Folder == nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s", ErrNotFolder, path))
		}
		return item, nil
	}
	if !graph.IsStatus(err, http.StatusNotFound) {
		return nil, altshiftErrors.New(fmt.Errorf("get item by path: %w", err), path)
	}

	parentId := RootItemId
	current := ""
	for _, segment := range graph.PathSegments(path) {
		current += "/" + segment

		item, err = c.GetItemByPath(ctx, driveId, current, options...)
		if graph.IsStatus(err, http.StatusNotFound) {
			item, err = c.CreateFolder(ctx, driveId, parentId, segment, create_folder_config.WithFetchOptions(options...))
			if graph.IsStatus(err, http.StatusConflict) {
				item, err = c.GetItemByPath(ctx, driveId, current, options...)
			}
		}
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("ensure folder: %w", err), current)
		}
		if item == nil {
			return nil, altshiftErrors.NewWithTrace(nil_error.New("drive item"), current)
		}
		if item.Folder == nil {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %s", ErrNotFolder, current),
			)
		}

		parentId = item.Id
	}

	if item == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("drive item"), path)
	}

	return item, nil
}

// UploadFile stores data as the file called name in the folder identified by
// parentId (RootItemId for the root). Files up to SimpleUploadLimit go in one
// request, larger ones through a resumable upload session. By default an
// existing item of that name fails the call with 409 Conflict; pass
// conflict_behavior.Replace to overwrite it as a new version.
func (c *Client) UploadFile(
	ctx context.Context,
	driveId string,
	parentId string,
	name string,
	data []byte,
	options ...upload_config.Option,
) (*drive_item.DriveItem, error) {
	if driveId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("drive id"))
	}
	if parentId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("parent id"))
	}
	if name == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("name"))
	}

	uploadConfig := upload_config.New(options...)

	if len(data) > SimpleUploadLimit {
		return c.uploadWithSession(ctx, driveId, parentId, name, data, uploadConfig)
	}

	contentType := uploadConfig.ContentType
	if contentType == "" {
		contentType = octetStreamContentType
	}

	query := url.Values{}
	if conflictBehavior := uploadConfig.ConflictBehavior; conflictBehavior != "" {
		query.Set(conflictBehaviorMember, string(conflictBehavior))
	}

	return rest.SendBytes[drive_item.DriveItem](
		ctx,
		http.MethodPut,
		childPath(driveId, parentId, name).Literal("/content").Url(c.baseUrl, query),
		data,
		contentType,
		c.fetchOptions(uploadConfig.FetchOptions),
	)
}

type uploadSessionItem struct {
	Name             string                             `json:"name,omitzero"`
	ConflictBehavior conflict_behavior.ConflictBehavior `json:"@microsoft.graph.conflictBehavior,omitzero"`
}

type createUploadSessionRequest struct {
	Item *uploadSessionItem `json:"item"`
}

type uploadSession struct {
	UploadUrl string `json:"uploadUrl"`
}

func (c *Client) uploadWithSession(
	ctx context.Context,
	driveId string,
	parentId string,
	name string,
	data []byte,
	uploadConfig *upload_config.Config,
) (*drive_item.DriveItem, error) {
	session, err := rest.SendJson[uploadSession](
		ctx,
		http.MethodPost,
		childPath(driveId, parentId, name).Literal("/createUploadSession").Url(c.baseUrl, nil),
		&createUploadSessionRequest{Item: &uploadSessionItem{ConflictBehavior: uploadConfig.ConflictBehavior}},
		c.fetchOptions(uploadConfig.FetchOptions),
	)
	if err != nil {
		return nil, fmt.Errorf("create upload session: %w", err)
	}
	if session.UploadUrl == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("upload url"))
	}

	uploadHttpClient := c.config.UploadHttpClient
	if uploadHttpClient == nil {
		uploadHttpClient = http.DefaultClient
	}
	// Appended last, so the unauthenticated client wins over any configured one.
	baseOptions := append(c.fetchOptions(uploadConfig.FetchOptions), fetch_config.WithHttpClient(uploadHttpClient))

	chunkSize := defaultUploadChunkSize
	if configured := c.config.UploadChunkSize; configured > 0 {
		// Graph rejects chunks that are not whole 320 KiB units, so round down.
		chunkSize = max(configured/uploadChunkUnit, 1) * uploadChunkUnit
	}

	contentType := uploadConfig.ContentType
	if contentType == "" {
		contentType = octetStreamContentType
	}

	total := len(data)
	for start := 0; start < total; start += chunkSize {
		end := min(start+chunkSize, total)

		chunkOptions := append(
			baseOptions[:len(baseOptions):len(baseOptions)],
			fetch_config.WithMethod(http.MethodPut),
			fetch_config.WithBody(data[start:end]),
			fetch_config.WithHeaders(map[string]string{
				"Content-Type":  contentType,
				"Content-Range": "bytes " + strconv.Itoa(start) + "-" + strconv.Itoa(end-1) + "/" + strconv.Itoa(total),
			}),
		)

		response, item, err := altshiftHttpUtils.FetchJson[*drive_item.DriveItem](ctx, session.UploadUrl, chunkOptions...)
		if err != nil {
			c.cancelUploadSession(ctx, session.UploadUrl, baseOptions)
			return nil, altshiftErrors.New(fmt.Errorf("fetch json (upload chunk): %w", err), start, end, total)
		}
		if response == nil {
			c.cancelUploadSession(ctx, session.UploadUrl, baseOptions)
			return nil, altshiftErrors.NewWithTrace(nil_error.New("http response"))
		}

		if end < total {
			if response.StatusCode != http.StatusAccepted {
				c.cancelUploadSession(ctx, session.UploadUrl, baseOptions)
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: %d before the last chunk", ErrUnexpectedUploadStatus, response.StatusCode),
					start, end, total,
				)
			}
			continue
		}

		if item == nil || item.Id == "" {
			return nil, altshiftErrors.NewWithTrace(nil_error.New("uploaded drive item"))
		}
		return item, nil
	}

	return nil, altshiftErrors.NewWithTrace(empty_error.New("data"))
}

// cancelUploadSession discards a failed session's uploaded bytes; failure is
// harmless, the session expiring by itself.
func (c *Client) cancelUploadSession(ctx context.Context, uploadUrl string, options []fetch_config.Option) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cancelUploadTimeout)
	defer cancel()
	_ = rest.Do(ctx, http.MethodDelete, uploadUrl, options)
}

type moveItemRequest struct {
	ParentReference *item_reference.ItemReference `json:"parentReference"`
	Name            string                        `json:"name,omitzero"`
}

// MoveItem moves the item identified by itemId into the folder identified by
// newParentId (RootItemId for the root) within the same drive.
func (c *Client) MoveItem(
	ctx context.Context,
	driveId string,
	itemId string,
	newParentId string,
	options ...move_item_config.Option,
) (*drive_item.DriveItem, error) {
	if driveId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("drive id"))
	}
	if itemId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("item id"))
	}
	if newParentId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("new parent id"))
	}

	moveItemConfig := move_item_config.New(options...)

	query := url.Values{}
	if conflictBehavior := moveItemConfig.ConflictBehavior; conflictBehavior != "" {
		query.Set(conflictBehaviorMember, string(conflictBehavior))
	}

	return rest.SendJson[drive_item.DriveItem](
		ctx,
		http.MethodPatch,
		itemPath(driveId, itemId).Url(c.baseUrl, query),
		&moveItemRequest{ParentReference: &item_reference.ItemReference{Id: newParentId}, Name: moveItemConfig.Name},
		c.fetchOptions(moveItemConfig.FetchOptions),
	)
}

// DeleteItem moves the item identified by itemId to the recycle bin.
func (c *Client) DeleteItem(ctx context.Context, driveId string, itemId string, options ...fetch_config.Option) error {
	if driveId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("drive id"))
	}
	if itemId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("item id"))
	}

	return rest.Do(ctx, http.MethodDelete, itemPath(driveId, itemId).Url(c.baseUrl, nil), c.fetchOptions(options))
}
