package message

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftMailErrors "github.com/altshiftab/utils_go/pkg/mail/errors"
	"github.com/altshiftab/utils_go/pkg/mail/types/message/message_config"
	"github.com/altshiftab/utils_go/pkg/mail/types/message/message_header"
)

type Body struct {
	Content     []byte
	ContentType string
	Headers     []*message_header.Header
	Parts       []*Body
	Subtype     string
}

// Validate refuses a body that cannot be written: a multipart one without its subtype, a part without its content
// type, and a part whose headers would end their lines (ErrHeaderInjection).
func (body *Body) Validate() error {
	if body == nil {
		return nil
	}
	if err := body.checkLines(); err != nil {
		return err
	}
	if len(body.Parts) > 0 {
		if body.Subtype == "" {
			return empty_error.New("body subtype")
		}
		for _, part := range body.Parts {
			if part == nil {
				continue
			}
			if err := part.Validate(); err != nil {
				return err
			}
		}
		return nil
	}
	if body.ContentType == "" {
		return empty_error.New("content type")
	}
	return nil
}

// checkLines refuses the part's own headers where one would end its line: its content type, its multipart subtype, and
// the headers of its own. What follows a line break is read as a header no one asked for, or as the part's content.
func (body *Body) checkLines() error {
	if strings.ContainsAny(body.ContentType, "\r\n") || strings.ContainsAny(body.Subtype, "\r\n") {
		return fmt.Errorf("%w: a part's content type has a line break", altshiftMailErrors.ErrHeaderInjection)
	}
	for _, header := range body.Headers {
		if header == nil {
			continue
		}
		if strings.ContainsAny(header.Name, "\r\n:") {
			return fmt.Errorf("%w: a part's header's name has a line break or a colon", altshiftMailErrors.ErrHeaderInjection)
		}
		if strings.ContainsAny(header.Value, "\r\n") {
			return fmt.Errorf("%w: a part's %s header's value has a line break", altshiftMailErrors.ErrHeaderInjection, header.Name)
		}
	}
	return nil
}

func (body *Body) writeMIME(builder *strings.Builder) error {
	if len(body.Parts) > 0 {
		boundary, err := makeBoundary()
		if err != nil {
			return altshiftErrors.NewWithTrace(err)
		}
		fmt.Fprintf(builder, "Content-Type: multipart/%s; boundary=%q\r\n", body.Subtype, boundary)
		body.writeExtraHeaders(builder)
		builder.WriteString("\r\n")
		for _, part := range body.Parts {
			if part == nil {
				continue
			}
			fmt.Fprintf(builder, "--%s\r\n", boundary)
			if err := part.writeMIME(builder); err != nil {
				return err
			}
			builder.WriteString("\r\n")
		}
		fmt.Fprintf(builder, "--%s--\r\n", boundary)
		return nil
	}
	fmt.Fprintf(builder, "Content-Type: %s\r\n", body.ContentType)
	body.writeExtraHeaders(builder)
	builder.WriteString("\r\n")
	builder.Write(body.Content)
	return nil
}

func (body *Body) writeExtraHeaders(builder *strings.Builder) {
	for _, header := range body.Headers {
		if header == nil {
			continue
		}
		fmt.Fprintf(builder, "%s: %s\r\n", header.Name, header.Value)
	}
}

func makeBoundary() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("rand read: %w", err)
	}
	return fmt.Sprintf("=_%x", buf), nil
}

// Text is a text/plain body of the text, quoted-printable, so that a line is never longer than mail allows and a
// character outside ASCII arrives as it was written.
func Text(text string) *Body {
	var buffer bytes.Buffer
	writer := quotedprintable.NewWriter(&buffer)
	// Writing to a bytes.Buffer does not fail.
	_, _ = writer.Write([]byte(text))
	_ = writer.Close()

	return &Body{
		Content:     buffer.Bytes(),
		ContentType: "text/plain; charset=utf-8",
		Headers:     []*message_header.Header{{Name: "Content-Transfer-Encoding", Value: "quoted-printable"}},
	}
}

// defaultContentType is what a file is sent as whose type is not known.
const defaultContentType = "application/octet-stream"

// MaxFilenameBytes bounds a file's name CheckFilename takes: written into Content-Type and Content-Disposition, its
// every byte percent-encoded at worst, it keeps either line well within the 998 octets RFC 5322 allows one.
const MaxFilenameBytes = 255

// CheckFilename refuses, as ErrValidationError, a name a file is not to be attached under: an empty one, "." or "..",
// one longer than MaxFilenameBytes or not UTF-8, and one with a control character, a format character (a bidirectional
// one, as in "invoice\u202efdp.exe", or one that shows as nothing, as a zero-width space), a line or paragraph
// separator -- each lets a name display as other than it is -- a quote, a backslash or a slash: a path's separators,
// and what Attachment would write encoded or escaped where mail clients read either differently. A name taken from
// someone else is checked before it is attached.
func CheckFilename(filename string) error {
	switch {
	case filename == "":
		return altshiftErrors.NewWithTrace(fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, empty_error.New("filename")))
	case len(filename) > MaxFilenameBytes:
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: the filename is %d bytes, more than %d", altshiftErrors.ErrValidationError, len(filename), MaxFilenameBytes),
		)
	case filename == "." || filename == "..":
		return altshiftErrors.NewWithTrace(fmt.Errorf("%w: the filename %q names no file", altshiftErrors.ErrValidationError, filename))
	case !utf8.ValidString(filename):
		return altshiftErrors.NewWithTrace(fmt.Errorf("%w: the filename %q is not UTF-8", altshiftErrors.ErrValidationError, filename))
	}
	for _, r := range filename {
		if unicode.In(r, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp) || r == '"' || r == '\\' || r == '/' {
			return altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: the filename %q has a character not taken in one: %q", altshiftErrors.ErrValidationError, filename, r),
			)
		}
	}
	return nil
}

// Attachment is a part carrying the content, base64, as a file of the content type, named filename when one is given.
// A content type that is not one, or not given, is application/octet-stream, rather than written as it is: what
// follows a line break in it would be a header of its own; one with a parameter that does not parse, or that parses but
// cannot be written again, is kept without its parameters. The name is written as a parameter of Content-Type and of
// Content-Disposition, quoted where it needs to be, and encoded as RFC 2231 has it where it is not printable ASCII.
func Attachment(filename, contentType string, content []byte) *Body {
	mediaType, params, err := mime.ParseMediaType(contentType)
	switch {
	case errors.Is(err, mime.ErrInvalidMediaParameter) && strings.Contains(mediaType, "/"):
		params = map[string]string{}
	case err != nil || !strings.Contains(mediaType, "/"):
		mediaType, params = defaultContentType, map[string]string{}
	}

	dispositionParams := map[string]string{}
	if filename != "" {
		params["name"] = filename
		dispositionParams["filename"] = filename
	}

	contentTypeValue := mime.FormatMediaType(mediaType, params)
	if contentTypeValue == "" {
		// A parameter can parse and still not be written again, as one with no name does from "*=utf-8''x".
		nameOnly := map[string]string{}
		if filename != "" {
			nameOnly["name"] = filename
		}
		contentTypeValue = mime.FormatMediaType(mediaType, nameOnly)
	}
	if contentTypeValue == "" {
		contentTypeValue = defaultContentType
	}
	dispositionValue := mime.FormatMediaType("attachment", dispositionParams)
	if dispositionValue == "" {
		dispositionValue = "attachment"
	}

	return &Body{
		Content:     base64Wrap(content),
		ContentType: contentTypeValue,
		Headers: []*message_header.Header{
			{Name: "Content-Disposition", Value: dispositionValue},
			{Name: "Content-Transfer-Encoding", Value: "base64"},
		},
	}
}

func base64Wrap(data []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(data)
	const lineLen = 76
	if len(encoded) <= lineLen {
		return []byte(encoded)
	}

	out := make([]byte, 0, len(encoded)+2*(len(encoded)/lineLen))
	for i := 0; i < len(encoded); i += lineLen {
		if i > 0 {
			out = append(out, '\r', '\n')
		}
		end := min(i+lineLen, len(encoded))
		out = append(out, encoded[i:end]...)
	}
	return out
}

type Message struct {
	From    *mail.Address
	To      []*mail.Address
	Cc      []*mail.Address
	Bcc     []*mail.Address
	Subject string
	Body    *Body
	ReplyTo []*mail.Address
	Domain  string
	Headers []*message_header.Header
	// Date is when the message is dated, in its own zone; the time it is written, if zero.
	Date time.Time
}

// foldLength is the length a header line is folded at, the one RFC 5322 recommends.
const foldLength = 78

// writeHeader writes the header folded at its spaces, so that no line is longer than foldLength where a space allows,
// and none near the 998 octets a line may hold: a References header grows with every reply in a thread. A run of
// text with no space in it is left whole.
func writeHeader(builder *strings.Builder, name string, value string) {
	builder.WriteString(name)
	builder.WriteString(":")
	lineLength := len(name) + 1
	for index, word := range strings.Split(value, " ") {
		// The first word goes on a line of its own after the name only when that is a line it fits.
		if lineLength+1+len(word) > foldLength && (index > 0 || 1+len(word) <= foldLength) {
			builder.WriteString("\r\n")
			lineLength = 0
		}
		builder.WriteString(" ")
		builder.WriteString(word)
		lineLength += 1 + len(word)
	}
	builder.WriteString("\r\n")
}

// checkHeaderValue refuses a value that would end its header line: what follows a line break is read as a header of
// its own, or as the body.
func checkHeaderValue(name string, value string) error {
	if strings.ContainsAny(value, "\r\n") {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: the %s header's value has a line break", altshiftMailErrors.ErrHeaderInjection, name),
		)
	}
	return nil
}

func (message *Message) String() (string, error) {
	if err := checkHeaderValue("Subject", message.Subject); err != nil {
		return "", err
	}
	for _, header := range message.Headers {
		if header == nil {
			continue
		}
		if err := checkHeaderValue(header.Name, header.Value); err != nil {
			return "", err
		}
		if strings.ContainsAny(header.Name, "\r\n:") {
			return "", altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: a header's name has a line break or a colon", altshiftMailErrors.ErrHeaderInjection),
			)
		}
	}
	if err := message.Body.Validate(); err != nil {
		return "", altshiftErrors.NewWithTrace(err)
	}

	var builder strings.Builder
	timeNow := time.Now()

	fromMailAddress := message.From

	writeHeader(&builder, "From", fromMailAddress.String())

	var toStrings []string
	for _, to := range message.To {
		if to != nil {
			toStrings = append(toStrings, to.String())
		}
	}
	writeHeader(&builder, "To", strings.Join(toStrings, ", "))

	if replyTo := message.ReplyTo; len(replyTo) > 0 {
		var replyToStrings []string
		for _, replyToAddress := range replyTo {
			replyToStrings = append(replyToStrings, replyToAddress.String())
		}
		writeHeader(&builder, "Reply-To", strings.Join(replyToStrings, ", "))
	}
	if cc := message.Cc; len(cc) > 0 {
		var ccStrings []string
		for _, ccAddress := range cc {
			ccStrings = append(ccStrings, ccAddress.String())
		}
		writeHeader(&builder, "Cc", strings.Join(ccStrings, ", "))
	}
	if bcc := message.Bcc; len(bcc) > 0 {
		var bccStrings []string
		for _, bccAddress := range bcc {
			bccStrings = append(bccStrings, bccAddress.String())
		}
		writeHeader(&builder, "Bcc", strings.Join(bccStrings, ", "))
	}
	// A subject outside ASCII is an encoded word (RFC 2047); one inside it is written as it is.
	writeHeader(&builder, "Subject", mime.QEncoding.Encode("utf-8", message.Subject))
	date := message.Date
	if date.IsZero() {
		date = timeNow
	}
	fmt.Fprintf(&builder, "Date: %s\r\n", date.Format(time.RFC1123Z))

	domain := message.Domain
	if domain == "" {
		fromAddress := fromMailAddress.Address
		if at := strings.LastIndex(fromAddress, "@"); at != -1 && at+1 < len(fromAddress) {
			domain = fromMailAddress.Address[at+1:]
		} else {
			return "", altshiftErrors.NewWithTrace(altshiftMailErrors.ErrBadFromAddress)
		}
	}
	if domain == "" {
		return "", altshiftErrors.NewWithTrace(empty_error.New("domain"))
	}

	messageIdRandomBuffer := make([]byte, 16)
	if _, err := rand.Read(messageIdRandomBuffer); err != nil {
		return "", altshiftErrors.NewWithTrace(fmt.Errorf("rand read: %w", err))
	}
	fmt.Fprintf(
		&builder,
		"Message-ID: <%d.%x@%s>\r\n",
		timeNow.UnixNano(), messageIdRandomBuffer, domain,
	)

	for _, header := range message.Headers {
		if header == nil {
			continue
		}
		writeHeader(&builder, header.Name, header.Value)
	}

	builder.WriteString("MIME-Version: 1.0\r\n")
	if body := message.Body; body != nil {
		if err := body.writeMIME(&builder); err != nil {
			return "", err
		}
	} else {
		builder.WriteString("\r\n")
	}

	return builder.String(), nil
}

func New(from *mail.Address, to []*mail.Address, subject string, body *Body, options ...message_config.Option) (*Message, error) {
	if from == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.NewWithInstance("address", "from"))
	}

	if len(to) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("to"))
	}

	if subject == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("subject"))
	}

	if err := body.Validate(); err != nil {
		return nil, altshiftErrors.NewWithTrace(err)
	}

	config := message_config.New(options...)

	return &Message{
		From:    from,
		To:      to,
		Cc:      config.Cc,
		Bcc:     config.Bcc,
		Subject: subject,
		Body:    body,
		ReplyTo: config.ReplyTo,
		Domain:  config.Domain,
		Headers: config.Headers,
		Date:    config.Date,
	}, nil
}
