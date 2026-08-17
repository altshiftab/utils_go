package problem_detail

import (
	"encoding/json/v2"
	"encoding/xml"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
)

type testValidationError struct {
	Error            string `json:"error"`
	InstanceLocation string `json:"instanceLocation"`
	KeywordLocation  string `json:"keywordLocation"`
}

func TestDetail_MarshalXML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		detail *Detail
		want   string
	}{
		{
			name: "standard fields only",
			detail: &Detail{
				Type:     "https://example.com/probs/out-of-credit",
				Title:    "You do not have enough credit.",
				Status:   403,
				Detail:   "Your current balance is 30, but that costs 50.",
				Instance: "https://example.net/account/12345/msgs/abc",
			},
			want: `<problem xmlns="urn:ietf:rfc:7807">` +
				`<type>https://example.com/probs/out-of-credit</type>` +
				`<title>You do not have enough credit.</title>` +
				`<status>403</status>` +
				`<detail>Your current balance is 30, but that costs 50.</detail>` +
				`<instance>https://example.net/account/12345/msgs/abc</instance>` +
				`</problem>`,
		},
		{
			name: "extension with array of structs",
			detail: &Detail{
				Title:    "Unprocessable Entity",
				Status:   422,
				Detail:   "Invalid body.",
				Instance: "d5ca2a79-be9f-475f-b9d3-c59ddec8e9af",
				Extension: map[string]any{
					"errors": []*testValidationError{
						{
							Error:            `value "" too short for "minLength" argument 1`,
							InstanceLocation: "#/name",
							KeywordLocation:  "#/properties/name/minLength",
						},
						{
							Error:            `unknown property "email_address"`,
							InstanceLocation: "#",
							KeywordLocation:  "#/additionalProperties",
						},
					},
				},
			},
			want: `<problem xmlns="urn:ietf:rfc:7807">` +
				`<title>Unprocessable Entity</title>` +
				`<status>422</status>` +
				`<detail>Invalid body.</detail>` +
				`<instance>d5ca2a79-be9f-475f-b9d3-c59ddec8e9af</instance>` +
				`<errors>` +
				`<i>` +
				`<error>value &#34;&#34; too short for &#34;minLength&#34; argument 1</error>` +
				`<instanceLocation>#/name</instanceLocation>` +
				`<keywordLocation>#/properties/name/minLength</keywordLocation>` +
				`</i>` +
				`<i>` +
				`<error>unknown property &#34;email_address&#34;</error>` +
				`<instanceLocation>#</instanceLocation>` +
				`<keywordLocation>#/additionalProperties</keywordLocation>` +
				`</i>` +
				`</errors>` +
				`</problem>`,
		},
		{
			name: "extension with nested object, scalars and null",
			detail: &Detail{
				Title:  "Forbidden",
				Status: 403,
				Extension: map[string]any{
					"balance": 30,
					"accounts": []any{
						"https://example.net/account/12345",
						"https://example.net/account/67890",
					},
					"customer": map[string]any{"id": "c112eeff0d810a8d", "locked": false},
					"reason":   nil,
				},
			},
			want: `<problem xmlns="urn:ietf:rfc:7807">` +
				`<title>Forbidden</title>` +
				`<status>403</status>` +
				`<accounts>` +
				`<i>https://example.net/account/12345</i>` +
				`<i>https://example.net/account/67890</i>` +
				`</accounts>` +
				`<balance>30</balance>` +
				`<customer>` +
				`<id>c112eeff0d810a8d</id>` +
				`<locked>false</locked>` +
				`</customer>` +
				`<reason></reason>` +
				`</problem>`,
		},
		{
			name: "reserved and empty extension keys are skipped",
			detail: &Detail{
				Title:  "Bad Request",
				Status: 400,
				Extension: map[string]any{
					"title": "shadow",
					"":      "x",
					"ok":    "v",
				},
			},
			want: `<problem xmlns="urn:ietf:rfc:7807">` +
				`<title>Bad Request</title>` +
				`<status>400</status>` +
				`<ok>v</ok>` +
				`</problem>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := xml.Marshal(tt.detail)
			if err != nil {
				t.Fatalf("xml marshal: %v", err)
			}

			if string(got) != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestDetail_MarshalXML_Errors(t *testing.T) {
	t.Parallel()

	t.Run("nil encoder", func(t *testing.T) {
		t.Parallel()

		if err := (&Detail{}).MarshalXML(nil, xml.StartElement{}); err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("unmarshalable extension value", func(t *testing.T) {
		t.Parallel()

		detail := &Detail{Status: 500, Extension: map[string]any{"ch": make(chan int)}}
		if _, err := xml.Marshal(detail); err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("out-of-range extension number", func(t *testing.T) {
		t.Parallel()

		detail := &Detail{Status: 500, Extension: map[string]any{"n": outOfRangeNumber{}}}
		if _, err := xml.Marshal(detail); err == nil {
			t.Error("expected an error")
		}
	})
}

// outOfRangeNumber marshals to syntactically valid JSON that cannot be
// unmarshaled back into a float64.
type outOfRangeNumber struct{}

func (outOfRangeNumber) MarshalJSON() ([]byte, error) {
	return []byte("1e999"), nil
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("write error")
}

// poisonedEncoder returns an encoder whose buffered writer has already
// recorded a write error, making every subsequent token write fail.
// EncodeElement flushes to the underlying writer, planting the error.
func poisonedEncoder() *xml.Encoder {
	encoder := xml.NewEncoder(errorWriter{})
	_ = encoder.EncodeElement("pad", xml.StartElement{Name: xml.Name{Local: "pad"}})
	return encoder
}

func TestDetail_MarshalXML_WriteErrors(t *testing.T) {
	t.Parallel()

	t.Run("root start token", func(t *testing.T) {
		t.Parallel()

		detail := &Detail{Status: 500}
		if err := detail.MarshalXML(poisonedEncoder(), xml.StartElement{}); err == nil {
			t.Error("expected an error")
		}
	})

	// EncodeElement flushes to the failing writer, so each field's element
	// is the first write error to surface.
	fieldDetails := map[string]*Detail{
		"type":      {Type: "t"},
		"title":     {Title: "T"},
		"status":    {Status: 500},
		"detail":    {Detail: "D"},
		"instance":  {Instance: "i"},
		"extension": {Extension: map[string]any{"foo": "bar"}},
	}
	for name, detail := range fieldDetails {
		t.Run(name+" element", func(t *testing.T) {
			t.Parallel()

			encoder := xml.NewEncoder(errorWriter{})
			if err := detail.MarshalXML(encoder, xml.StartElement{}); err == nil {
				t.Error("expected an error")
			}
		})
	}

	// End tokens only reach the failing writer when the encoder's internal
	// 4096-byte buffer overflows mid-write, so sweep the content size to
	// land the overflow on end token and closing tag writes as well.
	t.Run("token writes via buffer overflow", func(t *testing.T) {
		t.Parallel()

		for padLength := range 7 {
			for itemCount := 574; itemCount <= 584; itemCount++ {
				detail := &Detail{
					Extension: map[string]any{
						"a" + strings.Repeat("p", padLength): make([]any, itemCount),
					},
				}

				encoder := xml.NewEncoder(errorWriter{})
				if err := encoder.Encode(detail); err == nil {
					t.Errorf(
						"expected an error (pad length %d, item count %d)",
						padLength,
						itemCount,
					)
				}
			}
		}
	})
}

func TestEncodeXmlValue_WriteErrors(t *testing.T) {
	t.Parallel()

	values := map[string]any{
		"object": map[string]any{"a": 1},
		"array":  []any{1},
		"null":   nil,
		"scalar": "x",
	}
	for name, value := range values {
		t.Run(name+" start token", func(t *testing.T) {
			t.Parallel()

			if err := encodeXmlValue(poisonedEncoder(), "v", value); err == nil {
				t.Error("expected an error")
			}
		})
	}

	// A trailing scalar flushes to the failing writer, so its error
	// propagates through the enclosing object and array cases.
	nestedValues := map[string]any{
		"object": map[string]any{"a": map[string]any{}, "b": "flushes"},
		"array":  []any{map[string]any{}, "flushes"},
	}
	for name, value := range nestedValues {
		t.Run(name+" nested error propagation", func(t *testing.T) {
			t.Parallel()

			encoder := xml.NewEncoder(errorWriter{})
			if err := encodeXmlValue(encoder, "v", value); err == nil {
				t.Error("expected an error")
			}
		})
	}

	// Object end tokens only reach the failing writer when the encoder's
	// internal 4096-byte buffer overflows mid-write; sweep the element name
	// length to land the overflow on both start and end child tag writes.
	t.Run("object token writes via buffer overflow", func(t *testing.T) {
		t.Parallel()

		children := make(map[string]any, 320)
		for i := range 320 {
			children[fmt.Sprintf("c%03d", i)] = map[string]any{}
		}

		for nameLength := 1; nameLength <= 13; nameLength++ {
			encoder := xml.NewEncoder(errorWriter{})
			err := encodeXmlValue(encoder, strings.Repeat("v", nameLength), children)
			if err == nil {
				err = encoder.Flush()
			}
			if err == nil {
				t.Errorf("expected an error (name length %d)", nameLength)
			}
		}
	})
}

func TestDetail_MarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		detail *Detail
		want   string
	}{
		{
			name: "all fields with extension flattened",
			detail: &Detail{
				Type:      "https://example.com/probs/out-of-credit",
				Title:     "Out of credit",
				Status:    403,
				Detail:    "Balance too low.",
				Instance:  "https://example.net/msgs/abc",
				Extension: map[string]any{"balance": 30},
			},
			want: `{"balance":30,"detail":"Balance too low.",` +
				`"instance":"https://example.net/msgs/abc","status":403,` +
				`"title":"Out of credit","type":"https://example.com/probs/out-of-credit"}`,
		},
		{
			name:   "zero fields omitted",
			detail: &Detail{},
			want:   `{}`,
		},
		{
			name: "reserved and empty extension keys are skipped",
			detail: &Detail{
				Title:     "Real",
				Extension: map[string]any{"title": "shadow", "": "x", "foo": "bar"},
			},
			want: `{"foo":"bar","title":"Real"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := json.Marshal(tt.detail)
			if err != nil {
				t.Fatalf("json marshal: %v", err)
			}

			if string(got) != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}

	t.Run("nil receiver", func(t *testing.T) {
		t.Parallel()

		var detail *Detail
		got, err := detail.MarshalJSON()
		if err != nil {
			t.Fatalf("json marshal: %v", err)
		}

		if string(got) != "null" {
			t.Errorf("got %q, want %q", got, "null")
		}
	})

	t.Run("unmarshalable extension value", func(t *testing.T) {
		t.Parallel()

		detail := &Detail{Extension: map[string]any{"ch": make(chan int)}}
		if _, err := json.Marshal(detail); err == nil {
			t.Error("expected an error")
		}
	})
}

func TestDetail_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    string
		initial Detail
		want    Detail
		wantErr bool
	}{
		{
			name: "flat object with known and extension fields",
			data: `{"type":"https://example.com/probs/out-of-credit","title":"Out of credit",` +
				`"status":403,"detail":"Balance too low.","instance":"https://example.net/msgs/abc",` +
				`"balance":30,"account":"12345"}`,
			want: Detail{
				Type:      "https://example.com/probs/out-of-credit",
				Title:     "Out of credit",
				Status:    403,
				Detail:    "Balance too low.",
				Instance:  "https://example.net/msgs/abc",
				Extension: map[string]any{"balance": float64(30), "account": "12345"},
			},
		},
		{
			name: "status as string",
			data: `{"status":"422"}`,
			want: Detail{Status: 422},
		},
		{
			name:    "status as invalid string",
			data:    `{"status":"abc"}`,
			wantErr: true,
		},
		{
			name:    "status as object",
			data:    `{"status":{}}`,
			wantErr: true,
		},
		{
			name:    "type with wrong type",
			data:    `{"type":5}`,
			wantErr: true,
		},
		{
			name:    "title with wrong type",
			data:    `{"title":5}`,
			wantErr: true,
		},
		{
			name:    "detail with wrong type",
			data:    `{"detail":5}`,
			wantErr: true,
		},
		{
			name:    "instance with wrong type",
			data:    `{"instance":5}`,
			wantErr: true,
		},
		{
			name:    "null resets the detail",
			data:    `null`,
			initial: Detail{Title: "old", Extension: map[string]any{"old": true}},
			want:    Detail{},
		},
		{
			name:    "non-object input",
			data:    `[1,2]`,
			wantErr: true,
		},
		{
			name:    "no extension fields resets extension",
			data:    `{"title":"T"}`,
			initial: Detail{Extension: map[string]any{"old": true}},
			want:    Detail{Title: "T"},
		},
		{
			name: "empty extension key skipped",
			data: `{"":"x","foo":"bar"}`,
			want: Detail{Extension: map[string]any{"foo": "bar"}},
		},
		{
			name:    "out-of-range extension number",
			data:    `{"big":1e999}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			detail := tt.initial
			err := json.Unmarshal([]byte(tt.data), &detail)
			if tt.wantErr {
				if err == nil {
					t.Error("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("json unmarshal: %v", err)
			}

			if !reflect.DeepEqual(detail, tt.want) {
				t.Errorf("got:\n%#v\nwant:\n%#v", detail, tt.want)
			}
		})
	}

	t.Run("nil receiver", func(t *testing.T) {
		t.Parallel()

		var detail *Detail
		if err := detail.UnmarshalJSON([]byte(`{}`)); err == nil {
			t.Error("expected an error")
		}
	})

	// json.Unmarshal strips surrounding whitespace before invoking
	// UnmarshalJSON, so the nil raw map is only reachable directly.
	t.Run("null with whitespace", func(t *testing.T) {
		t.Parallel()

		detail := Detail{Title: "old"}
		if err := detail.UnmarshalJSON([]byte(" null ")); err != nil {
			t.Fatalf("json unmarshal: %v", err)
		}

		if !reflect.DeepEqual(detail, Detail{}) {
			t.Errorf("got %#v, want a zero detail", detail)
		}
	})
}

func TestDetail_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := &Detail{
		Type:      "https://example.com/probs/out-of-credit",
		Title:     "Out of credit",
		Status:    403,
		Detail:    "Balance too low.",
		Instance:  "https://example.net/msgs/abc",
		Extension: map[string]any{"account": "12345"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}

	var decoded Detail
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	if !reflect.DeepEqual(decoded, *original) {
		t.Errorf("got:\n%#v\nwant:\n%#v", decoded, *original)
	}
}

func TestDetail_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		detail Detail
		want   string
	}{
		{
			name:   "status and title",
			detail: Detail{Status: 422, Title: "Unprocessable Entity"},
			want:   "422 Unprocessable Entity",
		},
		{
			name:   "status only",
			detail: Detail{Status: 500},
			want:   "500",
		},
		{
			name:   "title fallback without status",
			detail: Detail{Title: "Some Problem", Detail: "It broke."},
			want:   "Some Problem\n\nIt broke.",
		},
		{
			name: "body fields joined",
			detail: Detail{
				Status:   403,
				Title:    "Forbidden",
				Detail:   "No credit.",
				Type:     "https://example.com/probs/out-of-credit",
				Instance: "abc",
			},
			want: "403 Forbidden\n\nNo credit.\n\nhttps://example.com/probs/out-of-credit\n\nabc",
		},
		{
			name:   "extension lines sorted",
			detail: Detail{Status: 403, Extension: map[string]any{"b": 1, "a": "x"}},
			want:   "403\n\na:x\nb:1",
		},
		{
			name:   "empty detail",
			detail: Detail{},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.detail.String()
			if err != nil {
				t.Fatalf("string: %v", err)
			}

			if got != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("defaults", func(t *testing.T) {
		t.Parallel()

		detail := New(404)
		if detail.Status != 404 {
			t.Errorf("got status %d, want 404", detail.Status)
		}
		if detail.Title != "Not Found" {
			t.Errorf("got title %q, want %q", detail.Title, "Not Found")
		}
		if detail.Instance == "" {
			t.Error("expected a generated instance")
		}
		if detail.Type != "" || detail.Detail != "" || detail.Extension != nil {
			t.Errorf("expected zero type, detail and extension, got %#v", detail)
		}
	})

	t.Run("with options", func(t *testing.T) {
		t.Parallel()

		extension := map[string]any{"foo": "bar"}
		detail := New(
			500,
			problem_detail_config.WithType("https://example.com/probs/oops"),
			problem_detail_config.WithDetail("boom"),
			problem_detail_config.WithInstance("inst-1"),
			problem_detail_config.WithExtension(extension),
		)

		want := &Detail{
			Type:      "https://example.com/probs/oops",
			Title:     "Internal Server Error",
			Status:    500,
			Detail:    "boom",
			Instance:  "inst-1",
			Extension: extension,
		}
		if !reflect.DeepEqual(detail, want) {
			t.Errorf("got:\n%#v\nwant:\n%#v", detail, want)
		}
	})
}
