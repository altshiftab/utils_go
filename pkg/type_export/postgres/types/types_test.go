package types

import (
	"reflect"
	"strings"
	"testing"
	"time"

	typeExportContext "github.com/altshiftab/utils_go/pkg/type_export/types/context"
	"github.com/altshiftab/utils_go/pkg/type_export/types/type_declaration"
)

type pgPrimitives struct {
	S   string  `json:"s"`
	I   int     `json:"i"`
	I8  int8    `json:"i8"`
	I64 int64   `json:"i64"`
	F   float64 `json:"f"`
	B   bool    `json:"b"`
}

type pgWithTime struct {
	When time.Time `json:"when"`
}

type pgWithBytes struct {
	Blob []byte `json:"blob"`
}

type pgWithStringSlice struct {
	Tags []string `json:"tags"`
}

type pgInner struct {
	Name string `json:"name"`
}

type pgWithReference struct {
	Inner pgInner `json:"inner"`
}

type pgWithStructSlice struct {
	Items []pgInner `json:"items"`
}

type pgWithTagOverrides struct {
	ID   string `json:"id" postgres:"id,primarykey,type:uuid"`
	When string `json:"when" postgres:"when,default:'now()',unique"`
}

func renderPG(t *testing.T, values ...any) string {
	t.Helper()
	ctx := &Context{Context: typeExportContext.New()}
	if err := ctx.Add(values...); err != nil {
		t.Fatalf("ctx.Add: %v", err)
	}
	out, err := ctx.Render()
	if err != nil {
		t.Fatalf("ctx.Render: %v", err)
	}
	return out
}

func TestPrimitiveTypeMappings(t *testing.T) {
	t.Parallel()

	out := renderPG(t, reflect.TypeFor[pgPrimitives]())

	// The postgres producer uses the Go field name (capitalized) — it does
	// not consult the `json` tag for column naming.
	expectations := map[string]string{
		"S ":   "text",
		"I ":   "integer",
		"I8 ":  "smallint",
		"I64 ": "bigint",
		"F ":   "double precision",
		"B ":   "boolean",
	}
	for fieldPrefix, wantType := range expectations {
		needle := "\t" + fieldPrefix + wantType
		if !strings.Contains(out, needle) {
			t.Errorf("expected line containing %q, got:\n%s", needle, out)
		}
	}
}

func TestTimeMapsToTimestampTZ(t *testing.T) {
	t.Parallel()

	out := renderPG(t, reflect.TypeFor[pgWithTime]())
	if !strings.Contains(out, "When timestamptz") {
		t.Errorf("expected `When timestamptz`, got:\n%s", out)
	}
}

func TestByteSliceMapsToByteA(t *testing.T) {
	t.Parallel()

	out := renderPG(t, reflect.TypeFor[pgWithBytes]())
	if !strings.Contains(out, "Blob bytea") {
		t.Errorf("expected `Blob bytea`, got:\n%s", out)
	}
}

func TestStringSliceMapsToTextArray(t *testing.T) {
	t.Parallel()

	out := renderPG(t, reflect.TypeFor[pgWithStringSlice]())
	if !strings.Contains(out, "Tags text[]") {
		t.Errorf("expected `Tags text[]`, got:\n%s", out)
	}
}

func TestStructFieldBecomesReference(t *testing.T) {
	t.Parallel()

	out := renderPG(t, reflect.TypeFor[pgWithReference]())

	// The inner struct also gets its own table.
	if !strings.Contains(out, "CREATE TABLE pg_inner") {
		t.Errorf("expected nested struct's own table, got:\n%s", out)
	}
	// The outer field should reference the inner table.
	if !strings.Contains(out, "REFERENCES pg_inner(id)") {
		t.Errorf("expected REFERENCES pg_inner(id), got:\n%s", out)
	}
}

func TestStructSliceCreatesAssociativeTable(t *testing.T) {
	t.Parallel()

	out := renderPG(t, reflect.TypeFor[pgWithStructSlice]())

	// The associative table joins the two participating tables; we don't
	// assert the exact schema, just that an additional CREATE TABLE for
	// the join exists in the output.
	createCount := strings.Count(out, "CREATE TABLE ")
	if createCount < 3 {
		t.Errorf("expected at least 3 CREATE TABLE statements (outer + inner + assoc), got %d:\n%s", createCount, out)
	}
}

type pgWithSkippedReference struct {
	Name   string   `json:"name"`
	Hidden *pgInner `json:"hidden,omitzero" postgres:"-"`
}

// A struct referenced only through a `postgres:"-"` field must not be
// materialized as a table, and the field itself must not become a column.
func TestSkippedStructFieldOmitsTableAndColumn(t *testing.T) {
	t.Parallel()

	out := renderPG(t, reflect.TypeFor[pgWithSkippedReference]())

	if !strings.Contains(out, "CREATE TABLE pg_with_skipped_reference") {
		t.Errorf("expected the outer table, got:\n%s", out)
	}
	if strings.Contains(out, "pg_inner") {
		t.Errorf("expected no table/reference for a struct reached only via a skipped field, got:\n%s", out)
	}
	if strings.Contains(out, "Hidden") {
		t.Errorf("expected no column for the skipped field, got:\n%s", out)
	}
	if got := strings.Count(out, "CREATE TABLE "); got != 1 {
		t.Errorf("expected exactly 1 CREATE TABLE (the outer struct), got %d:\n%s", got, out)
	}
}

type pgWithSkippedStructSlice struct {
	Name   string    `json:"name"`
	Hidden []pgInner `json:"hidden,omitzero" postgres:"-"`
}

// A skipped struct slice must not emit an associative (join) table nor a table
// for its element type.
func TestSkippedStructSliceOmitsAssociativeTable(t *testing.T) {
	t.Parallel()

	out := renderPG(t, reflect.TypeFor[pgWithSkippedStructSlice]())

	if strings.Contains(out, "pg_inner") {
		t.Errorf("expected no element or associative table for a skipped struct slice, got:\n%s", out)
	}
	if got := strings.Count(out, "CREATE TABLE "); got != 1 {
		t.Errorf("expected exactly 1 CREATE TABLE (the outer struct), got %d:\n%s", got, out)
	}
}

func TestPostgresTagOverrides(t *testing.T) {
	t.Parallel()

	out := renderPG(t, reflect.TypeFor[pgWithTagOverrides]())

	// type:uuid should override the inferred type for the `id` field.
	if !strings.Contains(out, "id uuid") {
		t.Errorf("expected id type overridden to uuid, got:\n%s", out)
	}
	// primarykey should add the PRIMARY KEY clause.
	if !strings.Contains(out, "PRIMARY KEY") {
		t.Errorf("expected PRIMARY KEY clause, got:\n%s", out)
	}
	// default:'now()' should be emitted as DEFAULT 'now()'.
	if !strings.Contains(out, "DEFAULT 'now()'") {
		t.Errorf("expected DEFAULT 'now()', got:\n%s", out)
	}
	// unique should add the UNIQUE clause.
	if !strings.Contains(out, "UNIQUE") {
		t.Errorf("expected UNIQUE clause, got:\n%s", out)
	}
}

func TestAutoIdInsertedWhenNoPrimaryKey(t *testing.T) {
	t.Parallel()

	// pgPrimitives has no field tagged with primarykey, so the renderer
	// should append a default `id uuid PRIMARY KEY DEFAULT gen_random_uuid()` line.
	out := renderPG(t, reflect.TypeFor[pgPrimitives]())
	if !strings.Contains(out, "id uuid PRIMARY KEY DEFAULT gen_random_uuid()") {
		t.Errorf("expected auto id column, got:\n%s", out)
	}
}

func TestNotNullByDefault(t *testing.T) {
	t.Parallel()

	// Required (non-omitempty) fields should pick up NOT NULL.
	out := renderPG(t, reflect.TypeFor[pgPrimitives]())
	if !strings.Contains(out, "NOT NULL") {
		t.Errorf("expected at least one NOT NULL clause, got:\n%s", out)
	}
}

type pgGeneric[T any] struct {
	Data T `json:"data"`
}

func TestGenericTypesUnsupported(t *testing.T) {
	t.Parallel()

	ctx := &Context{Context: typeExportContext.New()}
	if err := ctx.Add(reflect.TypeFor[pgGeneric[int]]()); err != nil {
		t.Fatalf("ctx.Add: %v", err)
	}
	if _, err := ctx.Render(); err == nil {
		t.Error("expected error for generic struct, got nil")
	}
}

// pgInterfaceDeclaration registers the type with a fresh context and returns
// the postgres-level InterfaceDeclaration wrapper for it.
func pgInterfaceDeclaration(t *testing.T, rt reflect.Type) *InterfaceDeclaration {
	t.Helper()
	ctx := &Context{Context: typeExportContext.New()}
	if err := ctx.Add(rt); err != nil {
		t.Fatalf("ctx.Add: %v", err)
	}
	decl, ok := ctx.TypeDeclarations[rt]
	if !ok {
		t.Fatal("type declaration not registered")
	}
	inner, ok := decl.(*type_declaration.InterfaceDeclaration)
	if !ok {
		t.Fatalf("expected InterfaceDeclaration, got %T", decl)
	}
	return &InterfaceDeclaration{InterfaceDeclaration: inner, c: ctx}
}

type pgIdTagged struct {
	Key int64 `postgres:"id"`
}

type pgIdTypeOverride struct {
	Key string `postgres:"id,type:uuid"`
}

type pgIdSkipped struct {
	Key int64 `postgres:"-"`
}

type pgNoId struct {
	Name string `postgres:"name"`
}

type pgCapitalId struct {
	Id int64 `json:"id"`
}

type pgIdUnsupported struct {
	Key chan int `postgres:"id"`
}

func TestResolveIdType(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		declaration *InterfaceDeclaration
		expected    string
		expectError bool
	}{
		{name: "nil declaration", declaration: nil},
		{name: "empty wrapper", declaration: &InterfaceDeclaration{}},
		{
			name:        "tag-named id resolves via postgres type",
			declaration: pgInterfaceDeclaration(t, reflect.TypeFor[pgIdTagged]()),
			expected:    "bigint",
		},
		{
			name:        "tag type override wins",
			declaration: pgInterfaceDeclaration(t, reflect.TypeFor[pgIdTypeOverride]()),
			expected:    "uuid",
		},
		{
			name:        "skipped field is ignored",
			declaration: pgInterfaceDeclaration(t, reflect.TypeFor[pgIdSkipped]()),
		},
		{
			name:        "no id property",
			declaration: pgInterfaceDeclaration(t, reflect.TypeFor[pgNoId]()),
		},
		{
			// The comparison is against the identifier as written (Go field
			// name or tag name), so an untagged `Id` field does not match.
			name:        "untagged Id field does not match",
			declaration: pgInterfaceDeclaration(t, reflect.TypeFor[pgCapitalId]()),
		},
		{
			name:        "unsupported id field type",
			declaration: pgInterfaceDeclaration(t, reflect.TypeFor[pgIdUnsupported]()),
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveIdType(testCase.declaration)
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
				t.Errorf("resolveIdType() = %q, want %q", got, testCase.expected)
			}
		})
	}
}
