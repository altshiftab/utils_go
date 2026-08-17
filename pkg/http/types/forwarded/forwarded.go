package forwarded

import (
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/abnf"
	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/types"
)

//go:embed grammar.abnf
var grammar []byte

var Grammar *abnf.Grammar

var (
	ErrInvalidQuotedValue = errors.New("invalid quoted value")
)

func Parse(data []byte) (*types.Forwarded, error) {
	paths, err := abnfUtils.GetParsedDataPaths(Grammar, data, "Forwarded")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError, data)
	}

	var forwarded types.Forwarded

	elementPaths := abnfUtils.SearchPath(
		paths[0],
		[]string{"forwarded-element"}, 2, false,
	)

	for _, elementPath := range elementPaths {
		element := &types.ForwardedElement{}

		pairPaths := abnfUtils.SearchPath(
			elementPath,
			[]string{"forwarded-pair"}, 2, false,
		)

		for _, pairPath := range pairPaths {
			pairValue := string(abnfUtils.ExtractPathValue(data, pairPath))

			name, value, found := strings.Cut(pairValue, "=")
			if !found {
				continue
			}

			name = strings.ToLower(strings.TrimSpace(name))
			value = strings.TrimSpace(value)

			quotedStringPath := abnfUtils.SearchPathSingleName(
				pairPath,
				"quoted-string",
				1,
				false,
			)
			if quotedStringPath != nil {
				quotedString := string(abnfUtils.ExtractPathValue(data, quotedStringPath))
				unquotedValue, err := strconv.Unquote(quotedString)
				if err != nil {
					return nil, altshiftErrors.NewWithTrace(
						fmt.Errorf(
							"%w: %w: strconv unquote: %w",
							altshiftErrors.ErrSemanticError,
							ErrInvalidQuotedValue,
							err,
						),
						quotedString,
					)
				}
				value = unquotedValue
			}

			switch name {
			case "for":
				element.For = value
			case "by":
				element.By = value
			case "host":
				element.Host = value
			case "proto":
				element.Proto = value
			default:
				if element.Extensions == nil {
					element.Extensions = make(map[string]string)
				}
				element.Extensions[name] = value
			}
		}

		forwarded.Elements = append(forwarded.Elements, element)
	}

	return &forwarded, nil
}

func init() {
	var err error
	Grammar, err = abnf.ParseABNF(grammar)
	if err != nil {
		panic(fmt.Sprintf("goabnf parse abnf (forwarded grammar): %v", err))
	}
}
