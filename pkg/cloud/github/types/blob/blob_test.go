package blob

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestData(t *testing.T) {
	t.Parallel()

	const contents = "match http m|^HTTP/1\\.[01]|\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(contents))

	testCases := []struct {
		name    string
		blob    *Blob
		want    string
		wantErr error
		anyErr  bool
	}{
		{
			name: "base64 contents",
			blob: &Blob{Encoding: Base64Encoding, Content: encoded},
			want: contents,
		},
		{
			// The API wraps encoded contents across lines, which the decoder
			// rejects unless they are taken out first.
			name: "wrapped contents",
			blob: &Blob{Encoding: Base64Encoding, Content: wrap(encoded)},
			want: contents,
		},
		{
			name: "no declared encoding is taken as base64",
			blob: &Blob{Content: encoded},
			want: contents,
		},
		{
			// Decoding these as base64 anyway would produce bytes that are not
			// the file, with nothing to say so.
			name:    "an encoding that is not base64",
			blob:    &Blob{Encoding: "utf-8", Content: contents},
			wantErr: ErrUnexpectedEncoding,
		},
		{
			name:   "contents that will not decode",
			blob:   &Blob{Encoding: Base64Encoding, Content: "not base64 !!!"},
			anyErr: true,
		},
		{
			name:   "no blob at all",
			blob:   nil,
			anyErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			data, err := testCase.blob.Data()
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("error = %v, want %v", err, testCase.wantErr)
				}
				return
			}
			if testCase.anyErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Data: %v", err)
			}
			if string(data) != testCase.want {
				t.Errorf("got %q, want %q", data, testCase.want)
			}
		})
	}
}

func wrap(value string) string {
	var wrapped strings.Builder
	for index := 0; index < len(value); index += 4 {
		wrapped.WriteString(value[index:min(index+4, len(value))])
		wrapped.WriteString("\n")
	}

	return wrapped.String()
}
