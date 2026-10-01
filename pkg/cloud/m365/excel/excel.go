// Package excel is a client for the Microsoft Graph workbook API, which edits
// .xlsx files stored in OneDrive or SharePoint in place. A workbook is
// addressed by its drive id and item id, as resolved with the drive package.
package excel

import (
	"context"
	"net/http"
	"net/url"
	"slices"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/internal/graph"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"

	"github.com/altshiftab/utils_go/pkg/cloud/m365/excel/add_table_rows_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/excel/excel_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/excel/request_config"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/excel/types/session"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/excel/types/table"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/excel/types/table_row"
	"github.com/altshiftab/utils_go/pkg/cloud/m365/excel/types/workbook_range"
)

const SessionIdHeader = "workbook-session-id"

type Client struct {
	baseUrl *url.URL
	config  *excel_config.Config
}

func NewClient(options ...excel_config.Option) *Client {
	config := excel_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = graph.DefaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/v1.0"
	u.RawPath = ""
	return &Client{baseUrl: &u, config: config}
}

// fetchOptions combines the client's options, the call's, and the session
// header when the call runs in a session.
func (c *Client) fetchOptions(sessionId string, options []fetch_config.Option) []fetch_config.Option {
	combined := append(slices.Clip(c.config.FetchOptions), options...)
	if sessionId != "" {
		combined = append(combined, fetch_config.WithHeaders(map[string]string{SessionIdHeader: sessionId}))
	}
	return combined
}

func validateWorkbook(driveId string, itemId string) error {
	if driveId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("drive id"))
	}
	if itemId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("item id"))
	}
	return nil
}

func workbookPath(driveId string, itemId string) *graph.PathBuilder {
	return new(graph.PathBuilder).
		Literal("/drives/").Escaped(driveId).
		Literal("/items/").Escaped(itemId).
		Literal("/workbook")
}

func tablePath(driveId string, itemId string, tableName string) *graph.PathBuilder {
	return workbookPath(driveId, itemId).Literal("/tables/").Escaped(tableName)
}

func rangePath(driveId string, itemId string, worksheet string, address string) *graph.PathBuilder {
	return workbookPath(driveId, itemId).
		Literal("/worksheets/").Escaped(worksheet).
		Literal("/range(address='").Escaped(graph.ODataString(address)).Literal("')")
}

// Sessions

type createSessionRequest struct {
	PersistChanges bool `json:"persistChanges"`
}

// CreateSession opens a workbook session. Running a series of calls in one is
// faster than running them sessionless; with persistChanges false their changes
// are discarded when it closes. Close it with CloseSession.
func (c *Client) CreateSession(
	ctx context.Context,
	driveId string,
	itemId string,
	persistChanges bool,
	options ...fetch_config.Option,
) (*session.Session, error) {
	if err := validateWorkbook(driveId, itemId); err != nil {
		return nil, err
	}

	return rest.SendJson[session.Session](
		ctx,
		http.MethodPost,
		workbookPath(driveId, itemId).Literal("/createSession").Url(c.baseUrl, nil),
		&createSessionRequest{PersistChanges: persistChanges},
		c.fetchOptions("", options),
	)
}

// CloseSession closes the session identified by sessionId.
func (c *Client) CloseSession(
	ctx context.Context,
	driveId string,
	itemId string,
	sessionId string,
	options ...fetch_config.Option,
) error {
	if err := validateWorkbook(driveId, itemId); err != nil {
		return err
	}
	if sessionId == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("session id"))
	}

	return rest.Do(
		ctx,
		http.MethodPost,
		workbookPath(driveId, itemId).Literal("/closeSession").Url(c.baseUrl, nil),
		c.fetchOptions(sessionId, options),
	)
}

// Tables

// ListTables retrieves the workbook's tables.
func (c *Client) ListTables(ctx context.Context, driveId string, itemId string, options ...request_config.Option) ([]*table.Table, error) {
	if err := validateWorkbook(driveId, itemId); err != nil {
		return nil, err
	}

	requestConfig := request_config.New(options...)
	firstUrl := workbookPath(driveId, itemId).Literal("/tables").Url(c.baseUrl, nil)

	return rest.ListPaginated(
		ctx,
		graph.NextLinkOr(firstUrl),
		graph.Extract[*table.Table],
		c.fetchOptions(requestConfig.SessionId, requestConfig.FetchOptions),
	)
}

type addTableRequest struct {
	Address    string `json:"address"`
	HasHeaders bool   `json:"hasHeaders"`
}

// AddTable creates a table over address, a range including its worksheet
// (e.g. "Sheet1!A1:D1"). With hasHeaders the range's first row becomes the
// header row; a header-only range makes an empty table to add rows to.
func (c *Client) AddTable(
	ctx context.Context,
	driveId string,
	itemId string,
	address string,
	hasHeaders bool,
	options ...request_config.Option,
) (*table.Table, error) {
	if err := validateWorkbook(driveId, itemId); err != nil {
		return nil, err
	}
	if address == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("address"))
	}

	requestConfig := request_config.New(options...)

	return rest.SendJson[table.Table](
		ctx,
		http.MethodPost,
		workbookPath(driveId, itemId).Literal("/tables/add").Url(c.baseUrl, nil),
		&addTableRequest{Address: address, HasHeaders: hasHeaders},
		c.fetchOptions(requestConfig.SessionId, requestConfig.FetchOptions),
	)
}

type addTableRowsRequest struct {
	Index  *int    `json:"index,omitzero"`
	Values [][]any `json:"values"`
}

// AddTableRows appends rows to the table identified by tableName (its name or
// id), growing it to fit. Each row needs one value per table column. Adding
// many rows in one call is much faster than one call per row.
func (c *Client) AddTableRows(
	ctx context.Context,
	driveId string,
	itemId string,
	tableName string,
	values [][]any,
	options ...add_table_rows_config.Option,
) (*table_row.TableRow, error) {
	if err := validateWorkbook(driveId, itemId); err != nil {
		return nil, err
	}
	if tableName == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("table name"))
	}
	if len(values) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("values"))
	}

	addTableRowsConfig := add_table_rows_config.New(options...)

	return rest.SendJson[table_row.TableRow](
		ctx,
		http.MethodPost,
		tablePath(driveId, itemId, tableName).Literal("/rows").Url(c.baseUrl, nil),
		&addTableRowsRequest{Index: addTableRowsConfig.Index, Values: values},
		c.fetchOptions(addTableRowsConfig.SessionId, addTableRowsConfig.FetchOptions),
	)
}

// ListTableRows retrieves the data rows of the table identified by tableName.
func (c *Client) ListTableRows(
	ctx context.Context,
	driveId string,
	itemId string,
	tableName string,
	options ...request_config.Option,
) ([]*table_row.TableRow, error) {
	if err := validateWorkbook(driveId, itemId); err != nil {
		return nil, err
	}
	if tableName == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("table name"))
	}

	requestConfig := request_config.New(options...)
	firstUrl := tablePath(driveId, itemId, tableName).Literal("/rows").Url(c.baseUrl, nil)

	return rest.ListPaginated(
		ctx,
		graph.NextLinkOr(firstUrl),
		graph.Extract[*table_row.TableRow],
		c.fetchOptions(requestConfig.SessionId, requestConfig.FetchOptions),
	)
}

// Ranges

// GetRange retrieves the cells at address (e.g. "A1:C10") on the worksheet
// identified by worksheet (its name or id).
func (c *Client) GetRange(
	ctx context.Context,
	driveId string,
	itemId string,
	worksheet string,
	address string,
	options ...request_config.Option,
) (*workbook_range.Range, error) {
	if err := validateWorkbook(driveId, itemId); err != nil {
		return nil, err
	}
	if worksheet == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("worksheet"))
	}
	if address == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("address"))
	}

	requestConfig := request_config.New(options...)

	return rest.GetJson[workbook_range.Range](
		ctx,
		rangePath(driveId, itemId, worksheet, address).Url(c.baseUrl, nil),
		c.fetchOptions(requestConfig.SessionId, requestConfig.FetchOptions),
	)
}

type updateRangeRequest struct {
	Values [][]any `json:"values"`
}

// UpdateRange writes values to the cells at address on the worksheet; values
// must match the range's dimensions, and a nil value leaves its cell unchanged.
// A string starting with "=" is entered as a formula.
func (c *Client) UpdateRange(
	ctx context.Context,
	driveId string,
	itemId string,
	worksheet string,
	address string,
	values [][]any,
	options ...request_config.Option,
) (*workbook_range.Range, error) {
	if err := validateWorkbook(driveId, itemId); err != nil {
		return nil, err
	}
	if worksheet == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("worksheet"))
	}
	if address == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("address"))
	}
	if len(values) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("values"))
	}

	requestConfig := request_config.New(options...)

	return rest.SendJson[workbook_range.Range](
		ctx,
		http.MethodPatch,
		rangePath(driveId, itemId, worksheet, address).Url(c.baseUrl, nil),
		&updateRangeRequest{Values: values},
		c.fetchOptions(requestConfig.SessionId, requestConfig.FetchOptions),
	)
}
