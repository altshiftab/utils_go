package cache_control

import (
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	"github.com/altshiftab/utils_go/pkg/abnf"
	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

//go:embed grammar.abnf
var grammar []byte

var Grammar *abnf.Grammar

var deltaSecondsDirectives = map[string]bool{
	"max-age":   true,
	"max-stale": true,
	"min-fresh": true,
	"s-maxage":  true,
}

var (
	ErrInvalidDeltaSeconds = errors.New("invalid delta-seconds")
)

func Parse(data []byte) (*altshiftHttpTypes.CacheControl, error) {
	paths, err := abnfUtils.GetParsedDataPaths(Grammar, data, "Cache-Control")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError, data)
	}

	var cacheControl altshiftHttpTypes.CacheControl

	directivePaths := abnfUtils.SearchPath(
		paths[0],
		[]string{"cache-directive"}, 2, false,
	)

	for _, directivePath := range directivePaths {
		directiveNamePath := abnfUtils.SearchPathSingleName(
			directivePath,
			"directive-name",
			1,
			false,
		)
		if directiveNamePath == nil {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, nil_error.New("directive name path")),
			)
		}
		directiveName := strings.ToLower(string(abnfUtils.ExtractPathValue(data, directiveNamePath)))

		var directiveValue string
		directiveValuePath := abnfUtils.SearchPathSingleName(
			directivePath,
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
				quotedString := string(abnfUtils.ExtractPathValue(data, quotedStringPath))
				unquoted, err := strconv.Unquote(quotedString)
				if err != nil {
					return nil, altshiftErrors.NewWithTrace(
						fmt.Errorf("strconv unquote (quoted-string): %w", err),
						quotedString,
					)
				}
				directiveValue = unquoted
			} else {
				directiveValue = string(abnfUtils.ExtractPathValue(data, directiveValuePath))
			}
		}

		if deltaSecondsDirectives[directiveName] && (directiveName != "max-stale" || directiveValue != "") {
			if _, err := strconv.Atoi(directiveValue); err != nil {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf(
						"%w: %w: strconv atoi (%s): %w",
						altshiftErrors.ErrSemanticError,
						ErrInvalidDeltaSeconds,
						directiveName,
						err,
					),
					directiveValue,
				)
			}
		}

		cacheControl.Directives = append(
			cacheControl.Directives,
			&altshiftHttpTypes.CacheControlDirective{
				Name:  directiveName,
				Value: directiveValue,
			},
		)
	}

	cacheControl.Raw = string(data)

	return &cacheControl, nil
}

func init() {
	var err error
	Grammar, err = abnf.ParseABNF(grammar)
	if err != nil {
		panic(fmt.Sprintf("goabnf parse abnf (cache control grammar): %v", err))
	}
}
