package postgres

import (
	"fmt"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/type_export/postgres/types"
	typeExportTypesContext "github.com/altshiftab/utils_go/pkg/type_export/types/context"
)

func Convert(values ...any) (string, error) {
	postgresContext := types.Context{Context: typeExportTypesContext.New()}
	if err := postgresContext.Add(values...); err != nil {
		return "", fmt.Errorf("add: %w", err)
	}

	output, err := postgresContext.Render()
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("render: %w", err), postgresContext)
	}

	return output, nil
}
