// Package message_text reads a Gmail message as text: its text/plain part, or, failing one, its text/html part with
// the markup taken out.
package message_text

import (
	"encoding/base64"
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message"
	"github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message/message_part"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

const (
	mimeTypeTextPlain = "text/plain"
	mimeTypeTextHtml  = "text/html"
)

// Text is the text of the message: its text/plain part, or, failing one, its text/html part read as text. A message
// with neither -- an image, a calendar invitation with no description -- has none.
//
// Gmail serves a part's data in the message's own character set, which for mail is overwhelmingly UTF-8; what is not
// valid UTF-8 is read as Latin-1, which is right for the rest of what arrives and readable for anything else.
func Text(msg *message.Message) string {
	if msg == nil || msg.Payload == nil {
		return ""
	}

	if part := FindTextPart(msg.Payload, mimeTypeTextPlain); part != nil {
		return NormalizeWhitespace(partText(part))
	}

	if part := FindTextPart(msg.Payload, mimeTypeTextHtml); part != nil {
		return HtmlToText(partText(part))
	}

	return ""
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

// partText decodes a part's data as text.
func partText(part *message_part.MessagePart) string {
	if part == nil || part.Body == nil {
		return ""
	}

	data, err := Decode(part.Body.Data)
	if err != nil {
		return ""
	}

	if utf8.Valid(data) {
		return string(data)
	}

	// Latin-1: every byte is the code point of the same number.
	runes := make([]rune, len(data))
	for i, b := range data {
		runes[i] = rune(b)
	}
	return string(runes)
}

var (
	// What is dropped whole: what a browser would run or apply, and what it would not show. Spelled out per element
	// rather than with a backreference, which RE2 has not got.
	htmlDroppedPattern = regexp.MustCompile(
		`(?is)<script\b[^>]*>.*?</script\s*>|<style\b[^>]*>.*?</style\s*>|<head\b[^>]*>.*?</head\s*>|<title\b[^>]*>.*?</title\s*>|<!--.*?-->`,
	)

	// A link, whose target is kept: mail says "confirm here" and means the address behind it.
	htmlLinkPattern = regexp.MustCompile(`(?is)<a\b[^>]*?\bhref\s*=\s*(?:"([^"]*)"|'([^']*)')[^>]*>(.*?)</a\s*>`)

	// What a browser would break the line at.
	htmlLineBreakPattern = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/li|/tr|/h[1-6]|/blockquote|/pre|/table|hr)\b[^>]*>`)

	htmlTagPattern = regexp.MustCompile(`(?s)<[^>]*>`)

	// Runs of blank space within a line, and runs of blank lines.
	spaceRunPattern     = regexp.MustCompile(`[ \t\r\f\v\x{00a0}]+`)
	blankLineRunPattern = regexp.MustCompile(`\n{3,}`)
)

// linkText is a link as text: what it says, and where it goes when that is somewhere else than what it says.
func linkText(match []string) string {
	target := match[1]
	if target == "" {
		target = match[2]
	}
	target = strings.TrimSpace(html.UnescapeString(target))
	label := strings.TrimSpace(htmlTagPattern.ReplaceAllString(match[3], ""))

	lowered := strings.ToLower(target)
	if !strings.HasPrefix(lowered, "http://") && !strings.HasPrefix(lowered, "https://") && !strings.HasPrefix(lowered, "mailto:") {
		return match[3]
	}
	if html.UnescapeString(label) == target || html.UnescapeString(label) == strings.TrimPrefix(target, "mailto:") {
		return match[3]
	}
	// Escaped, the brackets outlive the stripping of the tags, and come back with the entities.
	if label == "" {
		return "&lt;" + html.EscapeString(target) + "&gt;"
	}
	return match[3] + " &lt;" + html.EscapeString(target) + "&gt;"
}

// HtmlToText is the text a page would show, near enough to read: what is not shown is dropped, a link keeps its
// target, what breaks the line breaks it, the tags go, and the entities come back as the characters they stand for.
func HtmlToText(source string) string {
	text := htmlDroppedPattern.ReplaceAllString(source, "")
	text = htmlLinkPattern.ReplaceAllStringFunc(text, func(link string) string {
		return linkText(htmlLinkPattern.FindStringSubmatch(link))
	})
	text = htmlLineBreakPattern.ReplaceAllString(text, "\n")
	text = htmlTagPattern.ReplaceAllString(text, "")
	text = html.UnescapeString(text)
	return NormalizeWhitespace(text)
}

// NormalizeWhitespace is a text with one space between words, no trailing space on a line, and no more than one blank
// line in a row.
func NormalizeWhitespace(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = spaceRunPattern.ReplaceAllString(text, " ")

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	text = strings.Join(lines, "\n")

	text = blankLineRunPattern.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}
