package cloud_storage

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/crc32"
	"io"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// crc32cTable is the Castagnoli polynomial, which is the CRC32C Cloud Storage
// keeps for every object; the IEEE one hash/crc32 defaults to is a different
// checksum.
var crc32cTable = crc32.MakeTable(crc32.Castagnoli)

// NewCrc32c returns a hash computing the CRC32C Cloud Storage keeps for an
// object.
func NewCrc32c() hash.Hash32 {
	return crc32.New(crc32cTable)
}

// EncodeCrc32c renders a CRC32C as Cloud Storage writes it in an object's
// metadata: the four bytes big-endian, in standard base64.
func EncodeCrc32c(checksum uint32) string {
	return base64.StdEncoding.EncodeToString(binary.BigEndian.AppendUint32(nil, checksum))
}

// DecodeCrc32c reads a CRC32C written as Cloud Storage writes it.
func DecodeCrc32c(encoded string) (uint32, error) {
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return 0, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: base64 decode string (crc32c): %w", altshiftErrors.ErrParseError, err),
			encoded,
		)
	}
	if len(decoded) != 4 {
		return 0, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: crc32c is %d bytes, not 4", altshiftErrors.ErrParseError, len(decoded)),
			encoded,
		)
	}

	return binary.BigEndian.Uint32(decoded), nil
}

// Crc32cOf reads r to its end and returns the CRC32C of what it read, encoded
// as Cloud Storage writes it.
func Crc32cOf(r io.Reader) (string, error) {
	checksum := NewCrc32c()
	if _, err := io.Copy(checksum, r); err != nil {
		return "", altshiftErrors.NewWithTrace(fmt.Errorf("io copy: %w", err))
	}

	return EncodeCrc32c(checksum.Sum32()), nil
}
