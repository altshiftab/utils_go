package accept_language

import (
	_ "embed"
	"errors"
	"fmt"
	"strconv"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	"github.com/altshiftab/utils_go/pkg/abnf"
	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

//go:embed grammar.abnf
var grammar []byte

var Grammar *abnf.Grammar

var (
	ErrInvalidQvalue = errors.New("invalid qvalue")
)

// wildcardLanguageRange is the language-range of RFC 4647 Section 2.1 that
// matches every language tag.
const wildcardLanguageRange = "*"

func Parse(data []byte) (*altshiftHttpTypes.AcceptLanguage, error) {
	paths, err := abnfUtils.GetParsedDataPaths(Grammar, data, "Accept-Language")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError, data)
	}

	var acceptLanguage altshiftHttpTypes.AcceptLanguage

	interestingPaths := abnfUtils.SearchPath(
		paths[0],
		[]string{"element"}, 2, false,
	)
	for _, interestingPath := range interestingPaths {
		primarySubtagPath := abnfUtils.SearchPathSingleName(
			interestingPath,
			"Primary-subtag",
			2,
			false,
		)

		var primarySubtag []byte
		if primarySubtagPath != nil {
			primarySubtag = abnfUtils.ExtractPathValue(data, primarySubtagPath)
		} else {
			// RFC 4647 Section 2.1 lets a language-range be the wildcard
			// "*", which carries no subtags of its own.
			languageRangePath := abnfUtils.SearchPathSingleName(
				interestingPath,
				"language-range",
				2,
				false,
			)
			if languageRangePath == nil {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, nil_error.New("language range")),
				)
			}

			primarySubtag = abnfUtils.ExtractPathValue(data, languageRangePath)
			if string(primarySubtag) != wildcardLanguageRange {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, nil_error.New("primary subtag")),
				)
			}
		}

		subtagPath := abnfUtils.SearchPathSingleName(
			interestingPath,
			"Subtag",
			2,
			false,
		)
		var subtag []byte
		if subtagPath != nil {
			subtag = abnfUtils.ExtractPathValue(data, subtagPath)
		}

		var qualityValue float32 = 1.0
		qvaluePath := abnfUtils.SearchPathSingleName(
			interestingPath,
			"qvalue",
			1,
			false,
		)
		if qvaluePath != nil {
			qvalueString := string(abnfUtils.ExtractPathValue(data, qvaluePath))
			bitSize := 32
			parsedQualityValue, err := strconv.ParseFloat(qvalueString, bitSize)
			if err != nil {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf(
						"%w: %w: strvconv parse float (qvalue): %w",
						altshiftErrors.ErrSemanticError,
						ErrInvalidQvalue,
						err,
					),
					qvaluePath, bitSize,
				)
			}

			qualityValue = float32(parsedQualityValue)
		}

		acceptLanguage.LanguageQs = append(
			acceptLanguage.LanguageQs,
			&altshiftHttpTypes.LanguageQ{
				Tag: &altshiftHttpTypes.LanguageTag{
					PrimarySubtag: string(primarySubtag),
					Subtag:        string(subtag),
				},
				Q: qualityValue,
			},
		)
	}

	acceptLanguage.Raw = string(data)

	return &acceptLanguage, nil
}

func init() {
	var err error
	Grammar, err = abnf.ParseABNF(grammar)
	if err != nil {
		panic(fmt.Sprintf("goabnf parse abnf (accept encoding grammar): %v", err))
	}
}
