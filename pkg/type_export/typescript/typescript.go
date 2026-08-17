package typescript

import (
	"fmt"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	typeExportTypesContext "github.com/altshiftab/utils_go/pkg/type_export/types/context"
	"github.com/altshiftab/utils_go/pkg/type_export/typescript/types"
)

func Convert(values ...any) (string, error) {
	tsContext := types.Context{Context: typeExportTypesContext.New()}
	if err := tsContext.Add(values...); err != nil {
		return "", fmt.Errorf("add: %w", err)
	}

	output, err := tsContext.Render()
	if err != nil {
		return "", motmedelErrors.New(fmt.Errorf("render: %w", err), tsContext)
	}

	return output, nil
}
