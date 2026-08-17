package sheet

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gws/sheets/types/spreadsheet/sheet/sheet_properties"
)

type Sheet struct {
	Properties *sheet_properties.SheetProperties `json:"properties,omitzero"`
}
