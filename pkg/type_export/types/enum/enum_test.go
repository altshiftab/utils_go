package enum

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	typeExportErrors "github.com/altshiftab/utils_go/pkg/type_export/errors"
)

type valueReceiver string

func (valueReceiver) EnumValues() []string { return []string{"a", "b"} }

type pointerReceiver string

func (*pointerReceiver) EnumValues() []string { return []string{"c"} }

type plain string

type number int

func (number) EnumValues() []string { return []string{"1"} }

type empty string

func (empty) EnumValues() []string { return nil }

func TestValues(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		reflectType reflect.Type
		expected    []string
		expectedErr error
	}{
		{name: "value receiver", reflectType: reflect.TypeFor[valueReceiver](), expected: []string{"a", "b"}},
		{name: "pointer receiver", reflectType: reflect.TypeFor[pointerReceiver](), expected: []string{"c"}},
		{name: "pointer to enum", reflectType: reflect.TypeFor[*valueReceiver](), expected: []string{"a", "b"}},
		{name: "plain named string", reflectType: reflect.TypeFor[plain]()},
		{name: "string", reflectType: reflect.TypeFor[string]()},
		{name: "nil", reflectType: nil},
		{name: "non-string kind", reflectType: reflect.TypeFor[number](), expectedErr: typeExportErrors.ErrEnumNotString},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			values, err := Values(testCase.reflectType)
			if !errors.Is(err, testCase.expectedErr) {
				t.Fatalf("err = %v, expected %v", err, testCase.expectedErr)
			}
			if !slices.Equal(values, testCase.expected) {
				t.Errorf("values = %v, expected %v", values, testCase.expected)
			}
		})
	}
}

func TestValuesEmpty(t *testing.T) {
	t.Parallel()

	_, err := Values(reflect.TypeFor[empty]())
	if _, ok := errors.AsType[*empty_error.Error](err); !ok {
		t.Fatalf("err = %v, expected an empty error", err)
	}
}

type named string

func (named) EnumValues() []string         { return []string{"a", "b"} }
func (named) EnumNames() map[string]string { return map[string]string{"a": "A", "b": "B"} }

type namedPointerReceiver string

func (*namedPointerReceiver) EnumValues() []string         { return []string{"c"} }
func (*namedPointerReceiver) EnumNames() map[string]string { return map[string]string{"c": "C"} }

type namedNumber int

func (namedNumber) EnumValues() []string         { return []string{"1"} }
func (namedNumber) EnumNames() map[string]string { return map[string]string{"1": "One"} }

type unnamedValue string

func (unnamedValue) EnumValues() []string         { return []string{"a", "b"} }
func (unnamedValue) EnumNames() map[string]string { return map[string]string{"a": "A"} }

type namedNonValue string

func (namedNonValue) EnumValues() []string         { return []string{"a"} }
func (namedNonValue) EnumNames() map[string]string { return map[string]string{"a": "A", "z": "Z"} }

type duplicateName string

func (duplicateName) EnumValues() []string         { return []string{"a", "b"} }
func (duplicateName) EnumNames() map[string]string { return map[string]string{"a": "A", "b": "A"} }

type invalidName string

func (invalidName) EnumValues() []string         { return []string{"a-b"} }
func (invalidName) EnumNames() map[string]string { return map[string]string{"a-b": "a-b"} }

type snakeCase string

func (snakeCase) EnumValues() []string { return []string{"admin", "dd_admin_approver", "tier_2"} }

type notSnakeCase string

func (notSnakeCase) EnumValues() []string { return []string{"admin", "image/png"} }

type derivedAlike string

func (derivedAlike) EnumValues() []string { return []string{"x_1", "x1"} }

type namedEmpty string

func (namedEmpty) EnumValues() []string         { return []string{"a"} }
func (namedEmpty) EnumNames() map[string]string { return nil }

func TestMembers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		reflectType reflect.Type
		expected    []*Member
		expectedErr error
	}{
		{name: "value receiver", reflectType: reflect.TypeFor[named](), expected: []*Member{{Name: "A", Value: "a"}, {Name: "B", Value: "b"}}},
		{name: "pointer receiver", reflectType: reflect.TypeFor[namedPointerReceiver](), expected: []*Member{{Name: "C", Value: "c"}}},
		{name: "pointer to named", reflectType: reflect.TypeFor[*named](), expected: []*Member{{Name: "A", Value: "a"}, {Name: "B", Value: "b"}}},
		{name: "derived", reflectType: reflect.TypeFor[valueReceiver](), expected: []*Member{{Name: "A", Value: "a"}, {Name: "B", Value: "b"}}},
		{name: "derived from snake_case words", reflectType: reflect.TypeFor[snakeCase](), expected: []*Member{{Name: "Admin", Value: "admin"}, {Name: "DdAdminApprover", Value: "dd_admin_approver"}, {Name: "Tier2", Value: "tier_2"}}},
		{name: "not snake_case", reflectType: reflect.TypeFor[notSnakeCase]()},
		{name: "derived alike", reflectType: reflect.TypeFor[derivedAlike]()},
		{name: "plain named string", reflectType: reflect.TypeFor[plain]()},
		{name: "nil", reflectType: nil},
		{name: "non-string kind", reflectType: reflect.TypeFor[namedNumber](), expectedErr: typeExportErrors.ErrEnumNotString},
		{name: "unnamed value", reflectType: reflect.TypeFor[unnamedValue](), expectedErr: typeExportErrors.ErrEnumUnnamedValue},
		{name: "no names at all", reflectType: reflect.TypeFor[namedEmpty](), expectedErr: typeExportErrors.ErrEnumUnnamedValue},
		{name: "named non-value", reflectType: reflect.TypeFor[namedNonValue](), expectedErr: typeExportErrors.ErrEnumNamedNonValue},
		{name: "duplicate name", reflectType: reflect.TypeFor[duplicateName](), expectedErr: typeExportErrors.ErrEnumDuplicateName},
		{name: "invalid name", reflectType: reflect.TypeFor[invalidName](), expectedErr: typeExportErrors.ErrEnumInvalidName},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			members, err := Members(testCase.reflectType)
			if !errors.Is(err, testCase.expectedErr) {
				t.Fatalf("err = %v, expected %v", err, testCase.expectedErr)
			}
			if !slices.EqualFunc(members, testCase.expected, func(a, b *Member) bool { return *a == *b }) {
				t.Errorf("members = %v, expected %v", members, testCase.expected)
			}
		})
	}
}
