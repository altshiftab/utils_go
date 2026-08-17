package generic_type_info

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	goTypes "go/types"
	"reflect"
	"sort"
	"testing"
	"unique"

	"github.com/altshiftab/utils_go/pkg/type_export/types/shape"
)

func TestParseTypeArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{name: "no generics", input: "MyType", expected: nil},
		{name: "single arg", input: "MyType[int]", expected: []string{"int"}},
		{name: "two args", input: "MyType[int,string]", expected: []string{"int", "string"}},
		{name: "two args with spaces", input: "MyType[int, string]", expected: []string{"int", "string"}},
		{name: "nested generic arg", input: "MyType[pkg.Bar[int]]", expected: []string{"pkg.Bar[int]"}},
		{name: "full pkg path", input: "MyType[github.com/user/pkg.SomeType]", expected: []string{"github.com/user/pkg.SomeType"}},
		{name: "two full pkg paths", input: "MyType[github.com/a.X,github.com/b.Y]", expected: []string{"github.com/a.X", "github.com/b.Y"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := parseTypeArgs(tt.input)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d args, got %d: %v", len(tt.expected), len(result), result)
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("arg %d: expected %q, got %q", i, tt.expected[i], result[i])
				}
			}
		})
	}
}

type Inner struct {
	Value string
}

type GenericDirect[T any] struct {
	Data  T
	Other string
}

type GenericPointer[T any] struct {
	Data  *T
	Other string
}

type GenericSlice[T any] struct {
	Items []T
	Other string
}

type GenericMapValue[T any] struct {
	Mapping map[string]T
	Other   string
}

type GenericMapKeyValue[K comparable, V any] struct {
	Mapping map[K]V
}

type GenericTwoParams[T any, U any] struct {
	First  T
	Second []U
}

type GenericArray[T any] struct {
	Items [3]T
}

type GenericMapKeyOnly[K comparable] struct {
	Mapping map[K]string
}

type GenericSamePairTwice[T any] struct {
	A T
	B T
}

type GenericMapTSameParam[T comparable] struct {
	Mapping map[T]T
}

type GenericMixed[T any, U any, V comparable] struct {
	Direct  T
	Pointer *T
	Slice   []U
	Array   [2]U
	Mapping map[V]U
	Plain   string
}

// Distinct named types used as test type arguments so the reflection-based
// discoverer (which can only compare full type names) does not confuse them
// with concrete field types like `string` or `int`.
type uniqueT struct{}
type uniqueU struct{}
type uniqueV string

func TestDiscoverUsingReflection(t *testing.T) {
	t.Parallel()

	t.Run("direct", func(t *testing.T) {
		t.Parallel()

		rt := reflect.TypeFor[GenericDirect[int]]()
		info, err := discoverUsingReflection(rt)
		if err != nil {
			t.Fatal(err)
		}
		if info == nil {
			t.Fatal("expected non-nil info")
		}
		if len(info.TypeParameterNames) != 1 {
			t.Fatalf("expected 1 type param, got %d", len(info.TypeParameterNames))
		}
		shapes, ok := info.FieldNameToShapes["Data"]
		if !ok || len(shapes) != 1 {
			t.Fatalf("expected one shape for Data, got %v", shapes)
		}
		if shapes[0].Kind != shape.KindDirect {
			t.Errorf("expected KindDirect, got %v", shapes[0].Kind)
		}
		if _, ok := info.FieldNameToShapes["Other"]; ok {
			t.Error("Other should not be in FieldNameToShapes")
		}
	})

	t.Run("pointer", func(t *testing.T) {
		t.Parallel()

		rt := reflect.TypeFor[GenericPointer[Inner]]()
		info, err := discoverUsingReflection(rt)
		if err != nil {
			t.Fatal(err)
		}
		if info == nil {
			t.Fatal("expected non-nil info")
		}
		shapes, ok := info.FieldNameToShapes["Data"]
		if !ok || len(shapes) != 1 {
			t.Fatalf("expected one shape for Data, got %v", shapes)
		}
		if shapes[0].Kind != shape.KindPointer {
			t.Errorf("expected KindPointer, got %v", shapes[0].Kind)
		}
	})

	t.Run("slice", func(t *testing.T) {
		t.Parallel()

		rt := reflect.TypeFor[GenericSlice[string]]()
		info, err := discoverUsingReflection(rt)
		if err != nil {
			t.Fatal(err)
		}
		if info == nil {
			t.Fatal("expected non-nil info")
		}
		shapes, ok := info.FieldNameToShapes["Items"]
		if !ok || len(shapes) != 1 {
			t.Fatalf("expected one shape for Items, got %v", shapes)
		}
		if shapes[0].Kind != shape.KindSlice {
			t.Errorf("expected KindSlice, got %v", shapes[0].Kind)
		}
	})

	t.Run("map value", func(t *testing.T) {
		t.Parallel()

		rt := reflect.TypeFor[GenericMapValue[int]]()
		info, err := discoverUsingReflection(rt)
		if err != nil {
			t.Fatal(err)
		}
		if info == nil {
			t.Fatal("expected non-nil info")
		}
		shapes, ok := info.FieldNameToShapes["Mapping"]
		if !ok || len(shapes) != 1 {
			t.Fatalf("expected one shape for Mapping, got %v", shapes)
		}
		if shapes[0].Kind != shape.KindMapValue {
			t.Errorf("expected KindMapValue, got %v", shapes[0].Kind)
		}
	})

	t.Run("map key and value both generic", func(t *testing.T) {
		t.Parallel()

		rt := reflect.TypeFor[GenericMapKeyValue[string, int]]()
		info, err := discoverUsingReflection(rt)
		if err != nil {
			t.Fatal(err)
		}
		if info == nil {
			t.Fatal("expected non-nil info")
		}
		shapes := info.FieldNameToShapes["Mapping"]
		if len(shapes) != 2 {
			t.Fatalf("expected two shapes for Mapping (key + value), got %v", shapes)
		}
		var sawKey, sawValue bool
		for _, s := range shapes {
			//exhaustive:ignore
			switch s.Kind {
			case shape.KindMapKey:
				sawKey = true
			case shape.KindMapValue:
				sawValue = true
			}
		}
		if !sawKey || !sawValue {
			t.Errorf("expected both KindMapKey and KindMapValue, got %v", shapes)
		}
	})

	t.Run("two params", func(t *testing.T) {
		t.Parallel()

		rt := reflect.TypeFor[GenericTwoParams[int, Inner]]()
		info, err := discoverUsingReflection(rt)
		if err != nil {
			t.Fatal(err)
		}
		if info == nil {
			t.Fatal("expected non-nil info")
		}
		if len(info.TypeParameterNames) != 2 {
			t.Fatalf("expected 2 type params, got %d", len(info.TypeParameterNames))
		}

		s1, ok := info.FieldNameToShapes["First"]
		if !ok || len(s1) != 1 {
			t.Fatalf("expected one shape for First, got %v", s1)
		}
		if s1[0].Kind != shape.KindDirect {
			t.Errorf("expected KindDirect for First, got %v", s1[0].Kind)
		}

		s2, ok := info.FieldNameToShapes["Second"]
		if !ok || len(s2) != 1 {
			t.Fatalf("expected one shape for Second, got %v", s2)
		}
		if s2[0].Kind != shape.KindSlice {
			t.Errorf("expected KindSlice for Second, got %v", s2[0].Kind)
		}

		// Params should map to different fields
		if info.TypeParameterNameToFieldName[s1[0].Param] != "First" {
			t.Errorf("expected param %s to map to First", s1[0].Param)
		}
		if info.TypeParameterNameToFieldName[s2[0].Param] != "Second" {
			t.Errorf("expected param %s to map to Second", s2[0].Param)
		}
	})

	t.Run("non-generic returns nil", func(t *testing.T) {
		t.Parallel()

		rt := reflect.TypeFor[Inner]()
		info, err := discoverUsingReflection(rt)
		if err != nil {
			t.Fatal(err)
		}
		if info != nil {
			t.Error("expected nil info for non-generic type")
		}
	})

	t.Run("array", func(t *testing.T) {
		t.Parallel()

		rt := reflect.TypeFor[GenericArray[int]]()
		info, err := discoverUsingReflection(rt)
		if err != nil {
			t.Fatal(err)
		}
		shapes := info.FieldNameToShapes["Items"]
		if len(shapes) != 1 {
			t.Fatalf("expected one shape for Items, got %v", shapes)
		}
		if shapes[0].Kind != shape.KindArray {
			t.Errorf("expected KindArray, got %v", shapes[0].Kind)
		}
	})

	t.Run("map key only generic", func(t *testing.T) {
		t.Parallel()

		// Use a unique named type as K so it doesn't accidentally collide
		// with the concrete `string` value type in the reflection comparison.
		rt := reflect.TypeFor[GenericMapKeyOnly[uniqueV]]()
		info, err := discoverUsingReflection(rt)
		if err != nil {
			t.Fatal(err)
		}
		shapes := info.FieldNameToShapes["Mapping"]
		if len(shapes) != 1 {
			t.Fatalf("expected one shape for Mapping, got %v", shapes)
		}
		if shapes[0].Kind != shape.KindMapKey {
			t.Errorf("expected KindMapKey, got %v", shapes[0].Kind)
		}
	})

	t.Run("same param appears in two fields", func(t *testing.T) {
		t.Parallel()

		rt := reflect.TypeFor[GenericSamePairTwice[int]]()
		info, err := discoverUsingReflection(rt)
		if err != nil {
			t.Fatal(err)
		}

		for _, name := range []string{"A", "B"} {
			shapes := info.FieldNameToShapes[name]
			if len(shapes) != 1 {
				t.Errorf("field %s: expected one shape, got %v", name, shapes)
				continue
			}
			if shapes[0].Kind != shape.KindDirect {
				t.Errorf("field %s: expected KindDirect, got %v", name, shapes[0].Kind)
			}
		}

		// First-seen field wins for the param→field index.
		if got := info.TypeParameterNameToFieldName["T0"]; got != "A" {
			t.Errorf("expected param T0 → A (first-seen), got %q", got)
		}
	})

	t.Run("same param as both map key and value", func(t *testing.T) {
		t.Parallel()

		rt := reflect.TypeFor[GenericMapTSameParam[string]]()
		info, err := discoverUsingReflection(rt)
		if err != nil {
			t.Fatal(err)
		}

		shapes := info.FieldNameToShapes["Mapping"]
		if len(shapes) != 2 {
			t.Fatalf("expected two shapes for Mapping (key + value of same param), got %v", shapes)
		}
		var sawKey, sawValue bool
		for _, s := range shapes {
			if s.Param != "T0" {
				t.Errorf("expected Param T0, got %q", s.Param)
			}
			//exhaustive:ignore
			switch s.Kind {
			case shape.KindMapKey:
				sawKey = true
			case shape.KindMapValue:
				sawValue = true
			}
		}
		if !sawKey || !sawValue {
			t.Errorf("expected both KindMapKey and KindMapValue for same param, got %v", shapes)
		}
	})

	t.Run("mixed shapes across multiple fields", func(t *testing.T) {
		t.Parallel()

		// Use unique named types as type args so the reflection discoverer
		// can't confuse them with the concrete `string` Plain field.
		rt := reflect.TypeFor[GenericMixed[uniqueT, uniqueU, uniqueV]]()
		info, err := discoverUsingReflection(rt)
		if err != nil {
			t.Fatal(err)
		}
		if len(info.TypeParameterNames) != 3 {
			t.Fatalf("expected 3 type params, got %d", len(info.TypeParameterNames))
		}

		expectations := map[string]shape.Kind{
			"Direct":  shape.KindDirect,
			"Pointer": shape.KindPointer,
			"Slice":   shape.KindSlice,
			"Array":   shape.KindArray,
		}
		for fieldName, wantKind := range expectations {
			shapes := info.FieldNameToShapes[fieldName]
			if len(shapes) != 1 {
				t.Errorf("field %s: expected one shape, got %v", fieldName, shapes)
				continue
			}
			if shapes[0].Kind != wantKind {
				t.Errorf("field %s: expected %v, got %v", fieldName, wantKind, shapes[0].Kind)
			}
		}

		mappingShapes := info.FieldNameToShapes["Mapping"]
		if len(mappingShapes) != 2 {
			t.Fatalf("expected two shapes for Mapping, got %v", mappingShapes)
		}

		if _, ok := info.FieldNameToShapes["Plain"]; ok {
			t.Error("Plain (no type param) should not be in FieldNameToShapes")
		}

		// Every type param should have a representative field.
		for _, paramName := range info.TypeParameterNames {
			if info.TypeParameterNameToFieldName[paramName] == "" {
				t.Errorf("param %s missing from TypeParameterNameToFieldName", paramName)
			}
		}
	})
}

func TestDetectShapeAst(t *testing.T) {
	t.Parallel()

	type want struct {
		param string
		kind  shape.Kind
	}

	tests := []struct {
		name     string
		expr     string
		paramSet map[string]struct{}
		want     []want
	}{
		{
			name:     "direct param",
			expr:     "T",
			paramSet: map[string]struct{}{"T": {}},
			want:     []want{{"T", shape.KindDirect}},
		},
		{
			name:     "non-param identifier",
			expr:     "string",
			paramSet: map[string]struct{}{"T": {}},
			want:     nil,
		},
		{
			name:     "pointer to param",
			expr:     "*T",
			paramSet: map[string]struct{}{"T": {}},
			want:     []want{{"T", shape.KindPointer}},
		},
		{
			name:     "slice of param",
			expr:     "[]T",
			paramSet: map[string]struct{}{"T": {}},
			want:     []want{{"T", shape.KindSlice}},
		},
		{
			name:     "fixed array of param",
			expr:     "[3]T",
			paramSet: map[string]struct{}{"T": {}},
			want:     []want{{"T", shape.KindArray}},
		},
		{
			name:     "map with param value only",
			expr:     "map[string]V",
			paramSet: map[string]struct{}{"V": {}},
			want:     []want{{"V", shape.KindMapValue}},
		},
		{
			name:     "map with param key only",
			expr:     "map[K]string",
			paramSet: map[string]struct{}{"K": {}},
			want:     []want{{"K", shape.KindMapKey}},
		},
		{
			name:     "map with both params generic",
			expr:     "map[K]V",
			paramSet: map[string]struct{}{"K": {}, "V": {}},
			want:     []want{{"V", shape.KindMapValue}, {"K", shape.KindMapKey}},
		},
		{
			name:     "map with same param as key and value",
			expr:     "map[T]T",
			paramSet: map[string]struct{}{"T": {}},
			want:     []want{{"T", shape.KindMapValue}, {"T", shape.KindMapKey}},
		},
		{
			name:     "map with no generic types",
			expr:     "map[string]int",
			paramSet: map[string]struct{}{"T": {}},
			want:     nil,
		},
		{
			name:     "selector expression (qualified type) does not match",
			expr:     "pkg.X",
			paramSet: map[string]struct{}{"T": {}},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			expr, err := parser.ParseExpr(tt.expr)
			if err != nil {
				t.Fatalf("parse expr %q: %v", tt.expr, err)
			}

			got := detectShapeAst(expr, tt.paramSet)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d matches, want %d: got=%v want=%v", len(got), len(tt.want), got, tt.want)
			}

			gotPairs := make([]want, len(got))
			for i, m := range got {
				gotPairs[i] = want(m)
			}
			sort.Slice(gotPairs, func(i, j int) bool {
				if gotPairs[i].param != gotPairs[j].param {
					return gotPairs[i].param < gotPairs[j].param
				}
				return gotPairs[i].kind < gotPairs[j].kind
			})
			wantSorted := append([]want(nil), tt.want...)
			sort.Slice(wantSorted, func(i, j int) bool {
				if wantSorted[i].param != wantSorted[j].param {
					return wantSorted[i].param < wantSorted[j].param
				}
				return wantSorted[i].kind < wantSorted[j].kind
			})

			for i := range gotPairs {
				if gotPairs[i] != wantSorted[i] {
					t.Errorf("match %d: got %+v, want %+v", i, gotPairs[i], wantSorted[i])
				}
			}
		})
	}
}

func TestMatchTypeArg(t *testing.T) {
	t.Parallel()

	type want struct {
		argIdx int
		kind   shape.Kind
	}

	tests := []struct {
		name     string
		field    reflect.Type
		typeArgs []string
		want     []want
	}{
		{
			name:     "direct match",
			field:    reflect.TypeFor[int](),
			typeArgs: []string{"int"},
			want:     []want{{0, shape.KindDirect}},
		},
		{
			name:     "pointer to arg",
			field:    reflect.TypeFor[*int](),
			typeArgs: []string{"int"},
			want:     []want{{0, shape.KindPointer}},
		},
		{
			name:     "slice of arg",
			field:    reflect.TypeFor[[]string](),
			typeArgs: []string{"string"},
			want:     []want{{0, shape.KindSlice}},
		},
		{
			name:     "fixed array of arg",
			field:    reflect.TypeFor[[3]int](),
			typeArgs: []string{"int"},
			want:     []want{{0, shape.KindArray}},
		},
		{
			name:     "map value only",
			field:    reflect.TypeFor[map[string]int](),
			typeArgs: []string{"int"},
			want:     []want{{0, shape.KindMapValue}},
		},
		{
			name:     "map key only",
			field:    reflect.TypeFor[map[string]int](),
			typeArgs: []string{"string"},
			want:     []want{{0, shape.KindMapKey}},
		},
		{
			name:     "map both key and value match different args",
			field:    reflect.TypeFor[map[string]int](),
			typeArgs: []string{"string", "int"},
			want:     []want{{1, shape.KindMapValue}, {0, shape.KindMapKey}},
		},
		{
			name:     "map both key and value match same arg",
			field:    reflect.TypeFor[map[int]int](),
			typeArgs: []string{"int"},
			want:     []want{{0, shape.KindMapValue}, {0, shape.KindMapKey}},
		},
		{
			name:     "no match",
			field:    reflect.TypeFor[float64](),
			typeArgs: []string{"int", "string"},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := matchTypeArg(tt.field, tt.typeArgs)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d matches, want %d: got=%v want=%v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				gw := want{got[i].argIdx, got[i].kind}
				if gw != tt.want[i] {
					t.Errorf("match %d: got %+v, want %+v", i, gw, tt.want[i])
				}
			}
		})
	}
}

// typeCheckSource type-checks a single import-free source file and returns the
// resulting package, for exercising the go/types-based discovery path without
// depending on the importer.
func typeCheckSource(t *testing.T, src string) *goTypes.Package {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse file: %v", err)
	}

	pkg, err := (&goTypes.Config{}).Check("example", fileSet, []*ast.File{file}, nil)
	if err != nil {
		t.Fatalf("type check: %v", err)
	}

	return pkg
}

const discoverSource = `package example

type Direct[T any] struct {
	Data  T
	Other string
}

type Pointer[T any] struct{ Data *T }

type Slice[T any] struct{ Items []T }

type Array[T any] struct{ Items [3]T }

type MapValue[T any] struct{ Mapping map[string]T }

type MapKey[K comparable] struct{ Mapping map[K]string }

type MapKeyValue[K comparable, V any] struct{ Mapping map[K]V }

type MapSameParam[T comparable] struct{ Mapping map[T]T }

type Unused[T any] struct{ Plain string }

type NotGenericStruct struct{ Value string }

type GenericNotStruct[T any] int

type Alias = int

var NotType int
`

func TestDiscoverInTypesPackageShapes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		typeName       string
		expectedParams []string
		expectedShapes map[string][]shape.Shape
		expectedFields map[string]string
	}{
		{
			name:           "direct",
			typeName:       "Direct",
			expectedParams: []string{"T"},
			expectedShapes: map[string][]shape.Shape{"Data": {{Param: "T", Kind: shape.KindDirect}}},
			expectedFields: map[string]string{"T": "Data"},
		},
		{
			name:           "pointer",
			typeName:       "Pointer",
			expectedParams: []string{"T"},
			expectedShapes: map[string][]shape.Shape{"Data": {{Param: "T", Kind: shape.KindPointer}}},
			expectedFields: map[string]string{"T": "Data"},
		},
		{
			name:           "slice",
			typeName:       "Slice",
			expectedParams: []string{"T"},
			expectedShapes: map[string][]shape.Shape{"Items": {{Param: "T", Kind: shape.KindSlice}}},
			expectedFields: map[string]string{"T": "Items"},
		},
		{
			name:           "array",
			typeName:       "Array",
			expectedParams: []string{"T"},
			expectedShapes: map[string][]shape.Shape{"Items": {{Param: "T", Kind: shape.KindArray}}},
			expectedFields: map[string]string{"T": "Items"},
		},
		{
			name:           "map value",
			typeName:       "MapValue",
			expectedParams: []string{"T"},
			expectedShapes: map[string][]shape.Shape{"Mapping": {{Param: "T", Kind: shape.KindMapValue}}},
			expectedFields: map[string]string{"T": "Mapping"},
		},
		{
			name:           "map key",
			typeName:       "MapKey",
			expectedParams: []string{"K"},
			expectedShapes: map[string][]shape.Shape{"Mapping": {{Param: "K", Kind: shape.KindMapKey}}},
			expectedFields: map[string]string{"K": "Mapping"},
		},
		{
			name:           "map key and value",
			typeName:       "MapKeyValue",
			expectedParams: []string{"K", "V"},
			expectedShapes: map[string][]shape.Shape{
				"Mapping": {{Param: "V", Kind: shape.KindMapValue}, {Param: "K", Kind: shape.KindMapKey}},
			},
			expectedFields: map[string]string{"K": "Mapping", "V": "Mapping"},
		},
		{
			name:           "map same param as key and value",
			typeName:       "MapSameParam",
			expectedParams: []string{"T"},
			expectedShapes: map[string][]shape.Shape{
				"Mapping": {{Param: "T", Kind: shape.KindMapValue}, {Param: "T", Kind: shape.KindMapKey}},
			},
			expectedFields: map[string]string{"T": "Mapping"},
		},
		{
			name:           "param not used by any field",
			typeName:       "Unused",
			expectedParams: []string{"T"},
			expectedShapes: map[string][]shape.Shape{},
			expectedFields: map[string]string{},
		},
	}

	pkg := typeCheckSource(t, discoverSource)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			info, err := discoverInTypesPackage(pkg, testCase.typeName)
			if err != nil {
				t.Fatalf("discover in types package: %v", err)
			}
			if info == nil {
				t.Fatal("expected non-nil info")
			}

			if !reflect.DeepEqual(info.TypeParameterNames, testCase.expectedParams) {
				t.Errorf("TypeParameterNames = %v, want %v", info.TypeParameterNames, testCase.expectedParams)
			}
			if !reflect.DeepEqual(info.FieldNameToShapes, testCase.expectedShapes) {
				t.Errorf("FieldNameToShapes = %v, want %v", info.FieldNameToShapes, testCase.expectedShapes)
			}
			if !reflect.DeepEqual(info.TypeParameterNameToFieldName, testCase.expectedFields) {
				t.Errorf("TypeParameterNameToFieldName = %v, want %v", info.TypeParameterNameToFieldName, testCase.expectedFields)
			}
		})
	}
}

func TestDiscoverInTypesPackageErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		typeName      string
		expectedError error
	}{
		{name: "object is not a type name", typeName: "NotType", expectedError: ErrNotTypeName},
		{name: "alias is not a named type", typeName: "Alias", expectedError: ErrNotNamed},
		{name: "named type is not a struct", typeName: "GenericNotStruct", expectedError: ErrNotStruct},
		{name: "struct without type parameters", typeName: "NotGenericStruct"},
	}

	pkg := typeCheckSource(t, discoverSource)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			info, err := discoverInTypesPackage(pkg, testCase.typeName)
			if err == nil {
				t.Fatal("expected an error")
			}
			if testCase.expectedError != nil && !errors.Is(err, testCase.expectedError) {
				t.Errorf("error = %v, want %v", err, testCase.expectedError)
			}
			if info != nil {
				t.Errorf("expected nil info, got %+v", info)
			}
		})
	}

	t.Run("type not found", func(t *testing.T) {
		t.Parallel()

		info, err := discoverInTypesPackage(pkg, "NoSuchType")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info != nil {
			t.Errorf("expected nil info for unknown type, got %+v", info)
		}
	})

	t.Run("nil package", func(t *testing.T) {
		t.Parallel()

		if _, err := discoverInTypesPackage(nil, "Direct"); err == nil {
			t.Error("expected an error for nil package")
		}
	})
}

func TestDiscoverUsingTypesImporter(t *testing.T) {
	t.Parallel()

	t.Run("nonexistent package", func(t *testing.T) {
		t.Parallel()

		info, err := discoverUsingTypesImporter("example.com/does/not/exist", "X")
		if err == nil {
			t.Error("expected an error for a nonexistent package")
		}
		if info != nil {
			t.Errorf("expected nil info, got %+v", info)
		}
	})

	t.Run("stdlib generic struct", func(t *testing.T) {
		t.Parallel()

		// unique.Handle[T comparable] is a generic struct in the standard
		// library. Only its type parameter is asserted; the field layout is
		// an implementation detail of the stdlib.
		info, err := discoverUsingTypesImporter("unique", "Handle")
		if err != nil {
			t.Fatalf("discover using types importer: %v", err)
		}
		if info == nil {
			t.Fatal("expected non-nil info")
		}
		if !reflect.DeepEqual(info.TypeParameterNames, []string{"T"}) {
			t.Errorf("TypeParameterNames = %v, want [T]", info.TypeParameterNames)
		}
	})
}

func TestGetGenericTypeInfoErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		reflectType   reflect.Type
		expectedError error
	}{
		{name: "non-struct", reflectType: reflect.TypeFor[int](), expectedError: ErrNotStruct},
		{name: "non-generic struct", reflectType: reflect.TypeFor[Inner](), expectedError: ErrNotGeneric},
		{name: "anonymous struct has no type name", reflectType: reflect.TypeFor[struct{ A string }]()},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			info, err := GetGenericTypeInfo(testCase.reflectType)
			if err == nil {
				t.Fatal("expected an error")
			}
			if testCase.expectedError != nil && !errors.Is(err, testCase.expectedError) {
				t.Errorf("error = %v, want %v", err, testCase.expectedError)
			}
			if info != nil {
				t.Errorf("expected nil info, got %+v", info)
			}
		})
	}
}

func TestGetGenericTypeInfoImporterFallback(t *testing.T) {
	t.Parallel()

	// unique.Handle is not declared in this package's working directory, so
	// discovery must fall through to the go/types importer.
	info, err := GetGenericTypeInfo(reflect.TypeFor[unique.Handle[string]]())
	if err != nil {
		t.Fatalf("get generic type info: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if !reflect.DeepEqual(info.TypeParameterNames, []string{"T"}) {
		t.Errorf("TypeParameterNames = %v, want [T]", info.TypeParameterNames)
	}
}
