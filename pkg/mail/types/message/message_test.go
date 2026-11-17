package message

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"reflect"
	"strings"
	"testing"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
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

func TestMessage_String_Subject(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		subject string
		want    string
		wantErr bool
	}{
		{name: "ascii is written as it is", subject: "Invoice 42", want: "Subject: Invoice 42\r\n"},
		{name: "other text is an encoded word", subject: "Räkning för oktober", want: "Subject: =?utf-8?q?R=C3=A4kning_f=C3=B6r_oktober?=\r\n"},
		{name: "a line break is refused", subject: "Hi\r\nBcc: someone@example.com", wantErr: true},
		{name: "a bare newline is refused", subject: "Hi\nthere", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			msg := &Message{
				From:    &mail.Address{Address: "a@example.com"},
				To:      []*mail.Address{{Address: "b@example.com"}},
				Subject: testCase.subject,
				Body:    Text("x"),
			}
			got, err := msg.String()
			if (err != nil) != testCase.wantErr {
				t.Fatalf("%s: err = %v, want error %t", testCase.name, err, testCase.wantErr)
			}
			if testCase.wantErr {
				if !errors.Is(err, altshiftMailErrors.ErrHeaderInjection) {
					t.Errorf("%s: err = %v, want header injection", testCase.name, err)
				}
				return
			}
			if !strings.Contains(got, testCase.want) {
				t.Errorf("%s: %q does not have %q", testCase.name, got, testCase.want)
			}
		})
	}
}

func TestMessage_String_HeaderInjection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		header *message_header.Header
	}{
		{name: "a value with a line break", header: &message_header.Header{Name: "In-Reply-To", Value: "<a@b>\r\nBcc: x@example.com"}},
		{name: "a name with a colon", header: &message_header.Header{Name: "Bcc: x@example.com\r\nX", Value: "y"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			msg := &Message{
				From:    &mail.Address{Address: "a@example.com"},
				To:      []*mail.Address{{Address: "b@example.com"}},
				Subject: "s",
				Headers: []*message_header.Header{testCase.header},
			}
			if _, err := msg.String(); !errors.Is(err, altshiftMailErrors.ErrHeaderInjection) {
				t.Errorf("%s: err = %v, want header injection", testCase.name, err)
			}
		})
	}
}

func TestText(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		text string
	}{
		{name: "ascii", text: "Hello,\r\nsee you."},
		{name: "swedish", text: "Hej på dig, vi ses i övermorgon."},
		{name: "a long line", text: strings.Repeat("ord ", 60)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			body := Text(testCase.text)
			if body.ContentType != "text/plain; charset=utf-8" {
				t.Errorf("%s: content type = %q", testCase.name, body.ContentType)
			}
			for _, line := range strings.Split(string(body.Content), "\r\n") {
				if len(line) > 76 {
					t.Errorf("%s: a line of %d characters", testCase.name, len(line))
				}
			}
			decoded, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(body.Content)))
			if err != nil {
				t.Fatalf("%s: read: %v", testCase.name, err)
			}
			if string(decoded) != testCase.text {
				t.Errorf("%s: decoded %q, want %q", testCase.name, decoded, testCase.text)
			}
		})
	}
}

func TestMessage_String_FoldsLongHeaders(t *testing.T) {
	t.Parallel()

	var references []string
	for index := range 40 {
		references = append(references, fmt.Sprintf("<message-%d@mail.example.com>", index))
	}
	msg := &Message{
		From:    &mail.Address{Address: "a@example.com"},
		To:      []*mail.Address{{Address: "b@example.com"}},
		Subject: strings.Repeat("Räkning ", 20),
		Body:    Text("x"),
		Headers: []*message_header.Header{{Name: "References", Value: strings.Join(references, " ")}},
	}
	written, err := msg.String()
	if err != nil {
		t.Fatalf("string: %v", err)
	}
	head, _, _ := strings.Cut(written, "\r\n\r\n")
	for _, line := range strings.Split(head, "\r\n") {
		if len(line) > foldLength {
			t.Errorf("a line of %d characters: %q", len(line), line)
		}
	}

	// Read back, the headers are what was written.
	parsed, err := mail.ReadMessage(strings.NewReader(written))
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	if got := parsed.Header.Get("References"); got != strings.Join(references, " ") {
		t.Errorf("references = %q", got)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
	if err != nil || subject != strings.Repeat("Räkning ", 20) {
		t.Errorf("subject = %q, %v", subject, err)
	}
}

func TestMessage_String_Date(t *testing.T) {
	t.Parallel()

	stockholm, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	testCases := []struct {
		name    string
		options []message_config.Option
		want    string
	}{
		{
			name:    "dated in its own zone",
			options: []message_config.Option{message_config.WithDate(time.Date(2026, 11, 7, 19, 39, 14, 0, stockholm))},
			want:    "\r\nDate: Sat, 07 Nov 2026 19:39:14 +0100\r\n",
		},
		{name: "dated when written", want: "\r\nDate: "},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			msg, err := New(validFrom(), validTo(), "subj", &Body{Content: []byte("hi"), ContentType: "text/plain"}, testCase.options...)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			out, err := msg.String()
			if err != nil {
				t.Fatalf("String: %v", err)
			}
			if !strings.Contains(out, testCase.want) {
				t.Errorf("output missing %q:\n%s", testCase.want, out)
			}
		})
	}
}

func TestCheckFilename(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{name: "a plain name", filename: "request.csr"},
		{name: "a name outside ASCII", filename: "Årsredovisning 2025 – slutlig.pdf"},
		{name: "a name in another script", filename: "Счёт.pdf"},
		{name: "spaces, parentheses and a semicolon", filename: "scan (1); signed.pdf"},
		{name: "a name of the longest length", filename: strings.Repeat("a", MaxFilenameBytes-4) + ".pdf"},
		{name: "empty", filename: "", wantErr: true},
		{name: "a name too long", filename: strings.Repeat("a", MaxFilenameBytes-3) + ".pdf", wantErr: true},
		{name: "the directory itself", filename: ".", wantErr: true},
		{name: "the directory above", filename: "..", wantErr: true},
		{name: "a line break that would add a header", filename: "a.pdf\r\nBcc: x@example.com", wantErr: true},
		{name: "a bare newline", filename: "a\nb.pdf", wantErr: true},
		{name: "a tab", filename: "a\tb.pdf", wantErr: true},
		{name: "a NUL", filename: "a\x00.pdf", wantErr: true},
		{name: "a DEL", filename: "a\x7f.pdf", wantErr: true},
		{name: "a C1 control", filename: "a\u0085.pdf", wantErr: true},
		{name: "a right-to-left override", filename: "invoice\u202efdp.exe", wantErr: true},
		{name: "a quote", filename: `a".pdf`, wantErr: true},
		{name: "a backslash", filename: `a\b.pdf`, wantErr: true},
		{name: "a slash", filename: "../etc/passwd", wantErr: true},
		{name: "not UTF-8", filename: "a\xff.pdf", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := CheckFilename(testCase.filename)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("%s: err = %v, want error %t", testCase.name, err, testCase.wantErr)
			}
			if err != nil && !errors.Is(err, altshiftErrors.ErrValidationError) {
				t.Errorf("%s: err = %v, want a validation error", testCase.name, err)
			}
		})
	}
}

func TestAttachment_ContentType(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		contentType string
		want        string
	}{
		{name: "none is a stream of bytes", contentType: "", want: "application/octet-stream; name=x.pdf"},
		{name: "a type", contentType: "application/pdf", want: "application/pdf; name=x.pdf"},
		{name: "a type with a parameter, in capitals", contentType: "TEXT/Plain; charset=utf-8", want: "text/plain; charset=utf-8; name=x.pdf"},
		{name: "a type whose name is given", contentType: `application/pdf; name="other.pdf"`, want: "application/pdf; name=x.pdf"},
		{name: "a type with a parameter that does not parse", contentType: "application/pdf; charset", want: "application/pdf; name=x.pdf"},
		{name: "a line break that would add a header", contentType: "text/plain\r\nBcc: x@example.com", want: "application/octet-stream; name=x.pdf"},
		{name: "no subtype", contentType: "pdf", want: "application/octet-stream; name=x.pdf"},
		{name: "not a type", contentType: "a b/c", want: "application/octet-stream; name=x.pdf"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := Attachment("x.pdf", testCase.contentType, []byte("x")).ContentType; got != testCase.want {
				t.Errorf("%s: ContentType = %q, want %q", testCase.name, got, testCase.want)
			}
		})
	}
}

// TestAttachment_FilenameRoundTrip writes each name into a message and reads it back from the headers as written, so
// that a name is what a mail client reads, and none adds a header or a line longer than mail allows.
func TestAttachment_FilenameRoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		filename string
	}{
		{name: "plain", filename: "request.csr"},
		{name: "spaces and a semicolon, quoted", filename: "scan (1); signed.pdf"},
		{name: "outside ASCII, as RFC 2231 has it", filename: "Årsredovisning – slutlig.pdf"},
		{name: "the longest, every byte encoded", filename: strings.Repeat("å", MaxFilenameBytes/2)},
		{name: "a quote, escaped", filename: `a".pdf`},
		{name: "a line break, encoded rather than ending the line", filename: "a.pdf\r\nBcc: x@example.com"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			msg, err := New(validFrom(), validTo(), "s", &Body{
				Subtype: "mixed",
				Parts:   []*Body{Text("see attached"), Attachment(testCase.filename, "application/pdf", []byte("%PDF"))},
			})
			if err != nil {
				t.Fatalf("%s: New: %v", testCase.name, err)
			}
			out, err := msg.String()
			if err != nil {
				t.Fatalf("%s: String: %v", testCase.name, err)
			}
			for index, line := range strings.Split(out, "\r\n") {
				if len(line) > 998 {
					t.Errorf("%s: line %d is %d octets", testCase.name, index, len(line))
				}
			}

			parsed, err := mail.ReadMessage(strings.NewReader(out))
			if err != nil {
				t.Fatalf("%s: read message: %v", testCase.name, err)
			}
			if bcc := parsed.Header.Get("Bcc"); bcc != "" {
				t.Errorf("%s: a Bcc header was added: %q", testCase.name, bcc)
			}
			_, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
			if err != nil {
				t.Fatalf("%s: parse media type: %v", testCase.name, err)
			}
			reader := multipart.NewReader(parsed.Body, params["boundary"])
			var parts []*multipart.Part
			for {
				part, err := reader.NextPart()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("%s: next part: %v", testCase.name, err)
				}
				parts = append(parts, part)
			}
			if len(parts) != 2 {
				t.Fatalf("%s: %d parts, want 2", testCase.name, len(parts))
			}
			file := parts[1]
			if bcc := file.Header.Get("Bcc"); bcc != "" {
				t.Errorf("%s: a part's Bcc header was added: %q", testCase.name, bcc)
			}
			_, disposition, err := mime.ParseMediaType(file.Header.Get("Content-Disposition"))
			if err != nil {
				t.Fatalf("%s: parse disposition: %v", testCase.name, err)
			}
			_, typeParams, err := mime.ParseMediaType(file.Header.Get("Content-Type"))
			if err != nil {
				t.Fatalf("%s: parse content type: %v", testCase.name, err)
			}
			if disposition["filename"] != testCase.filename || typeParams["name"] != testCase.filename {
				t.Errorf("%s: filename %q, name %q, want %q", testCase.name, disposition["filename"], typeParams["name"], testCase.filename)
			}
		})
	}
}

func TestMessage_String_PartHeaderInjection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		part *Body
	}{
		{name: "a content type with a line break", part: &Body{Content: []byte("x"), ContentType: "text/plain\r\nBcc: x@example.com"}},
		{name: "a subtype with a line break", part: &Body{Subtype: "mixed\r\nBcc: x@example.com", Parts: []*Body{Text("x")}}},
		{name: "a header's value with a line break", part: &Body{
			Content: []byte("x"), ContentType: "text/plain",
			Headers: []*message_header.Header{{Name: "Content-Disposition", Value: "attachment\r\nBcc: x@example.com"}},
		}},
		{name: "a header's name with a colon", part: &Body{
			Content: []byte("x"), ContentType: "text/plain",
			Headers: []*message_header.Header{{Name: "Bcc: x@example.com\r\nX", Value: "y"}},
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			body := &Body{Subtype: "mixed", Parts: []*Body{Text("x"), testCase.part}}
			if _, err := New(validFrom(), validTo(), "s", body); !errors.Is(err, altshiftMailErrors.ErrHeaderInjection) {
				t.Errorf("%s: New err = %v, want header injection", testCase.name, err)
			}
			msg := &Message{From: validFrom(), To: validTo(), Subject: "s", Body: body}
			if _, err := msg.String(); !errors.Is(err, altshiftMailErrors.ErrHeaderInjection) {
				t.Errorf("%s: String err = %v, want header injection", testCase.name, err)
			}
		})
	}
}
