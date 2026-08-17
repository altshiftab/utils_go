package strict_transport_security

import (
	_ "embed"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/abnf"
	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

//go:embed grammar.abnf
var grammar []byte

var StrictTransportSecurityGrammar *abnf.Grammar

const (
	maxAgeDirectiveName = "max-age"
)

var digitRegexp = regexp.MustCompile(`^\d+$`)

// TODO: Update to use proper errors

func Parse(data []byte) (*motmedelHttpTypes.StrictTransportSecurityPolicy, error) {
	paths, err := abnfUtils.GetParsedDataPaths(StrictTransportSecurityGrammar, data, "Strict-Transport-Security")
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, motmedelErrors.NewWithTrace(motmedelErrors.ErrSyntaxError, data)
	}

	directiveNameSet := make(map[string]struct{})

	var strictTransportPolicy motmedelHttpTypes.StrictTransportSecurityPolicy

	interestingPaths := abnfUtils.SearchPath(paths[0], []string{"directive"}, 2, false)
	for _, interestingPath := range interestingPaths {
		directiveNamePath := abnfUtils.SearchPathSingleName(
			interestingPath,
			"directive-name",
			1,
			false,
		)
		if directiveNamePath == nil {
			return nil, nil
			//return nil, &motmedelErrors.Error{
			//	Message: "No directive name could be found in an interesting path.",
			//	Input:   interestingPath,
			//}
		}
		directiveName := string(abnfUtils.ExtractPathValue(data, directiveNamePath))

		var directiveStringValue string

		directiveValuePath := abnfUtils.SearchPathSingleName(
			interestingPath,
			"directive-value",
			1,
			false,
		)
		if directiveValuePath != nil {
			quotedStringPath := abnfUtils.SearchPathSingleName(
				directiveValuePath,
				"quoted-string",
				1,
				false,
			)
			if quotedStringPath != nil {
				var err error
				quotedString := string(abnfUtils.ExtractPathValue(data, quotedStringPath))
				directiveStringValue, err = strconv.Unquote(quotedString)
				if err != nil {
					return nil, motmedelErrors.NewWithTrace(
						fmt.Errorf("strvconv unquote (quoted-string): %w", err),
						quotedString,
					)
				}
			} else {
				directiveStringValue = string(abnfUtils.ExtractPathValue(data, directiveValuePath))
			}
		}

		lowercaseDirectiveName := strings.ToLower(directiveName)
		if _, ok := directiveNameSet[lowercaseDirectiveName]; ok {
			return nil, nil
			//return nil, &MultipleSameNameDirectivesError{
			//	Error: motmedelErrors.Error{
			//		Message: "Multiple directives with the same name were encountered.",
			//		Input:   lowercaseDirectiveName,
			//	},
			//}
		}
		directiveNameSet[lowercaseDirectiveName] = struct{}{}

		switch lowercaseDirectiveName {
		case maxAgeDirectiveName:
			if !digitRegexp.MatchString(directiveStringValue) {
				return nil, nil
				//return nil, &BadMaxAgeFormatError{
				//	Error: motmedelErrors.Error{
				//		Message: "A max-age directive value is not only digits.",
				//		Input:   directiveStringValue,
				//	},
				//}
			}

			maxAgeNumber, err := strconv.Atoi(directiveStringValue)
			if err != nil {
				return nil, motmedelErrors.NewWithTrace(
					fmt.Errorf("strvconv atoi (max-age): %w", err),
					directiveStringValue,
				)
			}

			strictTransportPolicy.MaxAge = maxAgeNumber
		case "includesubdomains":
			if directiveValuePath != nil {
				return nil, nil
				//return nil, &NonValuelessIncludeSubdomainsError{
				//	Error: motmedelErrors.Error{
				//		Message: "An includeSubdomains directive was encounter that is not valueless.",
				//		Input:   directiveStringValue,
				//	},
				//}
			}
			strictTransportPolicy.IncludeSubdomains = true
		case "preload":
			if directiveValuePath != nil {
				return nil, nil
			}
			strictTransportPolicy.Preload = true
		}
	}

	if _, ok := directiveNameSet[maxAgeDirectiveName]; !ok {
		return nil, nil
		//return nil, &MissingMaxAgeError{
		//	Error: motmedelErrors.Error{
		//		Message: "The required max-age directive is missing.",
		//		Input:   data,
		//	},
		//}
	}

	if len(directiveNameSet) == 0 {
		return nil, nil
	}

	strictTransportPolicy.Raw = string(data)

	return &strictTransportPolicy, nil
}

func init() {
	var err error
	StrictTransportSecurityGrammar, err = abnf.ParseABNF(grammar)
	if err != nil {
		panic(fmt.Sprintf("goabnf parse abnf (strict transport security grammar): %v", err))
	}
}
