package types

import (
	"reflect"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/type_export/types/shape"
	"github.com/altshiftab/utils_go/pkg/type_export/types/type_declaration"

	typeExportContext "github.com/altshiftab/utils_go/pkg/type_export/types/context"
)

type genericValue[V any] struct {
	Mapping map[string]V `json:"mapping"`
}

type genericKey[K comparable] struct {
	Mapping map[K]string `json:"mapping"`
}

type genericKeyValue[K comparable, V any] struct {
	Mapping map[K]V `json:"mapping"`
}

type genericPair[T any, U any] struct {
	First  T   `json:"first"`
	Second []U `json:"second"`
}

type genericPointerSlice[T any] struct {
	Pointer *T  `json:"pointer"`
	Slice   []T `json:"slice"`
}

func renderTS(t *testing.T, rt reflect.Type) string {
	t.Helper()
	ctx := &Context{Context: typeExportContext.New()}
	if err := ctx.Add(rt); err != nil {
		t.Fatalf("ctx.Add: %v", err)
	}
	out, err := ctx.Render()
	if err != nil {
		t.Fatalf("ctx.Render: %v", err)
	}
	return out
}

func TestRenderGenericMapValueOnly(t *testing.T) {
	t.Parallel()

	out := renderTS(t, reflect.TypeFor[genericValue[int]]())

	if !strings.Contains(out, "<V>") {
		t.Errorf("expected interface to declare type parameter <V>, got:\n%s", out)
	}
	if !strings.Contains(out, "{ [key: string]: V }") {
		t.Errorf("expected map index `string` and value `V`, got:\n%s", out)
	}
}

func TestRenderGenericTwoParams(t *testing.T) {
	t.Parallel()

	out := renderTS(t, reflect.TypeFor[genericPair[int, string]]())

	if !strings.Contains(out, "<T, U>") {
		t.Errorf("expected <T, U>, got:\n%s", out)
	}
	if !strings.Contains(out, "first: T") {
		t.Errorf("expected `first: T`, got:\n%s", out)
	}
	if !strings.Contains(out, "second: U[]") {
		t.Errorf("expected `second: U[]`, got:\n%s", out)
	}
}

func TestRenderGenericPointerAndSlice(t *testing.T) {
	t.Parallel()

	// Both fields share the single type parameter T — Pointer becomes T,
	// Slice becomes T[]. This exercises the same param appearing under two
	// different field names.
	out := renderTS(t, reflect.TypeFor[genericPointerSlice[int]]())

	if !strings.Contains(out, "<T>") {
		t.Errorf("expected <T>, got:\n%s", out)
	}
	if !strings.Contains(out, "pointer: T;") {
		t.Errorf("expected `pointer: T;`, got:\n%s", out)
	}
	if !strings.Contains(out, "slice: T[]") {
		t.Errorf("expected `slice: T[]`, got:\n%s", out)
	}
}

// addAndGetInterface registers the type with a fresh context and returns the
// resulting InterfaceDeclaration. Used by tests that need to assert on the
// discovered GenericTypeInfo without rendering all the way to a string.
func addAndGetInterface(t *testing.T, rt reflect.Type) *type_declaration.InterfaceDeclaration {
	t.Helper()
	ctx := &Context{Context: typeExportContext.New()}
	if err := ctx.Add(rt); err != nil {
		t.Fatalf("ctx.Add: %v", err)
	}
	decl, ok := ctx.TypeDeclarations[rt]
	if !ok {
		t.Fatalf("type declaration not registered for %v", rt)
	}
	iface, ok := decl.(*type_declaration.InterfaceDeclaration)
	if !ok {
		t.Fatalf("expected InterfaceDeclaration, got %T", decl)
	}
	return iface
}

func TestGenericMapKeyValueBothGeneric_DataLayer(t *testing.T) {
	t.Parallel()

	// This is the case the bug fix targets: a single field whose generic
	// instantiation has BOTH a generic key and a generic value. Before the
	// fix, only one of K / V would be recorded; after the fix, the
	// FieldNameToShapes entry for the field carries both the KindMapKey
	// shape (with Param=K) and the KindMapValue shape (with Param=V).
	//
	// We assert at the data layer rather than rendering to a string,
	// because MapType.String() has a separate, pre-existing limitation
	// that rejects non-`string`/`number` index types — including type
	// parameters. That's a different issue and not part of this fix.
	iface := addAndGetInterface(t, reflect.TypeFor[genericKeyValue[string, int]]())

	if iface.GenericTypeInfo == nil {
		t.Fatal("expected GenericTypeInfo to be populated")
	}
	if got := len(iface.GenericTypeInfo.TypeParameterNames); got != 2 {
		t.Fatalf("expected 2 type parameters, got %d", got)
	}

	shapes := iface.GenericTypeInfo.FieldNameToShapes["Mapping"]
	if len(shapes) != 2 {
		t.Fatalf("expected two shapes for Mapping (key + value), got %d: %v", len(shapes), shapes)
	}

	var keyParam, valueParam string
	for _, s := range shapes {
		//exhaustive:ignore
		switch s.Kind {
		case shape.KindMapKey:
			keyParam = s.Param
		case shape.KindMapValue:
			valueParam = s.Param
		default:
			t.Errorf("unexpected shape kind %v for Mapping", s.Kind)
		}
	}
	if keyParam == "" {
		t.Error("expected a KindMapKey shape for Mapping")
	}
	if valueParam == "" {
		t.Error("expected a KindMapValue shape for Mapping")
	}
	if keyParam == valueParam {
		t.Errorf("expected key and value to be different params, both got %q", keyParam)
	}

	// Both type parameters must point at this field.
	for _, paramName := range iface.GenericTypeInfo.TypeParameterNames {
		if got := iface.GenericTypeInfo.TypeParameterNameToFieldName[paramName]; got != "Mapping" {
			t.Errorf("expected param %s → Mapping, got %q", paramName, got)
		}
	}
}

type simpleInterface struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Done  bool   `json:"done"`
}

func TestRenderSimpleInterface(t *testing.T) {
	t.Parallel()

	out := renderTS(t, reflect.TypeFor[simpleInterface]())

	for _, want := range []string{
		"export interface SimpleInterface",
		"name: string",
		"count: number",
		"done: boolean",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

type withOptional struct {
	Required string `json:"required"`
	Maybe    string `json:"maybe,omitempty"`
	Skipped  string `json:"-"`
}

func TestRenderOptionalAndSkipped(t *testing.T) {
	t.Parallel()

	out := renderTS(t, reflect.TypeFor[withOptional]())

	if !strings.Contains(out, "maybe?:") {
		t.Errorf("expected `maybe?:` (optional marker), got:\n%s", out)
	}
	if !strings.Contains(out, "required:") || strings.Contains(out, "required?:") {
		t.Errorf("`required` should not be optional, got:\n%s", out)
	}
	if strings.Contains(out, "Skipped") || strings.Contains(out, `"-"`) {
		t.Errorf("json:\"-\" field should be omitted, got:\n%s", out)
	}
}

type withMapAndSlice struct {
	Mapping map[string]int `json:"mapping"`
	Items   []string       `json:"items"`
}

func TestRenderMapAndSlice(t *testing.T) {
	t.Parallel()

	out := renderTS(t, reflect.TypeFor[withMapAndSlice]())

	if !strings.Contains(out, "{ [key: string]: number }") {
		t.Errorf("expected map rendered as `{ [key: string]: number }`, got:\n%s", out)
	}
	if !strings.Contains(out, "items: string[]") {
		t.Errorf("expected slice rendered as `string[]`, got:\n%s", out)
	}
}

type EmbeddedBase struct {
	BaseField string `json:"base_field"`
}

type withEmbedded struct {
	EmbeddedBase
	OwnField int `json:"own_field"`
}

func TestRenderEmbeddedStruct(t *testing.T) {
	t.Parallel()

	out := renderTS(t, reflect.TypeFor[withEmbedded]())

	// Both fields from the embedded struct and the outer struct should appear
	// in the same interface — embedded structs are flattened into the outer
	// type, mirroring json.Marshal semantics.
	if !strings.Contains(out, "base_field: string") {
		t.Errorf("expected base_field from embedded struct, got:\n%s", out)
	}
	if !strings.Contains(out, "own_field: number") {
		t.Errorf("expected own_field from outer struct, got:\n%s", out)
	}
}

type MyInt int

type withTypeAlias struct {
	Count MyInt `json:"count"`
}

func TestRenderPrimitiveTypeAlias(t *testing.T) {
	t.Parallel()

	// A field whose Go type is a named primitive (e.g. `type MyInt int`)
	// should both reference that name and emit a standalone
	// `export type MyInt = number;` declaration.
	out := renderTS(t, reflect.TypeFor[withTypeAlias]())

	if !strings.Contains(out, "count: MyInt") {
		t.Errorf("expected count to reference MyInt, got:\n%s", out)
	}
	if !strings.Contains(out, "export type MyInt = number;") {
		t.Errorf("expected `export type MyInt = number;` declaration, got:\n%s", out)
	}
}

type nestedInner struct {
	Value string `json:"value"`
}

type nestedOuter struct {
	Inner nestedInner `json:"inner"`
}

func TestRenderNamedStructFieldDoesNotPanic(t *testing.T) {
	t.Parallel()

	// Regression: GetTypeScriptType used to apply the type-alias path to any
	// named type, including struct types whose TypeDeclaration is an
	// *InterfaceDeclaration. That produced a "convert: conversion not ok:
	// *type_declaration.InterfaceDeclaration" error during rendering.
	out := renderTS(t, reflect.TypeFor[nestedOuter]())

	if !strings.Contains(out, "inner: NestedInner") {
		t.Errorf("expected inner field to reference NestedInner, got:\n%s", out)
	}
}

func TestGenericMapKeyOnly_DataLayer(t *testing.T) {
	t.Parallel()

	// Sanity check that map[K]string still works (only one shape, KindMapKey)
	// and was not broken by changing the underlying representation to a slice.
	iface := addAndGetInterface(t, reflect.TypeFor[genericKey[string]]())

	shapes := iface.GenericTypeInfo.FieldNameToShapes["Mapping"]
	if len(shapes) != 1 {
		t.Fatalf("expected one shape for Mapping, got %d: %v", len(shapes), shapes)
	}
	if shapes[0].Kind != shape.KindMapKey {
		t.Errorf("expected KindMapKey, got %v", shapes[0].Kind)
	}
}

type withBinary struct {
	Content   []byte   `json:"content"`
	Documents [][]byte `json:"documents"`
}

func TestRenderByteSlice(t *testing.T) {
	t.Parallel()

	// []byte carries binary data and should render as Uint8Array, not number[].
	out := renderTS(t, reflect.TypeFor[withBinary]())

	if !strings.Contains(out, "content: Uint8Array") {
		t.Errorf("expected content to be Uint8Array, got:\n%s", out)
	}
	if !strings.Contains(out, "documents: Uint8Array[]") {
		t.Errorf("expected documents to be Uint8Array[], got:\n%s", out)
	}
}

// fakeDeclaration is a minimal TypeDeclaration for exercising TypeReference
// rendering without wiring up a full context.
type fakeDeclaration string

func (f fakeDeclaration) TypeReference() *TypeReference { return &TypeReference{TypeDeclaration: f} }

func (f fakeDeclaration) QualifiedName() string { return string(f) }

func TestTypeString(t *testing.T) {
	t.Parallel()

	badType := &MapType{IndexType: Boolean, ValueType: String}

	testCases := []struct {
		name        string
		input       Type
		expected    string
		expectError bool
	}{
		{name: "basic type", input: String, expected: "string"},
		{name: "type parameter", input: &TypeParameter{Identifier: "T"}, expected: "T"},
		{
			name:     "type reference without arguments",
			input:    &TypeReference{TypeDeclaration: fakeDeclaration("Foo")},
			expected: "Foo",
		},
		{
			name: "type reference with arguments",
			input: &TypeReference{
				TypeDeclaration: fakeDeclaration("Foo"),
				TypeArguments:   []Type{String, Number},
			},
			expected: "Foo<string, number>",
		},
		{
			name: "type reference with failing argument",
			input: &TypeReference{
				TypeDeclaration: fakeDeclaration("Foo"),
				TypeArguments:   []Type{badType},
			},
			expectError: true,
		},
		{name: "union type", input: UnionType{Types: []Type{String, Null}}, expected: "string | null"},
		{name: "union type with failing member", input: UnionType{Types: []Type{badType}}, expectError: true},
		{name: "array type", input: &ArrayType{ItemsType: String}, expected: "string[]"},
		{
			name:     "array of union is parenthesized",
			input:    &ArrayType{ItemsType: &UnionType{Types: []Type{String, Null}}},
			expected: "(string | null)[]",
		},
		{name: "array type with failing items", input: &ArrayType{ItemsType: badType}, expectError: true},
		{
			name:     "map with number index",
			input:    &MapType{IndexType: Number, ValueType: String},
			expected: "{ [key: number]: string }",
		},
		{name: "map with unsupported index", input: badType, expectError: true},
		{
			name:        "map with failing index",
			input:       &MapType{IndexType: badType, ValueType: String},
			expectError: true,
		},
		{
			name:        "map with failing value",
			input:       &MapType{IndexType: String, ValueType: badType},
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := testCase.input.String()
			if testCase.expectError {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != testCase.expected {
				t.Errorf("String() = %q, want %q", got, testCase.expected)
			}
		})
	}
}

type withNumberKeyMap struct {
	Counts map[int]string `json:"counts"`
}

func TestRenderNumberKeyMap(t *testing.T) {
	t.Parallel()

	out := renderTS(t, reflect.TypeFor[withNumberKeyMap]())

	if !strings.Contains(out, "{ [key: number]: string }") {
		t.Errorf("expected map rendered with number index, got:\n%s", out)
	}
}
