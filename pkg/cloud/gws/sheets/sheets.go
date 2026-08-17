package sheets

import (
	"context"
	"net/url"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/sheets/get_spreadsheet_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/sheets/get_values_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/sheets/sheets_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/sheets/types/spreadsheet"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/sheets/types/value_range"
)

const Domain = "sheets.googleapis.com"

const (
	ScopeSpreadsheets         = "https://www.googleapis.com/auth/spreadsheets"
	ScopeSpreadsheetsReadonly = "https://www.googleapis.com/auth/spreadsheets.readonly"
)

var defaultBaseUrl = &url.URL{
	Scheme: "https",
	Host:   Domain,
}

type Client struct {
	baseUrl *url.URL
	config  *sheets_config.Config
}

func NewClient(options ...sheets_config.Option) *Client {
	config := sheets_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = defaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/v4/spreadsheets/"
	return &Client{baseUrl: &u, config: config}
}

// The URL builders set both Path (decoded) and RawPath (escaped): appending
// PathEscape output to Path alone would make URL.String re-escape the percent
// signs, which matters for the non-ASCII sheet titles A1 ranges can contain.

func (c *Client) spreadsheetUrl(spreadsheetId string) string {
	u := *c.baseUrl
	u.Path = c.baseUrl.Path + spreadsheetId
	u.RawPath = c.baseUrl.Path + url.PathEscape(spreadsheetId)
	return u.String()
}

func (c *Client) valuesUrl(spreadsheetId string, valuesRange string) string {
	u := *c.baseUrl
	u.Path = c.baseUrl.Path + spreadsheetId + "/values/" + valuesRange
	u.RawPath = c.baseUrl.Path + url.PathEscape(spreadsheetId) + "/values/" + url.PathEscape(valuesRange)
	return u.String()
}

func (c *Client) fetchOptions(options []fetch_config.Option) []fetch_config.Option {
	return append(c.config.FetchOptions, options...)
}

func withQuery(urlString string, query url.Values) string {
	if encoded := query.Encode(); encoded != "" {
		return urlString + "?" + encoded
	}
	return urlString
}

// GetSpreadsheet retrieves spreadsheet metadata (the sheet list and properties), not cell data.
func (c *Client) GetSpreadsheet(ctx context.Context, spreadsheetId string, options ...get_spreadsheet_config.Option) (*spreadsheet.Spreadsheet, error) {
	if spreadsheetId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("spreadsheet id"))
	}

	getSpreadsheetConfig := get_spreadsheet_config.New(options...)

	query := url.Values{}
	if fields := getSpreadsheetConfig.Fields; fields != "" {
		query.Set("fields", fields)
	}

	return rest.GetJson[spreadsheet.Spreadsheet](
		ctx,
		withQuery(c.spreadsheetUrl(spreadsheetId), query),
		c.fetchOptions(getSpreadsheetConfig.FetchOptions),
	)
}

// GetValues retrieves a range of cell values. The range is given in A1 notation
// (e.g. "'Besök'!A2:E") or as a named range.
func (c *Client) GetValues(ctx context.Context, spreadsheetId string, valuesRange string, options ...get_values_config.Option) (*value_range.ValueRange, error) {
	if spreadsheetId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("spreadsheet id"))
	}
	if valuesRange == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("values range"))
	}

	getValuesConfig := get_values_config.New(options...)

	query := url.Values{}
	if majorDimension := getValuesConfig.MajorDimension; majorDimension != "" {
		query.Set("majorDimension", string(majorDimension))
	}

	return rest.GetJson[value_range.ValueRange](
		ctx,
		withQuery(c.valuesUrl(spreadsheetId, valuesRange), query),
		c.fetchOptions(getValuesConfig.FetchOptions),
	)
}
