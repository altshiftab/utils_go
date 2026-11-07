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

// echoHtml is a reader of HTML that says what it was given.
func echoHtml(source string) string {
	return "[html]" + source
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
			want: "Hello,\n\nSee you  tomorrow.",
		},
		{
			name: "plain text keeps its indentation",
			msg: &message.Message{Payload: textPart(
				"text/plain",
				"\n× unit.service\n     Loaded: loaded\n  Transient: yes  \r\n\r\n\r\n\r\n\tindented\n",
			)},
			want: "× unit.service\n     Loaded: loaded\n  Transient: yes\n\n\tindented",
		},
		{
			name: "plain text loses the invisible padding of a preview text",
			msg: &message.Message{Payload: textPart(
				"text/plain",
				"Nytt avrop \u034f \u200c\u00a0\u00a0\u00ad \u034f \u200c\u00a0\u00ad\nf\u00f6r\u00adst\u00e5 \U0001F468\u200d\U0001F469",
			)},
			want: "Nytt avrop\nförstå \U0001F468\u200d\U0001F469",
		},
		{
			name: "plain text that is an html document is read as html",
			msg: &message.Message{Payload: textPart(
				"text/plain",
				"  <!DOCTYPE html><html><body><p>Hej</p><p>Kvitto</p></body></html>",
			)},
			want: "[html]  <!DOCTYPE html><html><body><p>Hej</p><p>Kvitto</p></body></html>",
		},
		{
			name: "plain text that is an html document past a byte order mark and a comment is read as html",
			msg: &message.Message{Payload: textPart(
				"text/plain",
				"\ufeff<!-- generated --> <!-- twice -->\n<html><body><p>Hej</p></body></html>",
			)},
			want: "[html]<!-- generated --> <!-- twice -->\n<html><body><p>Hej</p></body></html>",
		},
		{
			name: "plain text beginning with an unclosed comment stays plain",
			msg:  &message.Message{Payload: textPart("text/plain", "<!-- note <html>")},
			want: "<!-- note <html>",
		},
		{
			name: "plain text that is an html element alone stays plain",
			msg:  &message.Message{Payload: textPart("text/plain", "<b>not a document</b>")},
			want: "<b>not a document</b>",
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
			name: "html only is read by the reader",
			msg: &message.Message{Payload: textPart(
				"text/html",
				`<html><head><title>T</title><style>p{color:red}</style></head><body><h1>Hi</h1><p>One &amp; two<br>three</p><!-- nothing --><script>alert(1)</script><div>four</div></body></html>`,
			)},
			want: "[html]" + `<html><head><title>T</title><style>p{color:red}</style></head><body><h1>Hi</h1><p>One &amp; two<br>three</p><!-- nothing --><script>alert(1)</script><div>four</div></body></html>`,
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

			if got := Text(testCase.msg, echoHtml); got != testCase.want {
				t.Errorf("body = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestTextReader(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		msg        *message.Message
		htmlToText func(string) string
		want       string
	}{
		{
			name:       "an html part is read by the reader",
			msg:        &message.Message{Payload: textPart("text/html", "<p>html</p>")},
			htmlToText: echoHtml,
			want:       "[html]<p>html</p>",
		},
		{
			name:       "a plain part is not",
			msg:        &message.Message{Payload: textPart("text/plain", "plain")},
			htmlToText: echoHtml,
			want:       "plain",
		},
		{
			name: "without a reader, html alone is no text",
			msg:  &message.Message{Payload: textPart("text/html", "<p>html</p>")},
		},
		{
			name: "without a reader, a plain part that is an html document is no text",
			msg:  &message.Message{Payload: textPart("text/plain", "<html><body>x</body></html>")},
		},
		{
			name: "without a reader, a plain part is read all the same",
			msg:  &message.Message{Payload: textPart("text/plain", "plain")},
			want: "plain",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := Text(testCase.msg, testCase.htmlToText); got != testCase.want {
				t.Errorf("text = %q, want %q", got, testCase.want)
			}
		})
	}
}
