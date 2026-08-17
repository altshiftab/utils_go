package session_claims_validator

import (
	"errors"
	"fmt"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/missing_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/interfaces/comparer"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/numeric_date"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator/registered_claims_validator"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator/setting"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type ExpectedClaims struct {
	AuthenticationMethodsComparer comparer.Comparer[string]
	AuthenticatedAtComparer       comparer.Comparer[numeric_date.Date]
	AuthorizedPartyComparer       comparer.Comparer[string]
	RolesComparer                 comparer.Comparer[string]

	OtherComparers map[string]comparer.Comparer[any]
}

type Validator struct {
	RegisteredClaimsValidator *registered_claims_validator.Validator
	Settings                  map[string]setting.Setting
	Expected                  *ExpectedClaims
}

func (validator *Validator) Validate(parsedClaims map[string]any) error {
	if parsedClaims == nil {
		return fmt.Errorf("%w: %w", motmedelErrors.ErrValidationError, nil_error.New("parsed claims"))
	}

	expected := validator.Expected
	if expected == nil {
		expected = &ExpectedClaims{}
	}

	var errs []error

	if registeredClaimsValidator := validator.RegisteredClaimsValidator; registeredClaimsValidator != nil {
		if err := registeredClaimsValidator.Validate(parsedClaims); err != nil {
			errs = append(errs, err)
		}
	}

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
		case "amr":
			authenticationMethods, err := utils.ConvertSlice[string](value)
			if err != nil {
				errs = append(
					errs,
					motmedelErrors.New(fmt.Errorf("convert (%s): %w", key, err), value),
				)
				continue
			}

			if authenticationMethodsComparer := expected.AuthenticationMethodsComparer; !utils.IsNil(authenticationMethodsComparer) {
				var matched bool
				for _, authenticationMethod := range authenticationMethods {
					ok, err := authenticationMethodsComparer.Compare(authenticationMethod)
					if err != nil {
						return motmedelErrors.New(fmt.Errorf("compare (%s): %w", key, err), authenticationMethod)
					}
					if ok {
						matched = true
						break
					}
				}
				if !matched {
					errs = append(errs, mismatch_error.New(key))
				}
			}
		case "auth_time":
			authenticatedAt, err := utils.Convert[numeric_date.Date](value)
			if err != nil {
				errs = append(
					errs,
					motmedelErrors.New(fmt.Errorf("convert (%s): %w", key, err), value),
				)
				continue
			}

			if authenticatedAtComparer := expected.AuthenticatedAtComparer; !utils.IsNil(authenticatedAtComparer) {
				ok, err := authenticatedAtComparer.Compare(authenticatedAt)
				if err != nil {
					return motmedelErrors.New(fmt.Errorf("compare (%s): %w", key, err), authenticatedAt)
				}
				if !ok {
					errs = append(errs, mismatch_error.New(key))
				}
			}
		case "azp":
			authorizedParty, err := utils.Convert[string](value)
			if err != nil {
				errs = append(
					errs,
					motmedelErrors.New(fmt.Errorf("convert (%s): %w", key, err), value),
				)
				continue
			}

			if authorizedPartyComparer := expected.AuthorizedPartyComparer; !utils.IsNil(authorizedPartyComparer) {
				ok, err := authorizedPartyComparer.Compare(authorizedParty)
				if err != nil {
					return motmedelErrors.New(fmt.Errorf("compare (%s): %w", key, err), authorizedParty)
				}
				if !ok {
					errs = append(errs, mismatch_error.New(key))
				}
			}
		case "roles":
			roles, err := utils.ConvertSlice[string](value)
			if err != nil {
				errs = append(
					errs,
					motmedelErrors.New(fmt.Errorf("convert (%s): %w", key, err), value),
				)
				continue
			}

			if rolesComparer := expected.RolesComparer; !utils.IsNil(rolesComparer) {
				var matched bool
				for _, role := range roles {
					ok, err := rolesComparer.Compare(role)
					if err != nil {
						return motmedelErrors.New(fmt.Errorf("compare (%s): %w", key, err), role)
					}
					if ok {
						matched = true
						break
					}
				}
				if !matched {
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
				return motmedelErrors.New(fmt.Errorf("compare (%s): %w", key, err), value)
			}
			if !ok {
				errs = append(errs, mismatch_error.New(key))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", motmedelErrors.ErrValidationError, errors.Join(errs...))
	}

	return nil
}
