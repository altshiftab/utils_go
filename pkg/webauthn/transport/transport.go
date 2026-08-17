// Package transport contains the JSON wire-format counterparts of the webauthn package's
// domain types, with base64url-encoded binary fields, and the conversions from wire format to
// parsed domain values.
package transport

import (
	"encoding/base64"
	"encoding/json/v2"
	"fmt"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// Base64URL is a byte string that is JSON-encoded as unpadded base64url.
type Base64URL []byte

func (b *Base64URL) UnmarshalJSON(data []byte) error {
	var encoded string
	if err := json.Unmarshal(data, &encoded); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal: %w", err))
	}

	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: base64 url encoding decode string: %w", altshiftErrors.ErrParseError, err),
			encoded,
		)
	}

	*b = decoded
	return nil
}

func (b *Base64URL) MarshalJSON() ([]byte, error) {
	encoded := base64.RawURLEncoding.EncodeToString(*b)
	data, err := json.Marshal(encoded)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json marshal: %w", err), encoded)
	}

	return data, nil
}
