package message

import (
	"errors"
	"net/mail"
	"reflect"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftMailErrors "github.com/altshiftab/utils_go/pkg/mail/errors"
	"github.com/altshiftab/utils_go/pkg/mail/types/message/message_config"
	"github.com/altshiftab/utils_go/pkg/mail/types/message/message_header"
)

func validFrom() *mail.Address { return &mail.Address{Address: "from@example.com"} }
func validTo() []*mail.Address { return []*mail.Address{{Address: "to@example.com"}} }

func TestNew_NilFrom(t *testing.T) {
	t.Parallel()

	msg, err := New(nil, validTo(), "subj", nil)
	if msg != nil {
		t.Fatalf("msg = %v, want nil", msg)
	}
	ne, ok := errors.AsType[*nil_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *nil_error.Error", err, err)
	}
	if ne.Field != "address" || ne.Instance != "from" {
		t.Errorf("ne = %+v, want Field=address Instance=from", ne)
	}
}

func TestNew_EmptyTo(t *testing.T) {
	t.Parallel()

	_, err := New(validFrom(), nil, "subj", nil)
	ee, ok := errors.AsType[*empty_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
	}
	if ee.Field != "to" {
		t.Errorf("Field = %q, want %q", ee.Field, "to")
	}
}

func TestNew_EmptySubject(t *testing.T) {
	t.Parallel()

	_, err := New(validFrom(), validTo(), "", nil)
	ee, ok := errors.AsType[*empty_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
	}
	if ee.Field != "subject" {
		t.Errorf("Field = %q, want %q", ee.Field, "subject")
	}
}

func TestNew_EmptyContentType(t *testing.T) {
	t.Parallel()

	_, err := New(validFrom(), validTo(), "subj", &Body{Content: []byte("hi")})
	ee, ok := errors.AsType[*empty_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
	}
	if ee.Field != "content type" {
		t.Errorf("Field = %q, want %q", ee.Field, "content type")
	}
}

func TestNew_AppliesOptions(t *testing.T) {
	t.Parallel()

	cc := []*mail.Address{{Address: "cc@example.com"}}
	bcc := []*mail.Address{{Address: "bcc@example.com"}}
	replyTo := []*mail.Address{{Address: "reply@example.com"}}
	headers := []*message_header.Header{
		{Name: "X-Foo", Value: "bar"},
		{Name: "X-Baz", Value: "qux"},
	}

	msg, err := New(
		validFrom(),
		validTo(),
		"subj",
		&Body{Content: []byte("hi"), ContentType: "text/plain"},
		message_config.WithCc(cc),
		message_config.WithBcc(bcc),
		message_config.WithReplyTo(replyTo),
		message_config.WithDomain("custom.example"),
		message_config.WithHeaders(headers...),
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(msg.Cc, cc) {
		t.Errorf("Cc = %v, want %v", msg.Cc, cc)
	}
	if !reflect.DeepEqual(msg.Bcc, bcc) {
		t.Errorf("Bcc = %v, want %v", msg.Bcc, bcc)
	}
	if !reflect.DeepEqual(msg.ReplyTo, replyTo) {
		t.Errorf("ReplyTo = %v, want %v", msg.ReplyTo, replyTo)
	}
	if msg.Domain != "custom.example" {
		t.Errorf("Domain = %q, want %q", msg.Domain, "custom.example")
	}
	if !reflect.DeepEqual(msg.Headers, headers) {
		t.Errorf("Headers = %v, want %v", msg.Headers, headers)
	}
}

func TestMessage_String_IncludesHeaders(t *testing.T) {
	t.Parallel()

	msg, err := New(
		validFrom(),
		validTo(),
		"subj",
		&Body{Content: []byte("hi"), ContentType: "text/plain"},
		message_config.WithHeaders(
			&message_header.Header{Name: "X-Foo", Value: "bar"},
			nil,
			&message_header.Header{Name: "List-Unsubscribe", Value: "<mailto:u@example.com>"},
		),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out, err := msg.String()
	if err != nil {
		t.Fatalf("String: %v", err)
	}
	if !strings.Contains(out, "\r\nX-Foo: bar\r\n") {
		t.Errorf("output missing X-Foo header:\n%s", out)
	}
	if !strings.Contains(out, "\r\nList-Unsubscribe: <mailto:u@example.com>\r\n") {
		t.Errorf("output missing List-Unsubscribe header:\n%s", out)
	}
	if strings.Contains(out, ": \r\n\r\n") {
		t.Errorf("nil header should be skipped:\n%s", out)
	}
}

func TestMessage_String_DerivesDomainFromFrom(t *testing.T) {
	t.Parallel()

	msg, err := New(
		&mail.Address{Address: "alice@derived.example"},
		validTo(),
		"subj",
		&Body{Content: []byte("hi"), ContentType: "text/plain"},
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out, err := msg.String()
	if err != nil {
		t.Fatalf("String: %v", err)
	}
	if !strings.Contains(out, "@derived.example>\r\n") {
		t.Errorf("Message-ID did not use From domain:\n%s", out)
	}
}

func TestMessage_String_BadFromAddress(t *testing.T) {
	t.Parallel()

	msg := &Message{
		From:    &mail.Address{Address: "no-at-sign"},
		To:      validTo(),
		Subject: "subj",
	}
	if _, err := msg.String(); !errors.Is(err, altshiftMailErrors.ErrBadFromAddress) {
		t.Fatalf("err = %v, want ErrBadFromAddress", err)
	}
}

func TestNew_MultipartEmptySubtype(t *testing.T) {
	t.Parallel()

	body := &Body{
		Parts: []*Body{
			{Content: []byte("hi"), ContentType: "text/plain"},
		},
	}
	_, err := New(validFrom(), validTo(), "subj", body)
	ee, ok := errors.AsType[*empty_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
	}
	if ee.Field != "body subtype" {
		t.Errorf("Field = %q, want %q", ee.Field, "body subtype")
	}
}

func TestNew_MultipartNestedLeafMissingContentType(t *testing.T) {
	t.Parallel()

	body := &Body{
		Subtype: "alternative",
		Parts: []*Body{
			{Content: []byte("hi"), ContentType: "text/plain"},
			{Content: []byte("<p>hi</p>")},
		},
	}
	_, err := New(validFrom(), validTo(), "subj", body)
	ee, ok := errors.AsType[*empty_error.Error](err)
	if !ok {
		t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
	}
	if ee.Field != "content type" {
		t.Errorf("Field = %q, want %q", ee.Field, "content type")
	}
}

func TestMessage_String_MultipartAlternative(t *testing.T) {
	t.Parallel()

	textContent := []byte("plain text version")
	htmlContent := []byte("<p>html version</p>")

	msg, err := New(
		validFrom(),
		validTo(),
		"subj",
		&Body{
			Subtype: "alternative",
			Parts: []*Body{
				{Content: textContent, ContentType: "text/plain; charset=utf-8"},
				{Content: htmlContent, ContentType: "text/html; charset=utf-8"},
			},
		},
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out, err := msg.String()
	if err != nil {
		t.Fatalf("String: %v", err)
	}

	headers, rest, ok := strings.Cut(out, "\r\n\r\n")
	if !ok {
		t.Fatalf("missing header/body separator:\n%s", out)
	}
	if !strings.Contains(headers, "\r\nContent-Type: multipart/alternative; boundary=\"") {
		t.Errorf("missing multipart Content-Type header:\n%s", headers)
	}

	ctIdx := strings.Index(headers, "boundary=\"")
	boundary := headers[ctIdx+len("boundary=\"") : strings.Index(headers[ctIdx+len("boundary=\""):], "\"")+ctIdx+len("boundary=\"")]
	if !strings.HasPrefix(boundary, "=_") {
		t.Errorf("boundary = %q, want prefix =_", boundary)
	}

	openMarker := "--" + boundary + "\r\n"
	closeMarker := "--" + boundary + "--\r\n"
	if strings.Count(rest, openMarker) != 2 {
		t.Errorf("expected 2 part openers, got %d:\n%s", strings.Count(rest, openMarker), rest)
	}
	if !strings.HasSuffix(rest, closeMarker) {
		t.Errorf("body does not end with closing boundary:\n%s", rest)
	}
	if !strings.Contains(rest, "Content-Type: text/plain; charset=utf-8\r\n\r\nplain text version\r\n") {
		t.Errorf("text part not rendered as expected:\n%s", rest)
	}
	if !strings.Contains(rest, "Content-Type: text/html; charset=utf-8\r\n\r\n<p>html version</p>\r\n") {
		t.Errorf("html part not rendered as expected:\n%s", rest)
	}
}

func TestMessage_String_LeafEmitsPartHeaders(t *testing.T) {
	t.Parallel()

	msg, err := New(
		validFrom(),
		validTo(),
		"subj",
		&Body{
			Content:     []byte("hi"),
			ContentType: "text/plain",
			Headers: []*message_header.Header{
				{Name: "Content-Language", Value: "en"},
				nil,
				{Name: "Content-Description", Value: "greeting"},
			},
		},
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out, err := msg.String()
	if err != nil {
		t.Fatalf("String: %v", err)
	}
	if !strings.Contains(out, "Content-Type: text/plain\r\nContent-Language: en\r\nContent-Description: greeting\r\n\r\nhi") {
		t.Errorf("part headers not emitted in order:\n%s", out)
	}
}

func TestAttachment_Defaults(t *testing.T) {
	t.Parallel()

	att := Attachment("", "", []byte("hi"))
	if att.ContentType != "application/octet-stream" {
		t.Errorf("ContentType = %q, want %q", att.ContentType, "application/octet-stream")
	}
	if len(att.Headers) != 2 {
		t.Fatalf("Headers len = %d, want 2", len(att.Headers))
	}
	if att.Headers[0].Name != "Content-Disposition" || att.Headers[0].Value != "attachment" {
		t.Errorf("Content-Disposition = %+v, want {attachment}", att.Headers[0])
	}
	if att.Headers[1].Name != "Content-Transfer-Encoding" || att.Headers[1].Value != "base64" {
		t.Errorf("Content-Transfer-Encoding = %+v, want {base64}", att.Headers[1])
	}
	if string(att.Content) != "aGk=" {
		t.Errorf("Content = %q, want base64 of \"hi\"", att.Content)
	}
}

func TestAttachment_WithFilenameAndType(t *testing.T) {
	t.Parallel()

	att := Attachment("report.pdf", "application/pdf", []byte("pdf-bytes"))
	if att.ContentType != "application/pdf; name=report.pdf" {
		t.Errorf("ContentType = %q", att.ContentType)
	}
	if att.Headers[0].Value != "attachment; filename=report.pdf" {
		t.Errorf("Content-Disposition = %q", att.Headers[0].Value)
	}
}

func TestAttachment_NonASCIIFilename(t *testing.T) {
	t.Parallel()

	att := Attachment("résumé.pdf", "application/pdf", []byte("pdf-bytes"))
	if !strings.Contains(att.ContentType, "name*=utf-8''r%C3%A9sum%C3%A9.pdf") {
		t.Errorf("ContentType missing RFC 2231 name*: %q", att.ContentType)
	}
	if !strings.Contains(att.Headers[0].Value, "filename*=utf-8''r%C3%A9sum%C3%A9.pdf") {
		t.Errorf("Content-Disposition missing RFC 2231 filename*: %q", att.Headers[0].Value)
	}
}

func TestAttachment_WrapsLongBase64(t *testing.T) {
	t.Parallel()

	raw := make([]byte, 200)
	for i := range raw {
		raw[i] = byte(i)
	}
	att := Attachment("blob.bin", "application/octet-stream", raw)
	lines := strings.Split(string(att.Content), "\r\n")
	if len(lines) < 2 {
		t.Fatalf("expected multiple lines, got %d", len(lines))
	}
	for i, line := range lines {
		if len(line) > 76 {
			t.Errorf("line %d length %d exceeds 76: %q", i, len(line), line)
		}
	}
}

func TestMessage_String_AttachmentRoundTrip(t *testing.T) {
	t.Parallel()

	msg, err := New(
		validFrom(),
		validTo(),
		"subj",
		&Body{
			Subtype: "mixed",
			Parts: []*Body{
				{Content: []byte("see attached"), ContentType: "text/plain"},
				Attachment("hello.txt", "text/plain", []byte("hello world")),
			},
		},
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out, err := msg.String()
	if err != nil {
		t.Fatalf("String: %v", err)
	}
	if !strings.Contains(out, "Content-Type: text/plain; name=hello.txt\r\n") {
		t.Errorf("attachment Content-Type missing:\n%s", out)
	}
	if !strings.Contains(out, "Content-Disposition: attachment; filename=hello.txt\r\n") {
		t.Errorf("attachment Content-Disposition missing:\n%s", out)
	}
	if !strings.Contains(out, "Content-Transfer-Encoding: base64\r\n") {
		t.Errorf("Content-Transfer-Encoding missing:\n%s", out)
	}
	if !strings.Contains(out, "\r\n\r\naGVsbG8gd29ybGQ=") {
		t.Errorf("base64 body of \"hello world\" missing:\n%s", out)
	}
}

func TestMessage_String_MultipartNested(t *testing.T) {
	t.Parallel()

	msg, err := New(
		validFrom(),
		validTo(),
		"subj",
		&Body{
			Subtype: "mixed",
			Parts: []*Body{
				{
					Subtype: "alternative",
					Parts: []*Body{
						{Content: []byte("plain"), ContentType: "text/plain"},
						{Content: []byte("<p>html</p>"), ContentType: "text/html"},
					},
				},
				{Content: []byte("attachment-bytes"), ContentType: "application/octet-stream"},
			},
		},
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out, err := msg.String()
	if err != nil {
		t.Fatalf("String: %v", err)
	}
	if !strings.Contains(out, "Content-Type: multipart/mixed; boundary=\"") {
		t.Errorf("missing outer multipart/mixed:\n%s", out)
	}
	if !strings.Contains(out, "Content-Type: multipart/alternative; boundary=\"") {
		t.Errorf("missing inner multipart/alternative:\n%s", out)
	}
	if !strings.Contains(out, "Content-Type: application/octet-stream\r\n\r\nattachment-bytes\r\n") {
		t.Errorf("attachment part not rendered as expected:\n%s", out)
	}
}
