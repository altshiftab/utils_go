package session

// Session is a workbook session; passing its id with later calls batches them
// and, with PersistChanges false, discards their changes when it closes.
type Session struct {
	Id             string `json:"id,omitzero"`
	PersistChanges bool   `json:"persistChanges,omitzero"`
}
