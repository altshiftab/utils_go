package pdf

import (
	"bytes"

	"github.com/altshiftab/utils_go/pkg/types/file_validator"
)

func IsSigned(documentData []byte) bool {
	return bytes.Contains(documentData, []byte("/Type /Sig")) && bytes.Contains(documentData, []byte("/ByteRange"))
}

// IsEncrypted reports whether the PDF appears to use the standard encryption
// dictionary (e.g. password-protected). Encrypted PDFs reference an /Encrypt
// entry in the trailer dictionary.
func IsEncrypted(documentData []byte) bool {
	return bytes.Contains(documentData, []byte("/Encrypt"))
}

func NewPdfFileValidator() *file_validator.Validator {
	return &file_validator.Validator{
		ExpectedFileExtension: ".pdf",
		ExpectedContentType:   "application/pdf",
	}
}
