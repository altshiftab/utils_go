package attachment_content

import "io"

// Content is an attachment's content as it arrives, unread: the caller reads Body and closes it.
// Length is -1 where Graph did not say.
type Content struct {
	Body        io.ReadCloser
	ContentType string
	Length      int64
}
