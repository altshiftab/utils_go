package jwk

import (
	"fmt"
	"strings"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	altshiftStrings "github.com/altshiftab/utils_go/pkg/strings"
	"github.com/altshiftab/utils_go/pkg/utils"
)

func Validate(keyMap map[string]any) error {
	if keyMap == nil {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, nil_error.New("map"))
	}

	kty, err := utils.MapGetConvert[string](keyMap, "kty")
	if err != nil {
		wrappedErr := fmt.Errorf("map get convert (kty): %w", err)
		if altshiftErrors.IsAny(err, altshiftErrors.ErrConversionNotOk, altshiftErrors.ErrNotInMap) {
			wrappedErr = fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, wrappedErr)
		}
		return wrappedErr
	}

	alg, err := utils.MapGetConvert[string](keyMap, "alg")
	if err != nil {
		wrappedErr := fmt.Errorf("map get convert (alg): %w", err)
		if altshiftErrors.IsAny(err, altshiftErrors.ErrConversionNotOk, altshiftErrors.ErrNotInMap) {
			wrappedErr = fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, wrappedErr)
		}
		return wrappedErr
	}

	var expectedKty string
	if altshiftStrings.HasAnyPrefix(alg, "RS", "PS") {
		expectedKty = "RSA"
	} else if strings.HasPrefix(alg, "ES") {
		expectedKty = "EC"
	}

	if expectedKty != "" {
		if kty != expectedKty {
			return altshiftErrors.New(
				fmt.Errorf("%w: %w", altshiftErrors.ErrVerificationError, mismatch_error.New("kty", kty, expectedKty)),
				alg, kty,
			)
		}

		if expectedKty == "EC" {
			if _, err := utils.MapGetConvert[string](keyMap, "crv"); err != nil {
				return altshiftErrors.New(fmt.Errorf("%w: %w (crv)", altshiftErrors.ErrValidationError, err))
			}
		}
	}

	return nil
}
