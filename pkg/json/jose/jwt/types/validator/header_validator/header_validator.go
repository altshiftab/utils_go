package header_validator

import (
	"errors"
	"fmt"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/missing_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/interfaces/comparer"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator/setting"
	"github.com/altshiftab/utils_go/pkg/utils"
)

// TODO: Rework this. Use map?

type ExpectedFields struct {
	Alg comparer.Comparer[string]
	Typ comparer.Comparer[string]
}

type Validator struct {
	Settings map[string]setting.Setting
	Expected *ExpectedFields
}

func (validator *Validator) Validate(fields map[string]any) error {
	if fields == nil {
		return fmt.Errorf("%w: %w", motmedelErrors.ErrValidationError, nil_error.New("fields"))
	}

	expected := validator.Expected
	if expected == nil {
		expected = &ExpectedFields{}
	}

	var errs []error

	for key, value := range validator.Settings {
		if _, ok := fields[key]; value == setting.Required && !ok {
			errs = append(errs, missing_error.New(key))
		}
	}

	for key, value := range fields {
		if fieldSetting := validator.Settings[key]; fieldSetting == setting.Skip {
			continue
		}

		switch key {
		case "alg":
			alg, err := utils.Convert[string](value)
			if err != nil {
				return motmedelErrors.New(fmt.Errorf("convert (%s): %w", key, err), value)
			}

			if algComparer := expected.Alg; !utils.IsNil(algComparer) {
				ok, err := algComparer.Compare(alg)
				if err != nil {
					return motmedelErrors.New(fmt.Errorf("compare (%s): %w", key, err), alg)
				}
				if !ok {
					errs = append(errs, mismatch_error.New(key))
				}
			}
		case "typ":
			typ, err := utils.Convert[string](value)
			if err != nil {
				return motmedelErrors.New(fmt.Errorf("convert (%s): %w", key, err), value)
			}

			if typComparer := expected.Typ; !utils.IsNil(typComparer) {
				ok, err := typComparer.Compare(typ)
				if err != nil {
					return motmedelErrors.New(fmt.Errorf("compare (%s): %w", key, err), typ)
				}
				if !ok {
					errs = append(errs, mismatch_error.New(key))
				}
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", motmedelErrors.ErrValidationError, errors.Join(errs...))
	}

	return nil
}
