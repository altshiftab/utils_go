package item_body

// The types a body is served as: Exchange keeps one and converts to the other when asked.
const (
	ContentTypeText = "text"
	ContentTypeHtml = "html"
)

// ItemBody is a message's body, and whether it is text or HTML.
type ItemBody struct {
	ContentType string `json:"contentType,omitzero"`
	Content     string `json:"content,omitzero"`
}
