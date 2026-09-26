package connection_name

import (
	"errors"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func TestParse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		input   string
		want    *ConnectionName
		wantErr bool
	}{
		{name: "plain", input: "proj:europe-north1:db", want: &ConnectionName{Project: "proj", Region: "europe-north1", Instance: "db"}},
		{name: "domain-scoped project", input: "example.com:proj:europe-north1:db", want: &ConnectionName{Project: "example.com:proj", Region: "europe-north1", Instance: "db"}},
		{name: "empty", input: "", wantErr: true},
		{name: "two parts", input: "proj:db", wantErr: true},
		{name: "five parts", input: "a:b:c:d:e", wantErr: true},
		{name: "empty region", input: "proj::db", wantErr: true},
		{name: "empty instance", input: "proj:region:", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := Parse(testCase.input)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) = %+v, want error", testCase.input, got)
				}
				if testCase.input != "" && !errors.Is(err, altshiftErrors.ErrParseError) {
					t.Errorf("Parse(%q) error %v is not a parse error", testCase.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q): %v", testCase.input, err)
			}
			if *got != *testCase.want {
				t.Errorf("Parse(%q) = %+v, want %+v", testCase.input, got, testCase.want)
			}
			if got.String() != testCase.input {
				t.Errorf("String() = %q, want %q", got.String(), testCase.input)
			}
		})
	}
}
