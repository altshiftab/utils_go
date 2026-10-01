package table_row

type TableRow struct {
	Index int `json:"index,omitzero"`
	// Values holds the row's cells as JSON strings, numbers, booleans or null.
	Values [][]any `json:"values,omitzero"`
}
