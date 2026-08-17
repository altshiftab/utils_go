package tests

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/json/schema/draft202012"
	_ "github.com/altshiftab/utils_go/pkg/json/schema/format"
	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// suiteGroup is one entry of a suite test file: a schema and the
// test cases to validate against it.
type suiteGroup struct {
	Description string         `json:"description"`
	Schema      jsontext.Value `json:"schema"`
	Tests       []*suiteCase   `json:"tests"`
}

// suiteCase is a single validation case within a suiteGroup.
type suiteCase struct {
	Description string         `json:"description"`
	Data        jsontext.Value `json:"data"`
	Valid       bool           `json:"valid"`
}

// requiredSkips maps "file.json/<group description>" (or
// "file.json/<group description>/<case description>") to the reason
// that suite entry is skipped when running the required tests.
var requiredSkips = map[string]string{
	//nolint:dupword // "with with" is the suite's own group description.
	"vocabulary.json/schema that uses custom metaschema with with no validation vocabulary": "unsupported: custom meta-schemas selected via $schema/$vocabulary are not recognized; only registered vocabularies (draft 2020-12) can be parsed",
	"vocabulary.json/ignore unrecognized optional vocabulary":                               "unsupported: custom meta-schemas selected via $schema/$vocabulary are not recognized; only registered vocabularies (draft 2020-12) can be parsed",
}

// formatSkips is like requiredSkips but for the optional format tests.
var formatSkips = map[string]string{}

// TestMain installs the loader that serves the suite's remote
// schemas from the vendored remotes directory.
func TestMain(m *testing.M) {
	schema.SetLoader(loadRemote)
	os.Exit(m.Run())
}

// loadRemote serves the vendored remote schemas that the suite
// references as http://localhost:1234/<path>. Meta-schema URIs
// (https://json-schema.org/...) are handled internally by the
// library and never reach a working loader, so any other host is
// an error.
func loadRemote(_ string, uri *url.URL) (*schema.Schema, error) {
	if uri.Host != "localhost:1234" {
		return nil, fmt.Errorf("%w: unknown remote schema host %q", errors.ErrUnsupported, uri.Host)
	}

	relativePath, err := filepath.Localize(strings.TrimPrefix(uri.Path, "/"))
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("filepath localize: %w", err), uri.Path)
	}

	fullPath := filepath.Join("remotes", relativePath)
	//nolint:gosec // Test-only loader; the path is localized above and serves vendored suite files.
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("os read file: %w", err), fullPath)
	}

	var v any
	if err := jsonv2.Unmarshal(data, &v); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("jsonv2 unmarshal: %w", err), fullPath)
	}

	s, err := schema.SchemaFromJSON(draft202012.SchemaID, nil, v)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("schema from json: %w", err), fullPath)
	}

	return s, nil
}

// runSuiteFile runs all groups and cases of one suite test file.
// validate reports whether the instance is valid against the schema;
// it returns a non-nil error for processing errors (not validation
// failures), which fail the test case.
func runSuiteFile(
	t *testing.T,
	path string,
	skips map[string]string,
	validate func(s *schema.Schema, instance any) (bool, error),
) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var groups []*suiteGroup
	if err := jsonv2.Unmarshal(data, &groups); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}

	base := filepath.Base(path)
	for _, group := range groups {
		t.Run(base+"/"+group.Description, func(t *testing.T) {
			t.Parallel()
			if reason, ok := skips[base+"/"+group.Description]; ok {
				t.Skipf("%s", reason)
			}

			var s schema.Schema
			if err := jsonv2.Unmarshal(group.Schema, &s); err != nil {
				t.Fatalf("parse schema: %v", err)
			}

			for _, testCase := range group.Tests {
				t.Run(testCase.Description, func(t *testing.T) {
					t.Parallel()
					key := base + "/" + group.Description + "/" + testCase.Description
					if reason, ok := skips[key]; ok {
						t.Skipf("%s", reason)
					}

					var instance any
					if err := jsonv2.Unmarshal(testCase.Data, &instance); err != nil {
						t.Fatalf("decode instance: %v", err)
					}

					got, err := validate(&s, instance)
					if err != nil {
						t.Fatalf("processing error: %v", err)
					}
					if got != testCase.Valid {
						t.Errorf("got valid=%v, want valid=%v", got, testCase.Valid)
					}
				})
			}
		})
	}
}

// validateRequired validates an instance without format validation,
// as the suite's required tests demand.
func validateRequired(s *schema.Schema, instance any) (bool, error) {
	err := s.ValidateWithOpts(instance, nil)
	if err == nil {
		return true, nil
	}
	if schema.IsValidationError(err) {
		return false, nil
	}
	return false, err
}

// validateFormat validates an instance with format validation
// enabled, for the optional format tests.
func validateFormat(s *schema.Schema, instance any) (bool, error) {
	err := s.Validate(instance)
	if err == nil {
		return true, nil
	}
	if _, ok := errors.AsType[*schema.ValidateError](err); ok {
		return false, nil
	}
	return false, err
}

// TestDraft202012 runs the required draft2020-12 suite tests.
func TestDraft202012(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob(filepath.Join("tests", "draft2020-12", "*.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no suite test files found; run go generate ./internal/tests")
	}

	for _, path := range paths {
		runSuiteFile(t, path, requiredSkips, validateRequired)
	}
}

// TestDraft202012OptionalFormat runs the optional format suite tests
// for the formats this library implements, with format validation on.
func TestDraft202012OptionalFormat(t *testing.T) {
	t.Parallel()
	fileNames := []string{
		"date.json",
		"date-time.json",
		"duration.json",
		"email.json",
		"hostname.json",
		"idn-email.json",
		"idn-hostname.json",
		"ipv4.json",
		"ipv6.json",
		"iri.json",
		"iri-reference.json",
		"json-pointer.json",
		"regex.json",
		"relative-json-pointer.json",
		"time.json",
		"uri.json",
		"uri-reference.json",
		"uuid.json",
	}

	for _, fileName := range fileNames {
		path := filepath.Join("tests", "draft2020-12", "optional", "format", fileName)
		runSuiteFile(t, path, formatSkips, validateFormat)
	}
}
