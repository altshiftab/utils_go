package message

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mime"
	"net/mail"
	"strings"
	"time"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	motmedelMailErrors "github.com/altshiftab/utils_go/pkg/mail/errors"
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
			return motmedelErrors.NewWithTrace(err)
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

func (message *Message) String() (string, error) {
	var builder strings.Builder
	timeNow := time.Now()

	fromMailAddress := message.From

	fmt.Fprintf(&builder, "From: %s\r\n", fromMailAddress.String())

	var toStrings []string
	for _, to := range message.To {
		if to != nil {
			toStrings = append(toStrings, to.String())
		}
	}
	fmt.Fprintf(&builder, "To: %s\r\n", strings.Join(toStrings, ", "))

	if replyTo := message.ReplyTo; len(replyTo) > 0 {
		var replyToStrings []string
		for _, replyToAddress := range replyTo {
			replyToStrings = append(replyToStrings, replyToAddress.String())
		}
		fmt.Fprintf(&builder, "Reply-To: %s\r\n", strings.Join(replyToStrings, ", "))
	}
	if cc := message.Cc; len(cc) > 0 {
		var ccStrings []string
		for _, ccAddress := range cc {
			ccStrings = append(ccStrings, ccAddress.String())
		}
		fmt.Fprintf(&builder, "Cc: %s\r\n", strings.Join(ccStrings, ", "))
	}
	if bcc := message.Bcc; len(bcc) > 0 {
		var bccStrings []string
		for _, bccAddress := range bcc {
			bccStrings = append(bccStrings, bccAddress.String())
		}
		fmt.Fprintf(&builder, "Bcc: %s\r\n", strings.Join(bccStrings, ", "))
	}
	fmt.Fprintf(&builder, "Subject: %s\r\n", message.Subject)
	fmt.Fprintf(&builder, "Date: %s\r\n", timeNow.Format(time.RFC1123Z))

	domain := message.Domain
	if domain == "" {
		fromAddress := fromMailAddress.Address
		if at := strings.LastIndex(fromAddress, "@"); at != -1 && at+1 < len(fromAddress) {
			domain = fromMailAddress.Address[at+1:]
		} else {
			return "", motmedelErrors.NewWithTrace(motmedelMailErrors.ErrBadFromAddress)
		}
	}
	if domain == "" {
		return "", motmedelErrors.NewWithTrace(empty_error.New("domain"))
	}

	messageIdRandomBuffer := make([]byte, 16)
	if _, err := rand.Read(messageIdRandomBuffer); err != nil {
		return "", motmedelErrors.NewWithTrace(fmt.Errorf("rand read: %w", err))
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
		fmt.Fprintf(&builder, "%s: %s\r\n", header.Name, header.Value)
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
		return nil, motmedelErrors.NewWithTrace(nil_error.NewWithInstance("address", "from"))
	}

	if len(to) == 0 {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("to"))
	}

	if subject == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("subject"))
	}

	if err := body.Validate(); err != nil {
		return nil, motmedelErrors.NewWithTrace(err)
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
