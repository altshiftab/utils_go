package content_disposition

import (
	"errors"
	"testing"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/testing/cmp"
)

func TestParseContentDisposition(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       []byte
		expected    *motmedelHttpTypes.ContentDisposition
		expectedErr error
	}{
		{
			name:        "empty content disposition",
			input:       nil,
			expected:    nil,
			expectedErr: motmedelErrors.ErrSyntaxError,
		},
		{
			name:  "valid content disposition with custom disposition type",
			input: []byte("CuStOm"),
			expected: &motmedelHttpTypes.ContentDisposition{
				DispositionType: "custom",
			},
		},
		{
			name:  "valid content disposition with uppercase disposition type and filename",
			input: []byte("Attachment; filename=example.html"),
			expected: &motmedelHttpTypes.ContentDisposition{
				DispositionType: "attachment",
				Filename:        "example.html",
			},
		},
		{
			name:  "valid content disposition with filename and custom parameters",
			input: []byte("attachment; filename=doc.txt; a=A;b=B"),
			expected: &motmedelHttpTypes.ContentDisposition{
				DispositionType: "attachment",
				Filename:        "doc.txt",
				ExtensionParameters: map[string]string{
					"a": "A",
					"b": "B",
				},
			},
		},
		{
			name:  "valid content disposition with lowercase disposition type and filename*",
			input: []byte("attachment; filename*= UTF-8''%e2%82%ac%20rates"),
			expected: &motmedelHttpTypes.ContentDisposition{
				DispositionType:  "attachment",
				FilenameAsterisk: "UTF-8''%e2%82%ac%20rates",
			},
		},
		{
			name:  "valid content disposition with lowercase disposition type and quoted filename, filename*",
			input: []byte(`attachment; filename="EURO rates"; filename*=utf-8''%e2%82%ac%20rates`),
			expected: &motmedelHttpTypes.ContentDisposition{
				DispositionType:  "attachment",
				Filename:         "EURO rates",
				FilenameAsterisk: "utf-8''%e2%82%ac%20rates",
			},
		},
		{
			name:        "invalid content disposition with uppercase disposition type and multiple filename",
			input:       []byte("Attachment; filename=example.html; FILENAME=bad"),
			expected:    nil,
			expectedErr: ErrDuplicateLabel,
		},
		{
			name:        "invalid content disposition with uppercase disposition type and multiple filename*",
			input:       []byte("Attachment; filename*=utf-8''bad; FILENAME*=utf-8''bad"),
			expected:    nil,
			expectedErr: ErrDuplicateLabel,
		},
		{
			name:        "invalid valid content disposition with duplicate custom parameter",
			input:       []byte("attachment; a=A;A=a"),
			expectedErr: ErrDuplicateLabel,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			contentDisposition, err := Parse(testCase.input)
			if !errors.Is(err, testCase.expectedErr) {
				t.Fatalf("expected error: %v, got: %v", testCase.expectedErr, err)
			}

			expected := testCase.expected

			if expected == nil && contentDisposition != nil {
				t.Fatalf("expected nil content disposition, got: %v", contentDisposition)
			}

			if diff := cmp.Diff(expected, contentDisposition); diff != "" {
				t.Fatalf("content disposition mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}
