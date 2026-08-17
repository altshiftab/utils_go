package problem_detail

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"encoding/xml"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strconv"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
)

type Detail struct {
	Type      string         `json:"type,omitempty"`
	Title     string         `json:"title,omitempty"`
	Status    int            `json:"status,omitempty"`
	Detail    string         `json:"detail,omitempty"`
	Instance  string         `json:"instance,omitempty"`
	Extension map[string]any `json:"extension,omitempty"`
}

// isReservedMemberName reports whether name is one of the standard problem
// detail member names (RFC 9457 Section 3.2), which extensions must not
// override.
func isReservedMemberName(name string) bool {
	switch name {
	case "type", "title", "status", "detail", "instance":
		return true
	}

	return false
}

// MarshalJSON flattens the Extension map into the top-level JSON object,
// instead of nesting it under the "extension" key.
func (d *Detail) MarshalJSON() ([]byte, error) {
	if d == nil {
		return []byte("null"), nil
	}

	m := make(map[string]any, 5)

	if d.Type != "" {
		m["type"] = d.Type
	}
	if d.Title != "" {
		m["title"] = d.Title
	}
	if d.Status != 0 {
		m["status"] = d.Status
	}
	if d.Detail != "" {
		m["detail"] = d.Detail
	}
	if d.Instance != "" {
		m["instance"] = d.Instance
	}

	if ext := d.Extension; ext != nil {
		for k, v := range ext {
			if k == "" || isReservedMemberName(k) {
				continue
			}
			m[k] = v
		}
	}

	b, err := json.Marshal(m, json.Deterministic(true))
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json marshal (detail): %w", err), m)
	}
	return b, nil
}

// UnmarshalJSON populates the Detail from a flat JSON object, collecting any
// non-standard fields into the Extension map.
func (d *Detail) UnmarshalJSON(data []byte) error {
	if d == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("json unmarshal (detail): nil receiver"))
	}

	// Accept null
	if string(data) == "null" {
		*d = Detail{}
		return nil
	}

	// Decode into raw map first to separate known vs extension fields.
	var raw map[string]jsontext.Value
	if err := json.Unmarshal(data, &raw); err != nil {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("json unmarshal (detail map): %w", err),
			string(data),
		)
	}

	// If it's not an object
	if raw == nil {
		*d = Detail{}
		return nil
	}

	// Known fields
	if v, ok := raw["type"]; ok {
		if err := json.Unmarshal(v, &d.Type); err != nil {
			return altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal (type): %w", err))
		}
		delete(raw, "type")
	}

	if v, ok := raw["title"]; ok {
		if err := json.Unmarshal(v, &d.Title); err != nil {
			return altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal (title): %w", err))
		}
		delete(raw, "title")
	}

	if v, ok := raw["status"]; ok {
		var statusInt int
		if err := json.Unmarshal(v, &statusInt); err != nil {
			// Try string then convert to int
			var statusStr string
			if errStr := json.Unmarshal(v, &statusStr); errStr != nil {
				return altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal (status): %w", err))
			}
			si, convErr := strconv.Atoi(statusStr)
			if convErr != nil {
				return altshiftErrors.NewWithTrace(fmt.Errorf("atoi (status): %w", convErr), statusStr)
			}
			statusInt = si
		}
		d.Status = statusInt
		delete(raw, "status")
	}

	if v, ok := raw["detail"]; ok {
		if err := json.Unmarshal(v, &d.Detail); err != nil {
			return altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal (detail): %w", err))
		}
		delete(raw, "detail")
	}

	if v, ok := raw["instance"]; ok {
		if err := json.Unmarshal(v, &d.Instance); err != nil {
			return altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal (instance): %w", err))
		}
		delete(raw, "instance")
	}

	// Remaining fields go to Extension
	if len(raw) > 0 {
		ext := make(map[string]any, len(raw))
		for k, v := range raw {
			if k == "" {
				continue
			}
			var val any
			if err := json.Unmarshal(v, &val); err != nil {
				return altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal (extension value): %w", err), k)
			}
			ext[k] = val
		}
		d.Extension = ext
	} else {
		d.Extension = nil
	}

	return nil
}

// encodeXmlValue writes a JSON-normalized value (map[string]any, []any, or a
// scalar) as an XML element, following the RFC 9457 Appendix B conventions:
// objects become elements with one child element per key, and arrays become
// elements containing one "i" child element per item.
func encodeXmlValue(encoder *xml.Encoder, localName string, value any) error {
	startElement := xml.StartElement{Name: xml.Name{Local: localName}}

	switch typedValue := value.(type) {
	case map[string]any:
		if err := encoder.EncodeToken(startElement); err != nil {
			return altshiftErrors.NewWithTrace(fmt.Errorf("encode token (object start): %w", err), localName)
		}

		for _, key := range slices.Sorted(maps.Keys(typedValue)) {
			if err := encodeXmlValue(encoder, key, typedValue[key]); err != nil {
				return err
			}
		}

		if err := encoder.EncodeToken(xml.EndElement{Name: startElement.Name}); err != nil {
			return altshiftErrors.NewWithTrace(fmt.Errorf("encode token (object end): %w", err), localName)
		}
	case []any:
		if err := encoder.EncodeToken(startElement); err != nil {
			return altshiftErrors.NewWithTrace(fmt.Errorf("encode token (array start): %w", err), localName)
		}

		for _, item := range typedValue {
			if err := encodeXmlValue(encoder, "i", item); err != nil {
				return err
			}
		}

		if err := encoder.EncodeToken(xml.EndElement{Name: startElement.Name}); err != nil {
			return altshiftErrors.NewWithTrace(fmt.Errorf("encode token (array end): %w", err), localName)
		}
	case nil:
		if err := encoder.EncodeToken(startElement); err != nil {
			return altshiftErrors.NewWithTrace(fmt.Errorf("encode token (null start): %w", err), localName)
		}

		if err := encoder.EncodeToken(xml.EndElement{Name: startElement.Name}); err != nil {
			return altshiftErrors.NewWithTrace(fmt.Errorf("encode token (null end): %w", err), localName)
		}
	default:
		if err := encoder.EncodeElement(typedValue, startElement); err != nil {
			return altshiftErrors.NewWithTrace(fmt.Errorf("encode element: %w", err), localName, typedValue)
		}
	}

	return nil
}

func (d *Detail) MarshalXML(encoder *xml.Encoder, start xml.StartElement) error {
	if encoder == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("xml encoder"))
	}

	start.Name.Local = "problem"
	start.Name.Space = "urn:ietf:rfc:7807"

	if err := encoder.EncodeToken(start); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("encode token (start): %w", err), start)
	}

	encode := func(localName string, value any) error {
		if reflect.ValueOf(value).IsZero() {
			return nil
		}

		if err := encoder.EncodeElement(value, xml.StartElement{Name: xml.Name{Local: localName}}); err != nil {
			return altshiftErrors.NewWithTrace(fmt.Errorf("encode element: %w", err), localName, value)
		}

		return nil
	}

	if err := encode("type", d.Type); err != nil {
		return err
	}

	if err := encode("title", d.Title); err != nil {
		return err
	}

	if err := encode("status", d.Status); err != nil {
		return err
	}

	if err := encode("detail", d.Detail); err != nil {
		return err
	}

	if err := encode("instance", d.Instance); err != nil {
		return err
	}

	if extension := d.Extension; extension != nil {
		extensionBytes, err := json.Marshal(d.Extension)
		if err != nil {
			return altshiftErrors.NewWithTrace(
				fmt.Errorf("json marshal (extension): %w", err),
				extension,
			)
		}

		var extensionMap map[string]any
		if err := json.Unmarshal(extensionBytes, &extensionMap); err != nil {
			return altshiftErrors.NewWithTrace(
				fmt.Errorf("json unmarshal (extension map): %w", err),
				extensionMap,
			)
		}

		for _, key := range slices.Sorted(maps.Keys(extensionMap)) {
			if key == "" || isReservedMemberName(key) {
				continue
			}

			if err := encodeXmlValue(encoder, key, extensionMap[key]); err != nil {
				return err
			}
		}
	}

	if err := encoder.EncodeToken(xml.EndElement{Name: start.Name}); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("encode token (end): %w", err), start)
	}

	return nil
}

func (d *Detail) String() (string, error) {
	var text string

	if status := d.Status; status != 0 {
		text = strconv.Itoa(status)
		if title := d.Title; title != "" {
			text += fmt.Sprintf(" %s", title)
		}
	} else if title := d.Title; title != "" {
		text = title
	}

	for _, s := range []string{d.Detail, d.Type, d.Instance} {
		if s == "" {
			continue
		}

		if text != "" {
			text += "\n\n"
		}
		text += s
	}

	var extensionText string
	for _, k := range slices.Sorted(maps.Keys(d.Extension)) {
		if extensionText != "" {
			extensionText += "\n"
		}

		extensionText += fmt.Sprintf("%s:%v", k, d.Extension[k])
	}
	if extensionText != "" {
		if text != "" {
			text += "\n\n"
		}
		text += extensionText
	}

	return text, nil
}

func New(code int, options ...problem_detail_config.Option) *Detail {
	config := problem_detail_config.New(options...)
	return &Detail{
		Type:      config.Type,
		Title:     http.StatusText(code),
		Status:    code,
		Detail:    config.Detail,
		Instance:  config.Instance,
		Extension: config.Extension,
	}
}
