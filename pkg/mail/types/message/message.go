package message

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"

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

func (body *Body) Validate() error {
	if body == nil {
		return nil
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

func Attachment(filename, contentType string, content []byte) *Body {
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = contentType
		params = map[string]string{}
	}

	dispositionParams := map[string]string{}
	if filename != "" {
		params["name"] = filename
		dispositionParams["filename"] = filename
	}

	contentTypeValue := mime.FormatMediaType(mediaType, params)
	if contentTypeValue == "" {
		contentTypeValue = contentType
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
	fmt.Fprintf(&builder, "Date: %s\r\n", timeNow.Format(time.RFC1123Z))

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
	}, nil
}
