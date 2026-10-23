package types

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	typeExportErrors "github.com/altshiftab/utils_go/pkg/type_export/errors"
	typeExportContext "github.com/altshiftab/utils_go/pkg/type_export/types/context"
)

type tsRole string

func (tsRole) EnumValues() []string { return []string{"admin", "dd", `say "hi"`} }

type tsNamed string

type tsEnumObject struct {
	Role          tsRole    `json:"role"`
	Roles         []tsRole  `json:"roles"`
	OptionalRoles *[]tsRole `json:"optional_roles,omitzero"`
	Kind          string    `json:"kind" jsonschema:"kind,enum:a,enum:b"`
	Kinds         []string  `json:"kinds" jsonschema:"kinds,enum:a,enum:b"`
}

type tsEnumTagOnNamed struct {
	Value tsNamed `json:"value" jsonschema:"value,enum:a"`
}

func TestRenderEnum(t *testing.T) {
	t.Parallel()

	out := renderTS(t, reflect.TypeFor[tsEnumObject]())

	testCases := []struct {
		name     string
		expected string
	}{
		{name: "named union alias", expected: `export type TsRole = "admin" | "dd" | "say \"hi\"";`},
		{name: "field references the alias", expected: "role: TsRole;"},
		{name: "slice of the alias", expected: "roles: TsRole[];"},
		{name: "optional pointer to slice", expected: "optional_roles?: TsRole[];"},
		{name: "tag-level inline union", expected: `kind: "a" | "b";`},
		{name: "tag-level union array", expected: `kinds: ("a" | "b")[];`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(out, testCase.expected) {
				t.Errorf("expected %q in:\n%s", testCase.expected, out)
			}
		})
	}
}

func TestRenderEnumNominalTypesLeaveUnion(t *testing.T) {
	t.Parallel()

	ctx := &Context{Context: typeExportContext.New(), GenerateNominalTypes: true}
	if err := ctx.Add(reflect.TypeFor[tsEnumObject]()); err != nil {
		t.Fatalf("ctx.Add: %v", err)
	}
	out, err := ctx.Render()
	if err != nil {
		t.Fatalf("ctx.Render: %v", err)
	}

	if strings.Contains(out, "_TsRolebrand") {
		t.Errorf("the enum union was branded:\n%s", out)
	}
	if !strings.Contains(out, `export type TsRole = "admin" | "dd"`) {
		t.Errorf("no union alias in:\n%s", out)
	}
}

func TestRenderEnumTagOnNamedType(t *testing.T) {
	t.Parallel()

	ctx := &Context{Context: typeExportContext.New()}
	if err := ctx.Add(reflect.TypeFor[tsEnumTagOnNamed]()); err != nil {
		t.Fatalf("ctx.Add: %v", err)
	}
	if _, err := ctx.Render(); !errors.Is(err, typeExportErrors.ErrEnumTagOnNamedType) {
		t.Fatalf("err = %v, expected %v", err, typeExportErrors.ErrEnumTagOnNamedType)
	}
}

type tsSliceOnly struct {
	Roles []tsRole `json:"roles"`
}

type tsPointerSliceOnly struct {
	Roles *[]tsRole `json:"roles,omitzero"`
}

type tsMapOnly struct {
	ByName map[string]tsRole `json:"by_name"`
}

func TestRenderEnumReachedOnlyThroughContainer(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		targetType reflect.Type
		expected   []string
	}{
		{name: "slice", targetType: reflect.TypeFor[tsSliceOnly](), expected: []string{"roles: TsRole[];", "export type TsRole = "}},
		{name: "pointer to slice", targetType: reflect.TypeFor[tsPointerSliceOnly](), expected: []string{"roles?: TsRole[];", "export type TsRole = "}},
		{name: "map value", targetType: reflect.TypeFor[tsMapOnly](), expected: []string{"by_name: { [key: string]: TsRole };", "export type TsRole = "}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			out := renderTS(t, testCase.targetType)
			for _, expected := range testCase.expected {
				if !strings.Contains(out, expected) {
					t.Errorf("expected %q in:\n%s", expected, out)
				}
			}
		})
	}
}

type tsNamedRole string

func (tsNamedRole) EnumValues() []string { return []string{"admin", "dd_viewer", `say "hi"`} }
func (tsNamedRole) EnumNames() map[string]string {
	return map[string]string{"admin": "Admin", "dd_viewer": "DdViewer", `say "hi"`: "SayHi"}
}

type tsBadlyNamedRole string

func (tsBadlyNamedRole) EnumValues() []string         { return []string{"admin", "dd"} }
func (tsBadlyNamedRole) EnumNames() map[string]string { return map[string]string{"admin": "Admin"} }

type tsNamedEnumObject struct {
	Role  tsNamedRole   `json:"role"`
	Roles []tsNamedRole `json:"roles"`
}

type tsBadlyNamedEnumObject struct {
	Role tsBadlyNamedRole `json:"role"`
}

func TestRenderNamedEnum(t *testing.T) {
	t.Parallel()

	out := renderTS(t, reflect.TypeFor[tsNamedEnumObject]())

	testCases := []struct {
		name     string
		expected string
	}{
		{
			name: "constants beside the union",
			expected: "export const TsNamedRole = {\n" +
				"\tAdmin: \"admin\",\n" +
				"\tDdViewer: \"dd_viewer\",\n" +
				"\tSayHi: \"say \\\"hi\\\"\",\n" +
				"} as const;\n" +
				`export type TsNamedRole = "admin" | "dd_viewer" | "say \"hi\"";`,
		},
		{name: "field references the alias", expected: "role: TsNamedRole;"},
		{name: "slice of the alias", expected: "roles: TsNamedRole[];"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(out, testCase.expected) {
				t.Errorf("expected %q in:\n%s", testCase.expected, out)
			}
		})
	}
}

type tsDerivedRole string

func (tsDerivedRole) EnumValues() []string { return []string{"admin", "dd_admin_approver"} }

type tsDerivedEnumObject struct {
	Role tsDerivedRole `json:"role"`
}

func TestRenderDerivedEnum(t *testing.T) {
	t.Parallel()

	out := renderTS(t, reflect.TypeFor[tsDerivedEnumObject]())

	expected := "export const TsDerivedRole = {\n" +
		"\tAdmin: \"admin\",\n" +
		"\tDdAdminApprover: \"dd_admin_approver\",\n" +
		"} as const;\n" +
		`export type TsDerivedRole = "admin" | "dd_admin_approver";`
	if !strings.Contains(out, expected) {
		t.Errorf("expected %q in:\n%s", expected, out)
	}
}

// TestRenderEnumNotDerivableHasNoConstants is an enum with a value no name can be derived from,
// which is left the union alone.
func TestRenderEnumNotDerivableHasNoConstants(t *testing.T) {
	t.Parallel()

	out := renderTS(t, reflect.TypeFor[tsEnumObject]())
	if strings.Contains(out, "export const") {
		t.Errorf("constants rendered for an enum with a value that is not snake_case:\n%s", out)
	}
}

func TestRenderNamedEnumIncomplete(t *testing.T) {
	t.Parallel()

	ctx := &Context{Context: typeExportContext.New()}
	if err := ctx.Add(reflect.TypeFor[tsBadlyNamedEnumObject]()); err != nil {
		t.Fatalf("ctx.Add: %v", err)
	}
	if _, err := ctx.Render(); !errors.Is(err, typeExportErrors.ErrEnumUnnamedValue) {
		t.Fatalf("err = %v, expected %v", err, typeExportErrors.ErrEnumUnnamedValue)
	}
}
