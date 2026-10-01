package table

type Table struct {
	Id          string `json:"id,omitzero"`
	Name        string `json:"name,omitzero"`
	ShowHeaders bool   `json:"showHeaders,omitzero"`
	ShowTotals  bool   `json:"showTotals,omitzero"`
	Style       string `json:"style,omitzero"`
}
