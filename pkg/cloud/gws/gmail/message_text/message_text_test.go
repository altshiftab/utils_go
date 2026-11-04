package message_text

import (
	"encoding/base64"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message/message_part"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message/message_part/body"
)

// textPart is a part of the mime type carrying the data, encoded the way
// Gmail serves it.
func textPart(mimeType string, data string) *message_part.MessagePart {
	return &message_part.MessagePart{
		MimeType: mimeType,
		Body:     &body.Body{Data: base64.URLEncoding.EncodeToString([]byte(data)), Size: len(data)},
	}
}

func multipart(mimeType string, parts ...*message_part.MessagePart) *message_part.MessagePart {
	return &message_part.MessagePart{MimeType: mimeType, Parts: parts}
}

func TestBody(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		msg  *message.Message
		want string
	}{
		{
			name: "nil",
		},
		{
			name: "no payload",
			msg:  &message.Message{},
		},
		{
			name: "plain text",
			msg:  &message.Message{Payload: textPart("text/plain", "Hello,\r\n\r\nSee you  tomorrow.\r\n")},
			want: "Hello,\n\nSee you tomorrow.",
		},
		{
			name: "plain text without padding",
			msg: &message.Message{Payload: &message_part.MessagePart{
				MimeType: "text/plain",
				Body:     &body.Body{Data: base64.RawURLEncoding.EncodeToString([]byte("no padding"))},
			}},
			want: "no padding",
		},
		{
			name: "alternative prefers plain text",
			msg: &message.Message{Payload: multipart(
				"multipart/alternative",
				textPart("text/plain", "plain"),
				textPart("text/html", "<p>html</p>"),
			)},
			want: "plain",
		},
		{
			name: "html only is read as text",
			msg: &message.Message{Payload: textPart(
				"text/html",
				`<html><head><title>T</title><style>p{color:red}</style></head><body><h1>Hi</h1><p>One &amp; two<br>three</p><!-- nothing --><script>alert(1)</script><div>four</div></body></html>`,
			)},
			want: "Hi\nOne & two\nthree\nfour",
		},
		{
			name: "nested mixed with an attachment",
			msg: &message.Message{Payload: multipart(
				"multipart/mixed",
				multipart(
					"multipart/alternative",
					textPart("text/html", "<b>html</b>"),
					textPart("text/plain", "plain"),
				),
				&message_part.MessagePart{
					MimeType: "text/plain",
					Filename: "notes.txt",
					Body:     &body.Body{AttachmentId: "att", Size: 12},
				},
			)},
			want: "plain",
		},
		{
			name: "an attachment of the text type is not the body",
			msg: &message.Message{Payload: multipart(
				"multipart/mixed",
				&message_part.MessagePart{
					MimeType: "text/plain",
					Filename: "notes.txt",
					Body:     &body.Body{Data: base64.URLEncoding.EncodeToString([]byte("attached"))},
				},
			)},
		},
		{
			name: "no text at all",
			msg:  &message.Message{Payload: multipart("multipart/mixed", textPart("image/png", "\x89PNG"))},
		},
		{
			name: "latin-1 is read as such",
			msg: &message.Message{Payload: &message_part.MessagePart{
				MimeType: "text/plain",
				Body:     &body.Body{Data: base64.URLEncoding.EncodeToString([]byte{'H', 'e', 'j', ' ', 0xe5, 0xe4, 0xf6})},
			}},
			want: "Hej åäö",
		},
		{
			name: "mime type case",
			msg:  &message.Message{Payload: textPart("Text/Plain", "x")},
			want: "x",
		},
		{
			name: "undecodable data",
			msg: &message.Message{Payload: &message_part.MessagePart{
				MimeType: "text/plain",
				Body:     &body.Body{Data: "not base64!"},
			}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := Text(testCase.msg); got != testCase.want {
				t.Errorf("body = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestHtmlToText(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "empty",
		},
		{
			name:   "plain",
			source: "just text",
			want:   "just text",
		},
		{
			name:   "paragraphs and breaks",
			source: "<p>one</p><p>two<br/>three</p>",
			want:   "one\ntwo\nthree",
		},
		{
			name:   "list and table",
			source: "<ul><li>a</li><li>b</li></ul><table><tr><td>c</td><td>d</td></tr></table>",
			want:   "a\nb\ncd",
		},
		{
			name:   "entities",
			source: "caf&eacute; &lt;b&gt; &#8364;&nbsp;5",
			want:   "café <b> € 5",
		},
		{
			name:   "style and script dropped",
			source: "<style>\np { color: red }\n</style>visible<script type=\"text/javascript\">alert('x')</script>",
			want:   "visible",
		},
		{
			name:   "tag across lines",
			source: "<a\nhref=\"https://example.com\"\n>link</a>",
			want:   "link <https://example.com>",
		},
		{
			name:   "a link keeps its target",
			source: `<p>Confirm <a class="b" href="https://example.com/c?a=1&amp;b=2">here</a>.</p>`,
			want:   "Confirm here <https://example.com/c?a=1&b=2>.",
		},
		{
			name:   "a link that says its target says it once",
			source: `<a href="https://example.com">https://example.com</a> <a href='mailto:a@example.com'>a@example.com</a>`,
			want:   "https://example.com a@example.com",
		},
		{
			name:   "a link of an image alone is its target",
			source: `<a href="https://example.com/x"><img src="logo.png"></a>`,
			want:   "<https://example.com/x>",
		},
		{
			name:   "a link elsewhere than the web keeps its text alone",
			source: `<a href="javascript:alert(1)">click</a> <a href="#top">top</a>`,
			want:   "click top",
		},
		{
			name:   "blank lines collapsed",
			source: "<p>a</p>\n\n\n\n<p>b</p>",
			want:   "a\n\nb",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := HtmlToText(testCase.source); got != testCase.want {
				t.Errorf("text = %q, want %q", got, testCase.want)
			}
		})
	}
}
