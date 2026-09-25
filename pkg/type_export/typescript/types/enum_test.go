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
