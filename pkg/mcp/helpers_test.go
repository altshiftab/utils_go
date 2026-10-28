package mcp

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"reflect"
	"testing"
)

func TestCallerOf(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		// set is whether the context carries caller at all.
		set     bool
		caller  *Caller
		want    *Caller
		wantErr bool
	}{
		{name: "a caller", set: true, caller: &Caller{Account: "a", ClientId: "c"}, want: &Caller{Account: "a", ClientId: "c"}},
		{name: "none", wantErr: true},
		{name: "a nil caller", set: true, wantErr: true},
		{name: "a caller of no account", set: true, caller: &Caller{ClientId: "c"}, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			if testCase.set {
				ctx = WithCaller(ctx, testCase.caller)
			}
			got, err := CallerOf(ctx)
			if (err != nil) != testCase.wantErr || !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("%s: %+v, %v", testCase.name, got, err)
			}
		})
	}
}

func TestReadArguments(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		arguments string
		want      string
		wantErr   bool
	}{
		{name: "the members taken", arguments: `{"say":"hello"}`, want: "hello"},
		{name: "none", arguments: `{}`},
		{name: "a member not taken", arguments: `{"say":"hello","shout":true}`, wantErr: true},
		{name: "a member of another type", arguments: `{"say":1}`, wantErr: true},
		{name: "no object", arguments: `[]`, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var input struct {
				Say string `json:"say"`
			}
			err := ReadArguments(jsontext.Value(testCase.arguments), &input)
			if testCase.wantErr {
				if _, ok := errors.AsType[*ToolError](err); !ok {
					t.Errorf("%s: %v, want a *ToolError", testCase.name, err)
				}
				return
			}
			if err != nil || input.Say != testCase.want {
				t.Errorf("%s: %q, %v", testCase.name, input.Say, err)
			}
		})
	}
}

func TestSchemas(t *testing.T) {
	t.Parallel()

	property := PropertySchema("integer", "How many.", map[string]any{"minimum": 1})
	if !reflect.DeepEqual(property, map[string]any{"type": "integer", "description": "How many.", "minimum": 1}) {
		t.Errorf("property: %v", property)
	}

	testCases := []struct {
		name     string
		required []string
		want     map[string]any
	}{
		{name: "with required", required: []string{"count"}, want: map[string]any{
			"type": "object", "properties": map[string]any{"count": property}, "additionalProperties": false, "required": []string{"count"},
		}},
		{name: "none required", want: map[string]any{
			"type": "object", "properties": map[string]any{"count": property}, "additionalProperties": false,
		}},
	}
	for _, testCase := range testCases {
		if got := ObjectSchema(map[string]any{"count": property}, testCase.required...); !reflect.DeepEqual(got, testCase.want) {
			t.Errorf("%s: %v", testCase.name, got)
		}
	}

	if annotations := ReadOnly("Look"); !annotations.ReadOnlyHint || !annotations.IdempotentHint || annotations.DestructiveHint || annotations.Title != "Look" {
		t.Errorf("read only: %+v", annotations)
	}
}
