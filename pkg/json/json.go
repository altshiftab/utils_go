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

func ObjectToMap(object any) (map[string]any, error) {
	data, err := json.Marshal(object)
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
