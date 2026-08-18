package strict_transport_security

import (
	_ "embed"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/abnf"
	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

//go:embed grammar.abnf
var grammar []byte

var StrictTransportSecurityGrammar *abnf.Grammar

const (
	maxAgeDirectiveName = "max-age"
)

var digitRegexp = regexp.MustCompile(`^\d+$`)

// TODO: Update to use proper errors

var (
	// ErrMissingDirectiveName is returned when a directive carries no name at all.
	ErrMissingDirectiveName = errors.New("missing directive name")
	// ErrDuplicateDirective is returned when the same directive appears twice.
	ErrDuplicateDirective = errors.New("duplicate directive")
	// ErrBadMaxAgeFormat is returned when max-age is not a plain number.
	ErrBadMaxAgeFormat = errors.New("max-age value is not only digits")
	// ErrNonValuelessDirective is returned when a valueless directive carries a value.
	ErrNonValuelessDirective = errors.New("valueless directive carries a value")
	// ErrMissingMaxAge is returned when the required max-age directive is absent.
	ErrMissingMaxAge = errors.New("missing max-age directive")
)

func Parse(data []byte) (*altshiftHttpTypes.StrictTransportSecurityPolicy, error) {
	paths, err := abnfUtils.GetParsedDataPaths(StrictTransportSecurityGrammar, data, "Strict-Transport-Security")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError, data)
	}

	directiveNameSet := make(map[string]struct{})

	var strictTransportPolicy altshiftHttpTypes.StrictTransportSecurityPolicy

	interestingPaths := abnfUtils.SearchPath(paths[0], []string{"directive"}, 2, false)
	for _, interestingPath := range interestingPaths {
		directiveNamePath := abnfUtils.SearchPathSingleName(
			interestingPath,
			"directive-name",
			1,
			false,
		)
		if directiveNamePath == nil {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %w", altshiftErrors.ErrSyntaxError, ErrMissingDirectiveName),
				data,
			)
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
					return nil, altshiftErrors.NewWithTrace(
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
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, ErrDuplicateDirective),
				lowercaseDirectiveName,
			)
		}
		directiveNameSet[lowercaseDirectiveName] = struct{}{}

		switch lowercaseDirectiveName {
		case maxAgeDirectiveName:
			if !digitRegexp.MatchString(directiveStringValue) {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, ErrBadMaxAgeFormat),
					directiveStringValue,
				)
			}

			maxAgeNumber, err := strconv.Atoi(directiveStringValue)
			if err != nil {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("strvconv atoi (max-age): %w", err),
					directiveStringValue,
				)
			}

			strictTransportPolicy.MaxAge = maxAgeNumber
		case "includesubdomains":
			if directiveValuePath != nil {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, ErrNonValuelessDirective),
					directiveStringValue,
				)
			}
			strictTransportPolicy.IncludeSubdomains = true
		case "preload":
			if directiveValuePath != nil {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, ErrNonValuelessDirective),
					directiveStringValue,
				)
			}
			strictTransportPolicy.Preload = true
		}
	}

	// An empty directive set cannot contain max-age either, so this one check
	// covers both the missing-directive and the no-directives-at-all case.
	if _, ok := directiveNameSet[maxAgeDirectiveName]; !ok {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, ErrMissingMaxAge),
			data,
		)
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
