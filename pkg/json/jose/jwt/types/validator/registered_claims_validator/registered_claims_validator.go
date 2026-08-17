package registered_claims_validator

import (
	"errors"
	"fmt"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/missing_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/interfaces/comparer"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/claim_strings"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/numeric_date"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator/setting"
	"github.com/altshiftab/utils_go/pkg/utils"
)

// TODO: Rework

type ExpectedClaims struct {
	IssuerComparer   comparer.Comparer[string]
	SubjectComparer  comparer.Comparer[string]
	AudienceComparer comparer.Comparer[string]
	IdComparer       comparer.Comparer[string]

	OtherComparers map[string]comparer.Comparer[any]
}

type Validator struct {
	Settings map[string]setting.Setting
	Expected *ExpectedClaims
}

func (validator *Validator) Validate(parsedClaims map[string]any) error {
	if parsedClaims == nil {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, nil_error.New("parsed claims"))
	}

	expected := validator.Expected
	if expected == nil {
		expected = &ExpectedClaims{}
	}

	var errs []error

	for key, value := range validator.Settings {
		if _, ok := parsedClaims[key]; value == setting.Required && !ok {
			errs = append(errs, missing_error.New(key))
		}
	}

	for key, value := range parsedClaims {
		if claimSetting := validator.Settings[key]; claimSetting == setting.Skip {
			continue
		}

		switch key {
		case "exp":
			expiresAt, err := utils.Convert[numeric_date.Date](value)
			if err != nil {
				errs = append(
					errs,
					altshiftErrors.New(fmt.Errorf("convert (%s): %w", key, err), value),
				)

				continue
			}

			if err := jwt.ValidateExpiresAt(expiresAt.Time, time.Now()); err != nil {
				errs = append(
					errs,
					altshiftErrors.New(fmt.Errorf("validate expires at: %w", err), expiresAt.Time),
				)
			}
		case "nbf":
			notBefore, err := utils.Convert[numeric_date.Date](value)
			if err != nil {
				errs = append(
					errs,
					altshiftErrors.New(fmt.Errorf("convert (%s): %w", key, err), value),
				)

				continue
			}

			if err := jwt.ValidateNotBefore(notBefore.Time, time.Now()); err != nil {
				errs = append(
					errs,
					altshiftErrors.New(fmt.Errorf("validate not before: %w", err), notBefore.Time),
				)
			}
		case "iat":
			issuedAt, err := utils.Convert[numeric_date.Date](value)
			if err != nil {
				errs = append(
					errs,
					altshiftErrors.New(fmt.Errorf("convert (%s): %w", key, err), value),
				)

				continue
			}

			if err := jwt.ValidateIssuedAt(issuedAt.Time, time.Now()); err != nil {
				errs = append(
					errs,
					altshiftErrors.New(fmt.Errorf("validate issued at: %w", err), issuedAt.Time),
				)
			}
		case "aud":
			audiences, err := utils.Convert[claim_strings.ClaimStrings](value)
			if err != nil {
				errs = append(
					errs,
					altshiftErrors.New(fmt.Errorf("convert (%s): %w", key, err), value),
				)

				continue
			}

			if audienceComparer := expected.AudienceComparer; !utils.IsNil(audienceComparer) {
				var audienceMatched bool

				for _, audience := range audiences {
					ok, err := audienceComparer.Compare(audience)
					if err != nil {
						return altshiftErrors.New(fmt.Errorf("compare (%s): %w", key, err), audience)
					}
					if ok {
						audienceMatched = true
						break
					}
				}

				if !audienceMatched {
					errs = append(errs, mismatch_error.New(key))
				}
			}
		case "iss":
			issuer, err := utils.Convert[string](value)
			if err != nil {
				errs = append(
					errs,
					altshiftErrors.New(fmt.Errorf("convert (%s): %w", key, err), value),
				)

				continue
			}

			if issuerComparer := expected.IssuerComparer; !utils.IsNil(issuerComparer) {
				ok, err := issuerComparer.Compare(issuer)
				if err != nil {
					return altshiftErrors.New(fmt.Errorf("compare (%s): %w", key, err), issuer)
				}
				if !ok {
					errs = append(errs, mismatch_error.New(key))
				}
			}
		case "sub":
			subject, err := utils.Convert[string](value)
			if err != nil {
				errs = append(
					errs,
					altshiftErrors.New(fmt.Errorf("convert (%s): %w", key, err), value),
				)

				continue
			}

			if subjectComparer := expected.SubjectComparer; !utils.IsNil(subjectComparer) {
				ok, err := subjectComparer.Compare(subject)
				if err != nil {
					return altshiftErrors.New(fmt.Errorf("compare (%s): %w", key, err), subject)
				}
				if !ok {
					errs = append(errs, mismatch_error.New(key))
				}
			}
		case "jti":
			id, err := utils.Convert[string](value)
			if err != nil {
				errs = append(
					errs,
					altshiftErrors.New(fmt.Errorf("convert (%s): %w", key, err), value),
				)

				continue
			}

			if idComparer := expected.IdComparer; !utils.IsNil(idComparer) {
				ok, err := idComparer.Compare(id)
				if err != nil {
					return altshiftErrors.New(fmt.Errorf("compare (%s): %w", key, err), id)
				}
				if !ok {
					errs = append(errs, mismatch_error.New(key))
				}
			}
		default:
			otherComparer, ok := expected.OtherComparers[key]
			if !ok || utils.IsNil(otherComparer) {
				continue
			}

			ok, err := otherComparer.Compare(value)
			if err != nil {
				return altshiftErrors.New(fmt.Errorf("compare (%s): %w", key, err), value)
			}
			if !ok {
				errs = append(errs, mismatch_error.New(key))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, errors.Join(errs...))
	}

	return nil
}
