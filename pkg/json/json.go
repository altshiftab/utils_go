package json

import (
	"encoding/json/v2"
	"fmt"
	"io"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func DecodeJson[T any](reader io.Reader) (T, error) {
	var obj T

	data, err := io.ReadAll(reader)
	if err != nil {
		return obj, altshiftErrors.New(fmt.Errorf("io read all: %w", err))
	}

	if err := json.Unmarshal(data, &obj); err != nil {
		return obj, altshiftErrors.New(fmt.Errorf("json unmarshal: %w", err), data)
	}

	return obj, err
}

// ObjectToMap converts an object to a map through its JSON form. The options are
// the marshalling's: jsontext.AllowInvalidUTF8(true), for one, has strings that
// are not UTF-8 mangled rather than failing the conversion, which is what an
// object rendered for a log needs, since its strings may be anyone's bytes.
func ObjectToMap(object any, options ...json.Options) (map[string]any, error) {
	data, err := json.Marshal(object, options...)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json marshal: %w", err), object)
	}

	var objectMap map[string]any
	if err = json.Unmarshal(data, &objectMap); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal: %w", err), data)
	}

	return objectMap, nil
}

func ObjectToBytes(object any) ([]byte, error) {
	objectMap, err := ObjectToMap(object)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("object to map: %w", err), object)
	}

	data, err := json.Marshal(objectMap)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("json marshal: %w", err), objectMap)
	}

	return data, nil
}
