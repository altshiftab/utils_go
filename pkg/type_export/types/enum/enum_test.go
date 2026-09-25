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
