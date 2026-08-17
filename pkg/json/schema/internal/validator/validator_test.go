package validator

import (
	"testing"

	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

func TestToInt(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		arg     schema.PartValue
		want    schema.PartInt
		wantErr bool
	}{
		{name: "int", arg: schema.PartInt(5), want: 5},
		{name: "integral float", arg: schema.PartFloat(5), want: 5},
		{name: "fractional float", arg: schema.PartFloat(5.5), wantErr: true},
		{name: "string", arg: schema.PartString("5"), wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, err := ToInt(testCase.arg)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("ToInt(%v): error %v, wantErr %t", testCase.arg, err, testCase.wantErr)
			}
			if err == nil && got != testCase.want {
				t.Errorf("ToInt(%v) = %d, want %d", testCase.arg, got, testCase.want)
			}
		})
	}
}

func TestToFloat(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		arg     schema.PartValue
		want    schema.PartFloat
		wantErr bool
	}{
		{name: "float", arg: schema.PartFloat(5.5), want: 5.5},
		{name: "int", arg: schema.PartInt(5), want: 5},
		{name: "bool", arg: schema.PartBool(true), wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, err := ToFloat(testCase.arg)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("ToFloat(%v): error %v, wantErr %t", testCase.arg, err, testCase.wantErr)
			}
			if err == nil && got != testCase.want {
				t.Errorf("ToFloat(%v) = %v, want %v", testCase.arg, got, testCase.want)
			}
		})
	}
}

func TestInstanceFloat(t *testing.T) {
	t.Parallel()
	type definedFloat float64

	testCases := []struct {
		name     string
		instance any
		want     float64
		wantOK   bool
	}{
		{name: "int", instance: 5, want: 5, wantOK: true},
		{name: "uint", instance: uint16(5), want: 5, wantOK: true},
		{name: "float", instance: 5.5, want: 5.5, wantOK: true},
		{name: "defined float type", instance: definedFloat(2.5), want: 2.5, wantOK: true},
		{name: "numeric string is not a number", instance: "5"},
		{name: "bool", instance: true},
		{name: "nil", instance: nil},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, ok := instanceFloat(testCase.instance)
			if ok != testCase.wantOK {
				t.Fatalf("instanceFloat(%v) ok = %t, want %t", testCase.instance, ok, testCase.wantOK)
			}
			if ok && got != testCase.want {
				t.Errorf("instanceFloat(%v) = %v, want %v", testCase.instance, got, testCase.want)
			}
		})
	}
}

func TestInstanceString(t *testing.T) {
	t.Parallel()
	type definedString string

	testCases := []struct {
		name     string
		instance any
		want     string
		wantOK   bool
	}{
		{name: "string", instance: "x", want: "x", wantOK: true},
		{name: "defined string type", instance: definedString("y"), want: "y", wantOK: true},
		{name: "int", instance: 5},
		{name: "nil", instance: nil},
		{name: "byte slice", instance: []byte("x")},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, ok := instanceString(testCase.instance)
			if ok != testCase.wantOK {
				t.Fatalf("instanceString(%v) ok = %t, want %t", testCase.instance, ok, testCase.wantOK)
			}
			if ok && got != testCase.want {
				t.Errorf("instanceString(%v) = %q, want %q", testCase.instance, got, testCase.want)
			}
		})
	}
}

func TestInstanceField(t *testing.T) {
	t.Parallel()
	type testStruct struct {
		Tagged   string `json:"tagged"`
		Untagged int
		hidden   string //nolint:unused // verifies unexported fields are not found
	}

	testCases := []struct {
		name         string
		fieldName    string
		instance     any
		wantValue    any
		wantJSONName string
		wantOK       bool
	}{
		{name: "map hit", fieldName: "a", instance: map[string]any{"a": 1}, wantValue: 1, wantJSONName: "a", wantOK: true},
		{name: "map miss", fieldName: "b", instance: map[string]any{"a": 1}},
		{name: "struct tagged field", fieldName: "tagged", instance: testStruct{Tagged: "v"}, wantValue: "v", wantJSONName: "tagged", wantOK: true},
		{name: "struct untagged field", fieldName: "Untagged", instance: testStruct{Untagged: 3}, wantValue: 3, wantJSONName: "Untagged", wantOK: true},
		{name: "struct case-folded field", fieldName: "untagged", instance: testStruct{Untagged: 3}, wantValue: 3, wantJSONName: "Untagged", wantOK: true},
		{name: "struct unexported field", fieldName: "hidden", instance: testStruct{hidden: "x"}},
		{name: "pointer to struct", fieldName: "tagged", instance: &testStruct{Tagged: "v"}, wantValue: "v", wantJSONName: "tagged", wantOK: true},
		{name: "nil pointer", fieldName: "tagged", instance: (*testStruct)(nil)},
		{name: "nil instance", fieldName: "tagged", instance: nil},
		{name: "non-object", fieldName: "tagged", instance: 5},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			value, jsonName, ok := instanceField(testCase.fieldName, testCase.instance)
			if ok != testCase.wantOK {
				t.Fatalf("instanceField(%q, %v) ok = %t, want %t", testCase.fieldName, testCase.instance, ok, testCase.wantOK)
			}
			if !ok {
				return
			}
			if value != testCase.wantValue {
				t.Errorf("value = %v, want %v", value, testCase.wantValue)
			}
			if jsonName != testCase.wantJSONName {
				t.Errorf("jsonName = %q, want %q", jsonName, testCase.wantJSONName)
			}
		})
	}
}

func TestInstanceFieldNames(t *testing.T) {
	t.Parallel()
	type testStruct struct {
		A string `json:"a"`
		B int
	}

	testCases := []struct {
		name      string
		instance  any
		wantNames []string
		wantOK    bool
	}{
		{name: "map", instance: map[string]any{"x": 1, "y": 2}, wantNames: []string{"x", "y"}, wantOK: true},
		{name: "struct", instance: testStruct{}, wantNames: []string{"a", "B"}, wantOK: true},
		{name: "nil pointer", instance: (*testStruct)(nil)},
		{name: "nil", instance: nil},
		{name: "slice", instance: []int{1}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fields, ok := instanceFieldNames(testCase.instance)
			if ok != testCase.wantOK {
				t.Fatalf("instanceFieldNames(%v) ok = %t, want %t", testCase.instance, ok, testCase.wantOK)
			}
			if !ok {
				return
			}
			if len(fields.byExactName) != len(testCase.wantNames) {
				t.Fatalf("got %d names, want %d", len(fields.byExactName), len(testCase.wantNames))
			}
			for _, name := range testCase.wantNames {
				if _, found := fields.byExactName[name]; !found {
					t.Errorf("name %q not found", name)
				}
			}
		})
	}
}

func TestFoldName(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{name: "ascii case fold", a: "FooBar", b: "foobar", want: true},
		{name: "different names", a: "foo", b: "bar", want: false},
		{name: "unicode fold", a: "ÅÄÖ", b: "åäö", want: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := foldName(testCase.a) == foldName(testCase.b); got != testCase.want {
				t.Errorf("foldName(%q) == foldName(%q) is %t, want %t", testCase.a, testCase.b, got, testCase.want)
			}
		})
	}
}
