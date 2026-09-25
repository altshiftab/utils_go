package types

import (
	"reflect"
	"strings"
	"testing"
)

type pgRole string

func (pgRole) EnumValues() []string { return []string{"admin", "o'brien"} }

type pgEnumTable struct {
	Id            string   `json:"id" postgres:"id,primarykey"`
	Role          pgRole   `json:"role"`
	Roles         []pgRole `json:"roles"`
	OptionalRole  *pgRole  `json:"optional_role" postgres:"optional_role,nullable"`
	CaseFreeRoles []pgRole `json:"case_free_roles" postgres:"case_free_roles,type:citext[]"`
}

func TestRenderEnumCheck(t *testing.T) {
	t.Parallel()

	out := renderPG(t, reflect.TypeFor[pgEnumTable]())

	testCases := []struct {
		name     string
		expected string
	}{
		{name: "scalar", expected: "Role text CHECK (Role IN ('admin', 'o''brien')) NOT NULL"},
		{name: "array", expected: "Roles text[] CHECK (Roles <@ ARRAY['admin', 'o''brien']::text[]) NOT NULL"},
		{name: "nullable", expected: "optional_role text CHECK (optional_role IN ('admin', 'o''brien')),"},
		{name: "type override", expected: "case_free_roles citext[] CHECK (case_free_roles <@ ARRAY['admin', 'o''brien']::citext[]) NOT NULL"},
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

func TestCheckConstraint(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		column     string
		columnType string
		values     []string
		isArray    bool
		expected   string
	}{
		{name: "scalar", column: "kind", columnType: "text", values: []string{"a", "b"}, expected: "CHECK (kind IN ('a', 'b'))"},
		{name: "array", column: "roles", columnType: "text[]", values: []string{"a"}, isArray: true, expected: "CHECK (roles <@ ARRAY['a']::text[])"},
		{name: "quote doubled", column: "name", columnType: "text", values: []string{"it's"}, expected: "CHECK (name IN ('it''s'))"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := CheckConstraint(testCase.column, testCase.columnType, testCase.values, testCase.isArray); got != testCase.expected {
				t.Errorf("CheckConstraint() = %q, expected %q", got, testCase.expected)
			}
		})
	}
}
