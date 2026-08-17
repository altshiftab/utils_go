package context

import (
	"reflect"
	"testing"

	"github.com/altshiftab/utils_go/pkg/type_export/types/type_declaration"
)

type ctxLeaf struct {
	Value string `json:"value"`
}

type ctxOuter struct {
	Leaf ctxLeaf `json:"leaf"`
}

type CtxEmbeddedBase struct {
	Base string `json:"base"`
}

type ctxWithEmbedded struct {
	CtxEmbeddedBase
	Own int `json:"own"`
}

func TestAddRegistersStructWithProperties(t *testing.T) {
	t.Parallel()

	c := New()
	rt := reflect.TypeFor[ctxLeaf]()
	if err := c.Add(rt); err != nil {
		t.Fatalf("Add: %v", err)
	}

	decl, ok := c.TypeDeclarations[rt]
	if !ok {
		t.Fatalf("expected type to be registered")
	}
	iface, ok := decl.(*type_declaration.InterfaceDeclaration)
	if !ok {
		t.Fatalf("expected InterfaceDeclaration, got %T", decl)
	}
	if len(iface.Properties) != 1 {
		t.Errorf("expected 1 property, got %d", len(iface.Properties))
	}
	if iface.Properties[0].Identifier != "Value" {
		t.Errorf("Identifier = %q, want %q", iface.Properties[0].Identifier, "Value")
	}
}

func TestAddNestedStructDiscoversBoth(t *testing.T) {
	t.Parallel()

	c := New()
	if err := c.Add(reflect.TypeFor[ctxOuter]()); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if _, ok := c.TypeDeclarations[reflect.TypeFor[ctxOuter]()]; !ok {
		t.Error("expected ctxOuter to be registered")
	}
	if _, ok := c.TypeDeclarations[reflect.TypeFor[ctxLeaf]()]; !ok {
		t.Error("expected ctxLeaf (nested) to be registered")
	}

	// Order matters: the leaf is finalized first so that the outer can
	// reference it without forward declarations.
	if len(c.TypeDeclarationsInOrder) < 2 {
		t.Fatalf("expected 2 declarations in order, got %d", len(c.TypeDeclarationsInOrder))
	}
	leafIdent := ""
	outerIdent := ""
	for _, decl := range c.TypeDeclarationsInOrder {
		if iface, ok := decl.(*type_declaration.InterfaceDeclaration); ok {
			switch iface.Identifier {
			case "CtxLeaf":
				if leafIdent == "" {
					leafIdent = iface.Identifier
				}
			case "CtxOuter":
				outerIdent = iface.Identifier
				if leafIdent == "" {
					t.Error("CtxOuter appears before CtxLeaf — nested types should be finalized first")
				}
			}
		}
	}
	if leafIdent == "" || outerIdent == "" {
		t.Errorf("expected both CtxLeaf and CtxOuter in InOrder, got leaf=%q outer=%q", leafIdent, outerIdent)
	}
}

func TestAddEmbeddedStructFlattensProperties(t *testing.T) {
	t.Parallel()

	c := New()
	rt := reflect.TypeFor[ctxWithEmbedded]()
	if err := c.Add(rt); err != nil {
		t.Fatalf("Add: %v", err)
	}

	decl := c.TypeDeclarations[rt].(*type_declaration.InterfaceDeclaration)

	identifiers := map[string]bool{}
	for _, p := range decl.Properties {
		identifiers[p.Identifier] = true
	}
	if !identifiers["Base"] {
		t.Errorf("expected `Base` field flattened from embedded struct, got: %v", identifiers)
	}
	if !identifiers["Own"] {
		t.Errorf("expected `Own` field, got: %v", identifiers)
	}
}

func TestAddNonStructIsIgnored(t *testing.T) {
	t.Parallel()

	c := New()
	if err := c.Add(reflect.TypeFor[int]()); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if len(c.TypeDeclarations) != 0 {
		t.Errorf("expected no declarations for non-struct, got %d", len(c.TypeDeclarations))
	}
}

func TestAddIsIdempotent(t *testing.T) {
	t.Parallel()

	c := New()
	rt := reflect.TypeFor[ctxLeaf]()
	if err := c.Add(rt); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if err := c.Add(rt); err != nil {
		t.Fatalf("second Add: %v", err)
	}

	if got := len(c.TypeDeclarationsInOrder); got != 1 {
		t.Errorf("expected adding the same type twice to keep one declaration, got %d", got)
	}
}

func TestTitle(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty", input: "", expected: ""},
		{name: "lowercase identifier", input: "myType", expected: "MyType"},
		{name: "already capitalized", input: "MyType", expected: "MyType"},
		{name: "underscore identifier", input: "my_type", expected: "My_type"},
		{name: "digit inside", input: "ip4Addr", expected: "Ip4Addr"},
		{name: "single rune", input: "x", expected: "X"},
		{name: "non-ascii first rune", input: "élan", expected: "Élan"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := title(testCase.input); got != testCase.expected {
				t.Errorf("title(%q) = %q, expected %q", testCase.input, got, testCase.expected)
			}
		})
	}
}

// localDupA and localDupB return distinct function-local types that share the
// simple name "dup", forcing an identifier collision in the context.
func localDupA() reflect.Type {
	type dup struct {
		A string `json:"a"`
	}
	return reflect.TypeFor[dup]()
}

func localDupB() reflect.Type {
	type dup struct {
		B int `json:"b"`
	}
	return reflect.TypeFor[dup]()
}

func TestAddAnonymousStructsGetAnonymousIdentifiers(t *testing.T) {
	t.Parallel()

	c := New()

	first := reflect.TypeFor[struct{ A string }]()
	second := reflect.TypeFor[struct{ B int }]()
	if err := c.Add(first, second); err != nil {
		t.Fatalf("Add: %v", err)
	}

	testCases := []struct {
		name               string
		reflectType        reflect.Type
		expectedIdentifier string
	}{
		{name: "first anonymous struct", reflectType: first, expectedIdentifier: "Anonymous1"},
		{name: "second anonymous struct", reflectType: second, expectedIdentifier: "Anonymous2"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decl, ok := c.TypeDeclarations[testCase.reflectType]
			if !ok {
				t.Fatal("expected type to be registered")
			}
			if got := decl.QualifiedName(); got != testCase.expectedIdentifier {
				t.Errorf("QualifiedName() = %q, want %q", got, testCase.expectedIdentifier)
			}
		})
	}
}

func TestAddDuplicateTypeNamesGetUniqueIdentifiers(t *testing.T) {
	t.Parallel()

	c := New()

	dupA := localDupA()
	dupB := localDupB()
	if err := c.Add(dupA, dupB); err != nil {
		t.Fatalf("Add: %v", err)
	}

	testCases := []struct {
		name               string
		reflectType        reflect.Type
		expectedIdentifier string
	}{
		{name: "first keeps base name", reflectType: dupA, expectedIdentifier: "Dup"},
		{name: "second gets numeric suffix", reflectType: dupB, expectedIdentifier: "Dup2"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			decl, ok := c.TypeDeclarations[testCase.reflectType]
			if !ok {
				t.Fatal("expected type to be registered")
			}
			if got := decl.QualifiedName(); got != testCase.expectedIdentifier {
				t.Errorf("QualifiedName() = %q, want %q", got, testCase.expectedIdentifier)
			}
		})
	}
}
