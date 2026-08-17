package builder_test

import (
	"encoding/json/v2"
	"reflect"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/json/schema/builder"
	"github.com/altshiftab/utils_go/pkg/json/schema/draft202012"
)

type basicStruct struct {
	Name     string  `json:"name"`
	Age      int     `json:"age,omitempty"`
	Active   bool    `json:"active"`
	Score    float64 `json:"score,omitzero"`
	Hidden   string  `json:"-"`
	Untagged string
}

type innerStruct struct {
	Value string `json:"value"`
}

type nestedStruct struct {
	Inner   innerStruct       `json:"inner"`
	Items   []innerStruct     `json:"items"`
	Fixed   [3]int            `json:"fixed"`
	Labels  map[string]string `json:"labels"`
	Pointer *string           `json:"pointer"`
}

type boundsStruct struct {
	I8  int8   `json:"i8"`
	U8  uint8  `json:"u8"`
	I16 int16  `json:"i16"`
	U32 uint32 `json:"u32"`
	U   uint   `json:"u"`
}

type timeStruct struct {
	When time.Time `json:"when"`
}

type tagStruct struct {
	Kind string `json:"kind" jsonschema:"enum=a,enum=b"`
	Note string `json:"note" jsonschema:"some description"`
}

type funcFieldStruct struct {
	F func() `json:"f"`
}

type chanFieldStruct struct {
	C chan int `json:"c"`
}

func TestInfer(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		infer   func() (*draft202012.Builder, error)
		want    string
		wantErr bool
	}{
		{
			name: "basic struct with tags and optional fields",
			infer: func() (*draft202012.Builder, error) {
				return draft202012.Infer[basicStruct](draft202012.NewBuilder(), nil)
			},
			want: `{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"properties": {
					"Untagged": {"type": "string"},
					"active": {"type": "boolean"},
					"age": {"type": "integer"},
					"name": {"type": "string"},
					"score": {"type": "number"}
				},
				"additionalProperties": false,
				"required": ["name", "active", "Untagged"],
				"type": "object"
			}`,
		},
		{
			name: "nested struct with slice array map and pointer",
			infer: func() (*draft202012.Builder, error) {
				return draft202012.Infer[nestedStruct](draft202012.NewBuilder(), nil)
			},
			want: `{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"properties": {
					"fixed": {
						"maxItems": 3,
						"minItems": 3,
						"items": {"type": "integer"},
						"type": "array"
					},
					"inner": {
						"properties": {"value": {"type": "string"}},
						"additionalProperties": false,
						"required": ["value"],
						"type": "object"
					},
					"items": {
						"items": {
							"properties": {"value": {"type": "string"}},
							"additionalProperties": false,
							"required": ["value"],
							"type": "object"
						},
						"type": "array"
					},
					"labels": {
						"additionalProperties": {"type": "string"},
						"type": "object"
					},
					"pointer": {"type": ["null", "string"]}
				},
				"additionalProperties": false,
				"required": ["inner", "items", "fixed", "labels", "pointer"],
				"type": "object"
			}`,
		},
		{
			name: "integer bounds",
			infer: func() (*draft202012.Builder, error) {
				return draft202012.Infer[boundsStruct](draft202012.NewBuilder(), nil)
			},
			want: `{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"properties": {
					"i8": {"maximum": 127, "minimum": -128, "type": "integer"},
					"u8": {"maximum": 255, "minimum": 0, "type": "integer"},
					"i16": {"maximum": 32767, "minimum": -32768, "type": "integer"},
					"u32": {"maximum": 4294967295, "minimum": 0, "type": "integer"},
					"u": {"minimum": 0, "type": "integer"}
				},
				"additionalProperties": false,
				"required": ["i8", "u8", "i16", "u32", "u"],
				"type": "object"
			}`,
		},
		{
			name: "time.Time becomes string",
			infer: func() (*draft202012.Builder, error) {
				return draft202012.Infer[timeStruct](draft202012.NewBuilder(), nil)
			},
			want: `{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"properties": {
					"when": {"type": "string"}
				},
				"additionalProperties": false,
				"required": ["when"],
				"type": "object"
			}`,
		},
		{
			name: "jsonschema tag enum and description",
			infer: func() (*draft202012.Builder, error) {
				return draft202012.Infer[tagStruct](draft202012.NewBuilder(), nil)
			},
			want: `{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"properties": {
					"kind": {"enum": ["a", "b"], "type": "string"},
					"note": {"description": "some description", "type": "string"}
				},
				"additionalProperties": false,
				"required": ["kind", "note"],
				"type": "object"
			}`,
		},
		{
			name: "func field returns error",
			infer: func() (*draft202012.Builder, error) {
				return draft202012.Infer[funcFieldStruct](draft202012.NewBuilder(), nil)
			},
			wantErr: true,
		},
		{
			name: "chan field returns error",
			infer: func() (*draft202012.Builder, error) {
				return draft202012.Infer[chanFieldStruct](draft202012.NewBuilder(), nil)
			},
			wantErr: true,
		},
		{
			name: "func field ignored with IgnoreInvalidTypes",
			infer: func() (*draft202012.Builder, error) {
				return draft202012.Infer[funcFieldStruct](
					draft202012.NewBuilder(),
					&builder.InferOpts{IgnoreInvalidTypes: true},
				)
			},
			want: `{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"additionalProperties": false,
				"type": "object"
			}`,
		},
		{
			name: "plain string",
			infer: func() (*draft202012.Builder, error) {
				return draft202012.Infer[string](draft202012.NewBuilder(), nil)
			},
			want: `{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"type": "string"
			}`,
		},
		{
			name: "map of string to int",
			infer: func() (*draft202012.Builder, error) {
				return draft202012.Infer[map[string]int](draft202012.NewBuilder(), nil)
			},
			want: `{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"additionalProperties": {"type": "integer"},
				"type": "object"
			}`,
		},
		{
			name: "slice of string",
			infer: func() (*draft202012.Builder, error) {
				return draft202012.Infer[[]string](draft202012.NewBuilder(), nil)
			},
			want: `{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"items": {"type": "string"},
				"type": "array"
			}`,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			inferredBuilder, err := testCase.infer()
			if testCase.wantErr {
				if err == nil {
					t.Fatal("Infer succeeded, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Infer returned unexpected error: %v", err)
			}

			gotData, err := json.Marshal(inferredBuilder.Build())
			if err != nil {
				t.Fatalf("failed to marshal built schema: %v", err)
			}

			var got any
			if err := json.Unmarshal(gotData, &got); err != nil {
				t.Fatalf("failed to unmarshal built schema JSON %s: %v", gotData, err)
			}

			var want any
			if err := json.Unmarshal([]byte(testCase.want), &want); err != nil {
				t.Fatalf("failed to unmarshal expected JSON: %v", err)
			}

			if !reflect.DeepEqual(got, want) {
				t.Errorf("built schema mismatch:\ngot:  %s\nwant: %s", gotData, testCase.want)
			}
		})
	}
}
