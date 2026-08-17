package content_type

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

//go:embed grammar.abnf
var grammar []byte

var ContentTypeGrammar *abnf.Grammar

var (
	ErrInvalidQuotedParameterValue = errors.New("invalid quoted parameter value")
)

func Parse(data []byte) (*altshiftHttpTypes.ContentType, error) {
	paths, err := abnfUtils.GetParsedDataPaths(ContentTypeGrammar, data, "Content-Type")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError, data)
	}

	var contentType altshiftHttpTypes.ContentType

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
