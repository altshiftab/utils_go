package authorization

import (
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/abnf"
	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

var (
	ErrInvalidQuotedParameterValue        = errors.New("invalid quoted parameter value")
	ErrDuplicateParameter                 = errors.New("duplicate parameter")
	ErrMutuallyExclusiveToken68Parameters = errors.New("mutually exclusive token68 or parameters")
)

var Grammar *abnf.Grammar

func Parse(data []byte) (*altshiftHttpTypes.Authorization, error) {
	paths, err := abnfUtils.GetParsedDataPaths(Grammar, data, "Authorization")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError, data)
	}

	var authorization altshiftHttpTypes.Authorization

	interestingPaths := abnfUtils.SearchPath(
		paths[0],
		[]string{"auth-scheme", "token68", "auth-param"}, 2, false,
	)
	for _, interestingPath := range interestingPaths {
		value := string(abnfUtils.ExtractPathValue(data, interestingPath))

		switch interestingPath.MatchRule {
		case "auth-scheme":
			authorization.Scheme = value
		case "token68":
			// NOTE: Sanity check; should not be possible, is a violation of the grammar.
			if authorization.Params != nil {
				return nil, fmt.Errorf(
					"%w: %w",
					altshiftErrors.ErrSyntaxError, ErrMutuallyExclusiveToken68Parameters,
				)
			}
			authorization.Token68 = value
		case "auth-param":
			// NOTE: Sanity check; should not be possible, is a violation of the grammar.
			if authorization.Token68 != "" {
				return nil, fmt.Errorf(
					"%w: %w",
					altshiftErrors.ErrSyntaxError, ErrMutuallyExclusiveToken68Parameters,
				)
			}

			key, parameterValue, _ := strings.Cut(value, "=")
			key = strings.ToLower(strings.TrimSpace(key))
			parameterValue = strings.TrimSpace(parameterValue)

			quotedStringPath := abnfUtils.SearchPathSingleName(
				interestingPath,
				"quoted-string",
				1,
				false,
			)
			if quotedStringPath != nil {
				var err error
				quotedString := string(abnfUtils.ExtractPathValue(data, quotedStringPath))
				parameterValue, err = strconv.Unquote(quotedString)
				if err != nil {
					return nil, altshiftErrors.NewWithTrace(
						fmt.Errorf(
							"%w: %w: strvconv unquote: %w",
							altshiftErrors.ErrSemanticError,
							ErrInvalidQuotedParameterValue,
							err,
						),
						quotedString,
					)
				}
			}

			if authorization.Params == nil {
				authorization.Params = make(map[string]string)
			}

			if _, ok := authorization.Params[key]; ok {
				return nil, altshiftErrors.New(
					fmt.Errorf("%w: %w: %s", altshiftErrors.ErrSemanticError, ErrDuplicateParameter, key),
					key,
				)
			}

			authorization.Params[key] = parameterValue
		}
	}

	return &authorization, nil
}

//go:embed grammar.abnf
var grammar []byte

func init() {
	var err error
	Grammar, err = abnf.ParseABNF(grammar)
	if err != nil {
		panic(fmt.Sprintf("An error occurred when parsing the grammar: %v", err))
	}
}
