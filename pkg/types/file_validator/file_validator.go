package file_validator

import (
	"fmt"
	"net/http"
	"path/filepath"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
)

type Validator struct {
	ExpectedContentType   string
	ExpectedFileExtension string
}

func (v *Validator) ValidateDataContentType(data []byte) error {
	if contentType := http.DetectContentType(data); contentType != v.ExpectedContentType {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: %w",
				altshiftErrors.ErrValidationError,
				mismatch_error.New("content type", v.ExpectedContentType, contentType),
			),
		)
	}
	return nil
}

func (v *Validator) ValidateFilePathExtension(path string) error {
	fileExtension := filepath.Ext(path)
	if fileExtension != v.ExpectedFileExtension {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: %w",
				altshiftErrors.ErrValidationError,
				mismatch_error.New("file extension", v.ExpectedFileExtension, fileExtension),
			),
		)
	}
	return nil
}

func (v *Validator) Validate(data []byte, filePath string) error {
	if expectedContentType := v.ExpectedContentType; expectedContentType != "" {
		if err := v.ValidateDataContentType(data); err != nil {
			return err
		}
	}
	if expectedFileExtension := v.ExpectedFileExtension; expectedFileExtension != "" {
		if err := v.ValidateFilePathExtension(filePath); err != nil {
			return err
		}
	}

	return nil
}
