package accept_encoding

import (
	_ "embed"
	"errors"
	"fmt"
	"strconv"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	"github.com/altshiftab/utils_go/pkg/abnf"
	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

//go:embed grammar.abnf
var grammar []byte

var Grammar *abnf.Grammar

var (
	ErrInvalidQvalue = errors.New("invalid qvalue")
)

func Parse(data []byte) (*motmedelHttpTypes.AcceptEncoding, error) {
	paths, err := abnfUtils.GetParsedDataPaths(Grammar, data, "Accept-Encoding")
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, motmedelErrors.NewWithTrace(motmedelErrors.ErrSyntaxError, data)
	}

	var acceptEncoding motmedelHttpTypes.AcceptEncoding

	interestingPaths := abnfUtils.SearchPath(
		paths[0],
		[]string{"element"}, 2, false,
	)
	for _, interestingPath := range interestingPaths {
		codingsPath := abnfUtils.SearchPathSingleName(
			interestingPath,
			"codings",
			1,
			false,
		)
		if codingsPath == nil {
			return nil, motmedelErrors.NewWithTrace(
				fmt.Errorf("%w: %w", motmedelErrors.ErrSemanticError, nil_error.New("codings path")),
			)
		}
		codingsValue := abnfUtils.ExtractPathValue(data, codingsPath)

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
				return nil, motmedelErrors.NewWithTrace(
					fmt.Errorf(
						"%w: %w: strvconv parse float (qvalue): %w",
						motmedelErrors.ErrSemanticError,
						ErrInvalidQvalue,
						err,
					),
					qvaluePath, bitSize,
				)
			}

			qualityValue = float32(parsedQualityValue)
		}

		acceptEncoding.Encodings = append(
			acceptEncoding.Encodings,
			&motmedelHttpTypes.Encoding{Coding: string(codingsValue), QualityValue: qualityValue},
		)
	}

	acceptEncoding.Raw = string(data)

	return &acceptEncoding, nil
}

func init() {
	var err error
	Grammar, err = abnf.ParseABNF(grammar)
	if err != nil {
		panic(fmt.Sprintf("goabnf parse abnf (accept encoding grammar): %v", err))
	}
}
