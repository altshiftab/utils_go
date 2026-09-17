package openapi

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	"github.com/altshiftab/utils_go/pkg/http/openapi/openapi_config"
)

func runEndpoints() []*endpointPkg.Endpoint {
	return []*endpointPkg.Endpoint{
		{
			Path:       "/api/order",
			Method:     http.MethodPost,
			BodyLoader: jsonBodyLoader(4096),
			Hint: &endpointPkg.Hint{
				Documented:        true,
				InputType:         reflect.TypeFor[testOrder](),
				OutputType:        reflect.TypeFor[testOrder](),
				OutputContentType: contentTypeJson,
			},
		},
	}
}

func TestRunWritesTheDocument(t *testing.T) {
	t.Parallel()

	outPath := filepath.Join(t.TempDir(), "openapi.json")

	if code := Run([]string{"--out", outPath}, runEndpoints(), testOptions()...); code != exitSuccess {
		t.Fatalf("writing exited with %d", code)
	}

	written, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	document, err := Generate(runEndpoints(), testOptions()...)
	if err != nil {
		t.Fatal(err)
	}

	expected, err := Marshal(document)
	if err != nil {
		t.Fatal(err)
	}

	if string(written) != string(expected) {
		t.Error("the written document is not what the endpoints produce")
	}
}

func TestRunCheck(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		// prepare writes the file the check runs against, and says whether to write one at all.
		prepare      func(t *testing.T, path string)
		expectedCode int
	}{
		{
			name: "a current document passes",
			prepare: func(t *testing.T, path string) {
				t.Helper()

				if code := Run([]string{"--out", path}, runEndpoints(), testOptions()...); code != exitSuccess {
					t.Fatalf("writing exited with %d", code)
				}
			},
			expectedCode: exitSuccess,
		},
		{
			name: "a drifted document fails",
			prepare: func(t *testing.T, path string) {
				t.Helper()

				if err := os.WriteFile(path, []byte("{\"openapi\": \"3.2.0\"}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			expectedCode: exitFailure,
		},
		{
			name:         "a missing document fails",
			prepare:      func(t *testing.T, _ string) { t.Helper() },
			expectedCode: exitFailure,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			outPath := filepath.Join(t.TempDir(), "openapi.json")
			testCase.prepare(t, outPath)

			code := Run([]string{"--out", outPath, "--check"}, runEndpoints(), testOptions()...)
			if code != testCase.expectedCode {
				t.Errorf("%s: exited with %d, expected %d", testCase.name, code, testCase.expectedCode)
			}
		})
	}
}

func TestRunArguments(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		arguments    []string
		endpoints    []*endpointPkg.Endpoint
		expectedCode int
	}{
		{
			name:         "an unknown option is a usage error",
			arguments:    []string{"--nonsense"},
			endpoints:    runEndpoints(),
			expectedCode: exitBadUsage,
		},
		{
			name:         "a version outside the choices is a usage error",
			arguments:    []string{"--openapi-version", "3.0.3"},
			endpoints:    runEndpoints(),
			expectedCode: exitBadUsage,
		},
		{
			name:         "checking without a file is a failure",
			arguments:    []string{"--check"},
			endpoints:    runEndpoints(),
			expectedCode: exitFailure,
		},
		{
			// The query method has no field before 3.2, and is refused rather than written as
			// something else.
			name:      "a query endpoint under 3.1 is a failure",
			arguments: []string{"--openapi-version", openapi_config.Version31, "--out", "-"},
			endpoints: []*endpointPkg.Endpoint{
				{
					Path:       "/api/orders",
					Method:     MethodQuery,
					BodyLoader: jsonBodyLoader(4096),
					Hint:       &endpointPkg.Hint{Documented: true, InputType: reflect.TypeFor[testOrdersQuery]()},
				},
			},
			expectedCode: exitFailure,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Every case here fails before anything is written, so standard output is left alone.
			code := Run(testCase.arguments, testCase.endpoints, testOptions()...)

			if code != testCase.expectedCode {
				t.Errorf("%s: exited with %d, expected %d", testCase.name, code, testCase.expectedCode)
			}
		})
	}
}

// TestRunWritesToStandardOutput takes over os.Stdout, which is process-wide, so it does not run in
// parallel with anything else in the package.
//
//nolint:paralleltest // Swapping os.Stdout is process-wide; running alongside anything else races on it.
func TestRunWritesToStandardOutput(t *testing.T) {
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	original := os.Stdout
	os.Stdout = writeEnd

	code := Run([]string{"--out", "-"}, runEndpoints(), testOptions()...)

	os.Stdout = original
	if err := writeEnd.Close(); err != nil {
		t.Fatal(err)
	}

	if code != exitSuccess {
		t.Fatalf("writing exited with %d", code)
	}

	buffer := make([]byte, 1<<16)
	n, err := readEnd.Read(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if err := readEnd.Close(); err != nil {
		t.Fatal(err)
	}

	if n == 0 {
		t.Fatal("nothing was written to standard output")
	}
	if buffer[0] != '{' {
		t.Errorf("what was written does not look like a document: %q", buffer[:min(n, 40)])
	}
}
