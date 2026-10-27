package http_context_extractor

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
	"mime"
	"net/url"
	"strings"

	"github.com/altshiftab/utils_go/pkg/http/types/http_context_extractor/http_context_extractor_config"
	"github.com/altshiftab/utils_go/pkg/schema"
)

// maskBodyFields masks what the entries matching the URL name of a body's content, as its content
// type reads. Content that is no JSON or form, or that does not parse -- a body cut at the limit is
// no longer whole JSON -- is masked whole, as is any matched by an entry naming nothing: what cannot
// be read field by field cannot be vouched for field by field.
func maskBodyFields(
	entries []*http_context_extractor_config.MaskedBodyFields,
	u *schema.Url,
	content string,
	contentType string,
) string {
	if content == "" || content == maskedValue {
		return content
	}

	fields := make(map[string]struct{})
	allValues := false
	matched := false
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if matches, _, _ := urlMatchesPattern(entry.Url, u); !matches {
			continue
		}
		matched = true
		if len(entry.Fields) == 0 && !entry.AllValues {
			return maskedValue
		}
		allValues = allValues || entry.AllValues
		for _, field := range entry.Fields {
			fields[field] = struct{}{}
		}
	}
	if !matched {
		return content
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return maskedValue
	}

	var masked string
	switch {
	case mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"):
		masked, err = maskJsonFields(content, fields, allValues)
	case mediaType == "application/x-www-form-urlencoded":
		masked, err = maskFormFields(content, fields, allValues)
	default:
		return maskedValue
	}
	if err != nil {
		return maskedValue
	}

	return masked
}

// maskJsonFields replaces the value of every member named in fields, at any depth, with
// maskedValue, or with allValues every string and number wherever it is, keeping the member names,
// their order and the rest as they were.
func maskJsonFields(content string, fields map[string]struct{}, allValues bool) (string, error) {
	decoder := jsontext.NewDecoder(strings.NewReader(content))
	var masked bytes.Buffer
	encoder := jsontext.NewEncoder(&masked)

	for {
		token, err := decoder.ReadToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}

		// A string read where an object expects a member's name is that name.
		kind, length := decoder.StackIndex(decoder.StackDepth())
		isName := kind == '{' && length%2 == 1 && token.Kind() == '"'

		if isName {
			// Read before anything else of the decoder's, which voids the token.
			_, isMasked := fields[token.String()]
			if err := encoder.WriteToken(token); err != nil {
				return "", err
			}
			if isMasked {
				if _, err := decoder.ReadValue(); err != nil {
					return "", err
				}
				if err := encoder.WriteToken(jsontext.String(maskedValue)); err != nil {
					return "", err
				}
			}
			continue
		}

		if allValues && (token.Kind() == '"' || token.Kind() == '0') {
			token = jsontext.String(maskedValue)
		}
		if err := encoder.WriteToken(token); err != nil {
			return "", err
		}
	}

	// The encoder ends a top-level value with a newline, which is no part of it.
	return strings.TrimSuffix(masked.String(), "\n"), nil
}

// maskFormFields replaces the value of every key named in fields, or with allValues of every key,
// with maskedValue, keeping the pairs in their order and the rest as they were.
func maskFormFields(content string, fields map[string]struct{}, allValues bool) (string, error) {
	if _, err := url.ParseQuery(content); err != nil {
		return "", err
	}

	pairs := strings.Split(content, "&")
	for i, pair := range pairs {
		key, _, _ := strings.Cut(pair, "=")
		name, err := url.QueryUnescape(key)
		if err != nil {
			return "", err
		}
		if _, isMasked := fields[name]; isMasked || allValues {
			pairs[i] = key + "=" + url.QueryEscape(maskedValue)
		}
	}

	return strings.Join(pairs, "&"), nil
}
