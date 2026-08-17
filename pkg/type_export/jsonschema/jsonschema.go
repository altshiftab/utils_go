package jsonschema

import (
	"fmt"
	"reflect"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/type_export/jsonschema/types"
	typeExportTypesContext "github.com/altshiftab/utils_go/pkg/type_export/types/context"
)

func Convert(root reflect.Type) (string, error) {
	jsonschemaContext := types.Context{Context: typeExportTypesContext.New()}
	if err := jsonschemaContext.Add(root); err != nil {
		return "", fmt.Errorf("add: %w", err)
	}

	output, err := jsonschemaContext.RenderRoot(root)
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("render root: %w", err), jsonschemaContext)
	}

	return output, nil
}
