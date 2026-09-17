package types

import (
	"encoding/json/v2"
	"errors"
	"reflect"
	"strings"
	"testing"

	typeExportErrors "github.com/altshiftab/utils_go/pkg/type_export/errors"
	typeExportContext "github.com/altshiftab/utils_go/pkg/type_export/types/context"
)

type refPrefixInner struct {
	Value string `json:"value"`
}

type refPrefixOuter struct {
	Name  string          `json:"name"`
	Inner *refPrefixInner `json:"inner"`
}

// propertyRef digs the $ref out of a property schema, whether it is written plainly or wrapped in
// the anyOf a nullable reference becomes.
func propertyRef(t *testing.T, properties map[string]any, name string) string {
	t.Helper()

	property, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("property %q is not an object: %v", name, properties[name])
	}

	if ref, ok := property["$ref"].(string); ok {
		return ref
	}

	anyOf, ok := property["anyOf"].([]any)
	if !ok {
		t.Fatalf("property %q has neither $ref nor anyOf: %v", name, property)
	}

	for _, alternative := range anyOf {
		alternativeMap, ok := alternative.(map[string]any)
		if !ok {
			continue
		}
		if ref, ok := alternativeMap["$ref"].(string); ok {
			return ref
		}
	}

	t.Fatalf("property %q has no $ref among its alternatives: %v", name, property)

	return ""
}

func TestContextRefPrefix(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		refPrefix      string
		expectedPrefix string
	}{
		{
			name:           "unset means defs",
			refPrefix:      "",
			expectedPrefix: "#/$defs/",
		},
		{
			name:           "openapi components",
			refPrefix:      "#/components/schemas/",
			expectedPrefix: "#/components/schemas/",
		},
		{
			name:           "default given explicitly",
			refPrefix:      "#/$defs/",
			expectedPrefix: "#/$defs/",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			context := &Context{Context: typeExportContext.New(), RefPrefix: testCase.refPrefix}

			reflectType := reflect.TypeFor[refPrefixOuter]()
			if err := context.Add(reflectType); err != nil {
				t.Fatal(err)
			}

			schemas, err := context.BuildSchemas()
			if err != nil {
				t.Fatal(err)
			}

			outer, ok := schemas["RefPrefixOuter"].(map[string]any)
			if !ok {
				t.Fatalf("%s: no RefPrefixOuter schema among %v", testCase.name, slicesOfKeys(schemas))
			}

			properties, ok := outer["properties"].(map[string]any)
			if !ok {
				t.Fatalf("%s: RefPrefixOuter has no properties: %v", testCase.name, outer)
			}

			ref := propertyRef(t, properties, "inner")
			if !strings.HasPrefix(ref, testCase.expectedPrefix) {
				t.Errorf("%s: ref %q does not start with %q", testCase.name, ref, testCase.expectedPrefix)
			}
			if ref != testCase.expectedPrefix+"RefPrefixInner" {
				t.Errorf("%s: unexpected ref %q", testCase.name, ref)
			}
		})
	}
}

// TestBuildSchemasHoldsEveryDeclaration checks that the schemas offered on their own are the ones
// RenderRoot would have put under $defs -- the reason the method was extracted from it.
func TestBuildSchemasHoldsEveryDeclaration(t *testing.T) {
	t.Parallel()

	context := &Context{Context: typeExportContext.New()}

	reflectType := reflect.TypeFor[refPrefixOuter]()
	if err := context.Add(reflectType); err != nil {
		t.Fatal(err)
	}

	schemas, err := context.BuildSchemas()
	if err != nil {
		t.Fatal(err)
	}

	output, err := context.RenderRoot(reflectType)
	if err != nil {
		t.Fatal(err)
	}

	var document map[string]any
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatal(err)
	}

	defs, ok := document["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("rendered document has no $defs: %v", document)
	}

	if len(defs) != len(schemas) {
		t.Fatalf("$defs holds %d schemas, BuildSchemas returned %d", len(defs), len(schemas))
	}

	for identifier := range defs {
		if _, ok := schemas[identifier]; !ok {
			t.Errorf("BuildSchemas is missing %q, which $defs holds", identifier)
		}
	}

	for _, identifier := range []string{"RefPrefixOuter", "RefPrefixInner"} {
		if _, ok := schemas[identifier]; !ok {
			t.Errorf("BuildSchemas is missing %q", identifier)
		}
	}
}

// TestRenderRootRejectsRefPrefix checks the misuse the prefix invites: a whole document whose
// references point outside it.
func TestRenderRootRejectsRefPrefix(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		refPrefix   string
		expectError bool
	}{
		{name: "unset renders", refPrefix: "", expectError: false},
		{name: "default renders", refPrefix: "#/$defs/", expectError: false},
		{name: "components refused", refPrefix: "#/components/schemas/", expectError: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			context := &Context{Context: typeExportContext.New(), RefPrefix: testCase.refPrefix}

			reflectType := reflect.TypeFor[refPrefixOuter]()
			if err := context.Add(reflectType); err != nil {
				t.Fatal(err)
			}

			_, err := context.RenderRoot(reflectType)

			if testCase.expectError {
				if !errors.Is(err, typeExportErrors.ErrRefPrefixWithRoot) {
					t.Errorf("%s: expected ErrRefPrefixWithRoot, got %v", testCase.name, err)
				}
			} else if err != nil {
				t.Errorf("%s: unexpected error: %v", testCase.name, err)
			}
		})
	}
}

func slicesOfKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}

	return keys
}
