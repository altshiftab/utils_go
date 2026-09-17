package openapi

import (
	"reflect"
	"testing"

	openapiTypes "github.com/altshiftab/utils_go/pkg/http/openapi/types"
)

type queryIdInput struct {
	Id string `json:"id" query:"id,format=uuid"`
}

type queryMixedInput struct {
	Id       string   `json:"id" query:"id,format=uuid"`
	Name     string   `json:"name" query:"name"`
	Optional string   `json:"optional" query:"optional,omitzero"`
	Count    int      `json:"count" query:"count"`
	Ratio    float64  `json:"ratio" query:"ratio"`
	Enabled  bool     `json:"enabled" query:"enabled"`
	Kinds    []string `json:"kinds" query:"kinds"`
	Raw      []byte   `json:"raw" query:"raw"`
	Address  string   `json:"address" query:"address,format=email"`
	Link     string   `json:"link" query:"link,format=url"`
	Skipped  string   `json:"skipped" query:"-"`
	//nolint:unused // Unexported on purpose: the walk must leave it out, which is what it is here to show.
	hidden string
}

type queryJsonFallbackInput struct {
	Id   string `json:"id"`
	Note string `json:"note,omitzero"`
}

type queryPointerInput struct {
	Id *string `query:"id"`
}

type queryFixedArrayInput struct {
	Pair [2]int `query:"pair"`
}

// parameterByName finds one parameter, so that a case can assert about it without depending on the
// order the fields happen to be walked in.
func parameterByName(parameters []*openapiTypes.Parameter, name string) *openapiTypes.Parameter {
	for _, parameter := range parameters {
		if parameter.Name == name {
			return parameter
		}
	}

	return nil
}

func TestMakeParametersNames(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		reflectType   reflect.Type
		expectedNames []string
		expectError   bool
	}{
		{
			name:          "nil type has no parameters",
			reflectType:   nil,
			expectedNames: nil,
		},
		{
			name:          "empty interface has no parameters",
			reflectType:   reflect.TypeFor[any](),
			expectedNames: nil,
		},
		{
			name:          "single query parameter",
			reflectType:   reflect.TypeFor[queryIdInput](),
			expectedNames: []string{"id"},
		},
		{
			name:        "pointer to struct is walked",
			reflectType: reflect.TypeFor[*queryIdInput](),

			expectedNames: []string{"id"},
		},
		{
			name:        "unexported and skipped fields are left out",
			reflectType: reflect.TypeFor[queryMixedInput](),
			expectedNames: []string{
				"id", "name", "optional", "count", "ratio", "enabled", "kinds", "raw", "address", "link",
			},
		},
		{
			name:          "json tag answers where there is no query tag",
			reflectType:   reflect.TypeFor[queryJsonFallbackInput](),
			expectedNames: []string{"id", "note"},
		},
		{
			// The extractor refuses a pointer field outright, so a document describing one would
			// describe a request the server cannot accept.
			name:        "pointer field is refused",
			reflectType: reflect.TypeFor[queryPointerInput](),
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			parameters, err := MakeParameters(testCase.reflectType)

			if testCase.expectError {
				if err == nil {
					t.Fatalf("%s: expected an error", testCase.name)
				}

				return
			}
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", testCase.name, err)
			}

			if len(parameters) != len(testCase.expectedNames) {
				var actualNames []string
				for _, parameter := range parameters {
					actualNames = append(actualNames, parameter.Name)
				}
				t.Fatalf("%s: expected %v, got %v", testCase.name, testCase.expectedNames, actualNames)
			}

			for i, expectedName := range testCase.expectedNames {
				if parameters[i].Name != expectedName {
					t.Errorf(
						"%s: parameter %d is %q, expected %q",
						testCase.name,
						i,
						parameters[i].Name,
						expectedName,
					)
				}
				if parameters[i].In != parameterInQuery {
					t.Errorf("%s: parameter %q is in %q", testCase.name, expectedName, parameters[i].In)
				}
			}
		})
	}
}

func TestMakeParametersSchemas(t *testing.T) {
	t.Parallel()

	parameters, err := MakeParameters(reflect.TypeFor[queryMixedInput]())
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name             string
		parameterName    string
		expectedType     string
		expectedFormat   string
		expectedRequired bool
		expectedItemType string
	}{
		{
			name:             "format carries across",
			parameterName:    "id",
			expectedType:     "string",
			expectedFormat:   "uuid",
			expectedRequired: true,
		},
		{
			name:             "a plain string has no format",
			parameterName:    "name",
			expectedType:     "string",
			expectedRequired: true,
		},
		{
			name:             "omitzero means optional",
			parameterName:    "optional",
			expectedType:     "string",
			expectedRequired: false,
		},
		{
			name:             "an int is an integer",
			parameterName:    "count",
			expectedType:     "integer",
			expectedRequired: true,
		},
		{
			name:             "a float is a number",
			parameterName:    "ratio",
			expectedType:     "number",
			expectedRequired: true,
		},
		{
			name:             "a bool is a boolean",
			parameterName:    "enabled",
			expectedType:     "boolean",
			expectedRequired: true,
		},
		{
			name:             "a slice is an array of its element",
			parameterName:    "kinds",
			expectedType:     "array",
			expectedItemType: "string",
			expectedRequired: true,
		},
		{
			// The extractor sets a []byte from the value as it stands. The JSON exporter would call
			// it a base64 string, which is not what the server reads.
			name:             "a byte slice is one plain string",
			parameterName:    "raw",
			expectedType:     "string",
			expectedRequired: true,
		},
		{
			name:             "email format carries across",
			parameterName:    "address",
			expectedType:     "string",
			expectedFormat:   "email",
			expectedRequired: true,
		},
		{
			// The extractor's vocabulary calls it "url", JSON Schema calls it "uri".
			name:             "url format becomes uri",
			parameterName:    "link",
			expectedType:     "string",
			expectedFormat:   "uri",
			expectedRequired: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			parameter := parameterByName(parameters, testCase.parameterName)
			if parameter == nil {
				t.Fatalf("%s: no parameter named %q", testCase.name, testCase.parameterName)
			}

			if parameter.Required != testCase.expectedRequired {
				t.Errorf(
					"%s: required is %v, expected %v",
					testCase.name,
					parameter.Required,
					testCase.expectedRequired,
				)
			}

			if actualType := parameter.Schema["type"]; actualType != testCase.expectedType {
				t.Errorf("%s: type is %v, expected %q", testCase.name, actualType, testCase.expectedType)
			}

			actualFormat, _ := parameter.Schema["format"].(string)
			if actualFormat != testCase.expectedFormat {
				t.Errorf("%s: format is %q, expected %q", testCase.name, actualFormat, testCase.expectedFormat)
			}

			if testCase.expectedItemType != "" {
				items, ok := parameter.Schema["items"].(map[string]any)
				if !ok {
					t.Fatalf("%s: no items in %v", testCase.name, parameter.Schema)
				}
				if actualItemType := items["type"]; actualItemType != testCase.expectedItemType {
					t.Errorf(
						"%s: item type is %v, expected %q",
						testCase.name,
						actualItemType,
						testCase.expectedItemType,
					)
				}
			}
		})
	}
}

// TestMakeParametersHasNoExporterMinimums checks the reason the walk is separate: the JSON schema
// exporter puts a minimum length on every string and a minimum count on every array, which is true
// of a validated body and not of a query.
func TestMakeParametersHasNoExporterMinimums(t *testing.T) {
	t.Parallel()

	parameters, err := MakeParameters(reflect.TypeFor[queryMixedInput]())
	if err != nil {
		t.Fatal(err)
	}

	for _, parameter := range parameters {
		if _, ok := parameter.Schema["minLength"]; ok {
			t.Errorf("parameter %q carries a minLength", parameter.Name)
		}

		if parameter.Name == "kinds" {
			if _, ok := parameter.Schema["minItems"]; ok {
				t.Errorf("parameter %q carries a minItems", parameter.Name)
			}
		}
	}
}

// TestMakeParametersFixedArray checks that an array of a fixed size is described as taking exactly
// that many values, which is what the extractor requires of it.
func TestMakeParametersFixedArray(t *testing.T) {
	t.Parallel()

	parameters, err := MakeParameters(reflect.TypeFor[queryFixedArrayInput]())
	if err != nil {
		t.Fatal(err)
	}

	parameter := parameterByName(parameters, "pair")
	if parameter == nil {
		t.Fatal("no parameter named \"pair\"")
	}

	if parameter.Schema["minItems"] != 2 || parameter.Schema["maxItems"] != 2 {
		t.Errorf("expected exactly two items, got %v", parameter.Schema)
	}
}
