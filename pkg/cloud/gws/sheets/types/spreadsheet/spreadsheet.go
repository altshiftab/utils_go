package spreadsheet

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gws/sheets/types/spreadsheet/sheet"
)

type Spreadsheet struct {
	SpreadsheetId  string         `json:"spreadsheetId,omitzero"`
	SpreadsheetUrl string         `json:"spreadsheetUrl,omitzero"`
	Sheets         []*sheet.Sheet `json:"sheets,omitzero"`
}
