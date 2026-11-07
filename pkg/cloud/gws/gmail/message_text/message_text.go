// Package message_text reads a Gmail message as text: its text/plain part, or, failing one, its text/html part read by
// a reader of HTML the caller gives, as github.com/altshiftab/html_text's Text.
package message_text

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message/message_part"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

const (
	mimeTypeTextPlain = "text/plain"
	mimeTypeTextHtml  = "text/html"
)

// Text is the text of the message: its text/plain part, or, failing one, its text/html part read as text by
// htmlToText, which also reads a text/plain part that is an HTML document in fact, as some senders' are. A message with
// neither -- an image, a calendar invitation with no description -- has none, and nor has one of HTML alone when
// htmlToText is nil. A text/plain part keeps its indentation and its runs of spaces, which line its columns up.
//
// Gmail serves a part's data in the message's own character set, which for mail is overwhelmingly UTF-8; what is not
// valid UTF-8 is read as Latin-1, which is right for the rest of what arrives and readable for anything else.
func Text(msg *message.Message, htmlToText func(source string) string) string {
	if msg == nil || msg.Payload == nil {
		return ""
	}

	if part := FindTextPart(msg.Payload, mimeTypeTextPlain); part != nil {
		text := partText(part)
		if !isHtmlDocument(text) {
			return normalizePlainText(text)
		}
		if htmlToText == nil {
			return ""
		}
		return htmlToText(text)
	}

	if part := FindTextPart(msg.Payload, mimeTypeTextHtml); part != nil && htmlToText != nil {
		return htmlToText(partText(part))
	}

	return ""
}

// dropInvisible drops a soft hyphen, a combining grapheme joiner, a zero-width space and non-joiner, a word joiner and
// a byte order mark. The zero-width joiner is kept, which joins an emoji of several.
func dropInvisible(r rune) rune {
	switch r {
	case '\u00ad', '\u034f', '\u200b', '\u200c', '\u2060', '\ufeff':
		return -1
	default:
		return r
	}
}

// isHtmlDocument reports whether a text begins as an HTML document does, past blank space, a byte order mark, and
// comments.
func isHtmlDocument(text string) bool {
	start := strings.TrimLeftFunc(text, isLeadingBlank)
	for strings.HasPrefix(start, "<!--") {
		end := strings.Index(start, "-->")
		if end < 0 {
			return false
		}
		start = strings.TrimLeftFunc(start[end+len("-->"):], isLeadingBlank)
	}
	return hasPrefixFold(start, "<!doctype html") || hasPrefixFold(start, "<html")
}

func isLeadingBlank(r rune) bool {
	return unicode.IsSpace(r) || r == '\ufeff'
}

func hasPrefixFold(text string, prefix string) bool {
	return len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix)
}

// normalizePlainText is a text with no trailing space on a line and no more than one blank line in a row, its
// indentation and its runs of spaces kept, and without the characters that show nothing, with which mail pads the
// preview text an inbox shows.
func normalizePlainText(text string) string {
	text = strings.Map(dropInvisible, text)
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRightFunc(line, unicode.IsSpace)
	}
	text = blankLineRunPattern.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return strings.Trim(text, "\n")
}

// FindTextPart returns the first part of the mime type that is text rather than an attachment, searching the parts in
// the order they appear. A message that is not multipart is its own single part.
func FindTextPart(part *message_part.MessagePart, mimeType string) *message_part.MessagePart {
	if part == nil {
		return nil
	}

	if part.Filename == "" && strings.EqualFold(part.MimeType, mimeType) && part.Body != nil && part.Body.Data != "" {
		return part
	}

	for _, child := range part.Parts {
		if found := FindTextPart(child, mimeType); found != nil {
			return found
		}
	}

	return nil
}

// Decode decodes data as Gmail serves it, base64url-encoded, with or without padding.
func Decode(data string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(data, "="))
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("base64 raw url encoding decode string: %w", err))
	}
	return decoded, nil
}

// partText decodes a part's data as text, without the byte order mark it may begin with, which says nothing.
func partText(part *message_part.MessagePart) string {
	if part == nil || part.Body == nil {
		return ""
	}

	data, err := Decode(part.Body.Data)
	if err != nil {
		return ""
	}

	if utf8.Valid(data) {
		return strings.TrimPrefix(string(data), "\ufeff")
	}

	// Latin-1: every byte is the code point of the same number.
	runes := make([]rune, len(data))
	for i, b := range data {
		runes[i] = rune(b)
	}
	return string(runes)
}

// blankLineRunPattern is a run of blank lines.
var blankLineRunPattern = regexp.MustCompile(`\n{3,}`)
