// Package force_send encodes a request with fields written even where their tags would leave them
// out as empty. A Google API reads a field left out of an update as one to keep as it is, so the
// one way to clear a value is to send it empty -- which a tag that omits empty values never does.
// It is ForceSendFields, as Google's own client libraries call it.
package force_send

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"reflect"
	"slices"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// Marshal encodes value, a struct or a pointer to one, as json.Marshal does, and then writes the
// fields whose JSON names are in fields whatever they hold.
func Marshal(value any, fields []string) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json marshal: %w", err))
	}
	if len(fields) == 0 {
		return encoded, nil
	}

	structValue := reflect.Indirect(reflect.ValueOf(value))
	if structValue.Kind() != reflect.Struct {
		return encoded, nil
	}

	var members map[string]jsontext.Value
	if err := json.Unmarshal(encoded, &members); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal: %w", err))
	}
	if members == nil {
		members = make(map[string]jsontext.Value)
	}

	structType := structValue.Type()
	for i := range structType.NumField() {
		name, _, _ := strings.Cut(structType.Field(i).Tag.Get("json"), ",")
		if name == "" || name == "-" || !slices.Contains(fields, name) {
			continue
		}
		if _, present := members[name]; present {
			continue
		}

		fieldValue, err := json.Marshal(structValue.Field(i).Interface())
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json marshal (%s): %w", name, err))
		}
		members[name] = fieldValue
	}

	forced, err := json.Marshal(members, json.Deterministic(true))
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json marshal (members): %w", err))
	}

	return forced, nil
}
