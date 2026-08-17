package content_type

import (
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/abnf"
	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

//go:embed grammar.abnf
var grammar []byte

var ContentTypeGrammar *abnf.Grammar

var (
	ErrInvalidQuotedParameterValue = errors.New("invalid quoted parameter value")
)

func Parse(data []byte) (*motmedelHttpTypes.ContentType, error) {
	paths, err := abnfUtils.GetParsedDataPaths(ContentTypeGrammar, data, "Content-Type")
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, motmedelErrors.NewWithTrace(motmedelErrors.ErrSyntaxError, data)
	}

	var contentType motmedelHttpTypes.ContentType

	interestingPaths := abnfUtils.SearchPath(
		paths[0],
		[]string{"type", "subtype", "parameter"}, 2, false,
	)
	for _, interestingPath := range interestingPaths {
		value := string(abnfUtils.ExtractPathValue(data, interestingPath))
		switch interestingPath.MatchRule {
		case "type":
			contentType.Type = value
		case "subtype":
			contentType.Subtype = value
		case "parameter":
			key, parameterValue, _ := strings.Cut(value, "=")

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
					return nil, motmedelErrors.NewWithTrace(
						fmt.Errorf(
							"%w: %w: strvconv unquote: %w",
							motmedelErrors.ErrSemanticError,
							ErrInvalidQuotedParameterValue,
							err,
						),
						quotedString,
					)
				}
			}

			contentType.Parameters = append(contentType.Parameters, [2]string{key, parameterValue})
		}
	}

	return &contentType, nil
}

func init() {
	var err error
	ContentTypeGrammar, err = abnf.ParseABNF(grammar)
	if err != nil {
		panic(fmt.Sprintf("goabnf parse abnf (content type grammar): %v", err))
	}
}
