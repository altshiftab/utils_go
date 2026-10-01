package workbook_range

// Range is a rectangle of cells. Values holds what the cells evaluate to (JSON
// strings, numbers or booleans), Text how they display, Formulas what was typed.
type Range struct {
	Address      string     `json:"address,omitzero"`
	AddressLocal string     `json:"addressLocal,omitzero"`
	CellCount    int        `json:"cellCount,omitzero"`
	ColumnCount  int        `json:"columnCount,omitzero"`
	ColumnIndex  int        `json:"columnIndex,omitzero"`
	RowCount     int        `json:"rowCount,omitzero"`
	RowIndex     int        `json:"rowIndex,omitzero"`
	Values       [][]any    `json:"values,omitzero"`
	Text         [][]string `json:"text,omitzero"`
	Formulas     [][]any    `json:"formulas,omitzero"`
	NumberFormat [][]any    `json:"numberFormat,omitzero"`
}
