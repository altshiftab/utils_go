package file_validator

import (
	"errors"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func TestValidateDataContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		data        []byte
		wantErr     bool
	}{
		{
			name:        "valid html content type",
			contentType: "text/html; charset=utf-8",
			data:        []byte("<html><body>hello</body></html>"),
			wantErr:     false,
		},
		{
			name:        "mismatched content type",
			contentType: "application/pdf",
			data:        []byte("<html><body>hello</body></html>"),
			wantErr:     true,
		},
		{
			name:        "valid plain text content type",
			contentType: "text/plain; charset=utf-8",
			data:        []byte("just plain text"),
			wantErr:     false,
		},
		{
			name:        "empty data mismatched content type",
			contentType: "application/pdf",
			data:        []byte(""),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := &Validator{ExpectedContentType: tt.contentType}
			err := v.ValidateDataContentType(tt.data)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err != nil && !errors.Is(err, altshiftErrors.ErrValidationError) {
				t.Fatalf("expected validation error, got: %v", err)
			}
		})
	}
}

func TestValidateFilePathExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		extension string
		path      string
		wantErr   bool
	}{
		{
			name:      "valid pdf extension",
			extension: ".pdf",
			path:      "/tmp/document.pdf",
			wantErr:   false,
		},
		{
			name:      "mismatched extension",
			extension: ".pdf",
			path:      "/tmp/document.txt",
			wantErr:   true,
		},
		{
			name:      "no extension",
			extension: ".pdf",
			path:      "/tmp/document",
			wantErr:   true,
		},
		{
			name:      "valid json extension",
			extension: ".json",
			path:      "data.json",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := &Validator{ExpectedFileExtension: tt.extension}
			err := v.ValidateFilePathExtension(tt.path)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err != nil && !errors.Is(err, altshiftErrors.ErrValidationError) {
				t.Fatalf("expected validation error, got: %v", err)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	t.Run("both checks pass", func(t *testing.T) {
		t.Parallel()

		v := &Validator{
			ExpectedContentType:   "text/plain; charset=utf-8",
			ExpectedFileExtension: ".txt",
		}
		if err := v.Validate([]byte("plain text content"), "file.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("content type fails", func(t *testing.T) {
		t.Parallel()

		v := &Validator{
			ExpectedContentType:   "application/pdf",
			ExpectedFileExtension: ".txt",
		}
		if err := v.Validate([]byte("plain text content"), "file.txt"); err == nil {
			t.Fatalf("expected error but got nil")
		}
	})

	t.Run("file extension fails", func(t *testing.T) {
		t.Parallel()

		v := &Validator{
			ExpectedContentType:   "text/plain; charset=utf-8",
			ExpectedFileExtension: ".pdf",
		}
		if err := v.Validate([]byte("plain text content"), "file.txt"); err == nil {
			t.Fatalf("expected error but got nil")
		}
	})

	t.Run("empty content type skips content check", func(t *testing.T) {
		t.Parallel()

		v := &Validator{
			ExpectedFileExtension: ".txt",
		}
		if err := v.Validate([]byte("anything"), "file.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty extension skips extension check", func(t *testing.T) {
		t.Parallel()

		v := &Validator{
			ExpectedContentType: "text/plain; charset=utf-8",
		}
		if err := v.Validate([]byte("plain text content"), "file.pdf"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("both empty skips all checks", func(t *testing.T) {
		t.Parallel()

		v := &Validator{}
		if err := v.Validate(nil, ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
