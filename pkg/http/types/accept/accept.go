package accept

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

var (
	ErrParameterNamedQ        = errors.New("parameter named q")
	ErrInvalidQvalue          = errors.New("invalid qvalue")
	ErrCouldNotSplitParameter = errors.New("could not split parameter")
)

func Parse(data []byte) (*altshiftHttpTypes.Accept, error) {
	paths, err := abnfUtils.GetParsedDataPaths(Grammar, data, "Accept")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError, data)
	}

	accept := &altshiftHttpTypes.Accept{Raw: string(data)}

	interestingPaths := abnfUtils.SearchPath(
		paths[0],
		[]string{"media-range"},
		2,
		false,
	)

	for _, interestingPath := range interestingPaths {
		if interestingPath == nil {
			continue
		}

		mediaRange := &altshiftHttpTypes.MediaRange{Weight: 1.0}

		mediaRangeType := "*"
		mediaRangeSubtype := "*"

		if typePath := abnfUtils.SearchPathSingle(interestingPath, []string{"type"}, 1, false); typePath != nil {
			mediaRangeType = string(abnfUtils.ExtractPathValue(data, typePath))
		}

		if subtypePath := abnfUtils.SearchPathSingle(interestingPath, []string{"subtype"}, 1, false); subtypePath != nil {
			mediaRangeSubtype = string(abnfUtils.ExtractPathValue(data, subtypePath))
		}

		mediaRange.Type = mediaRangeType
		mediaRange.Subtype = mediaRangeSubtype

		parameterPaths := abnfUtils.SearchPath(interestingPath, []string{"weight", "parameter"}, 2, false)
		for i, parameterPath := range parameterPaths {
			if parameterPath.MatchRule == "weight" {
				if i != len(parameterPaths)-1 {
					return nil, altshiftErrors.NewWithTrace(
						fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, ErrParameterNamedQ),
					)
				}

				qValuePath := abnfUtils.SearchPathSingle(interestingPath, []string{"qvalue"}, 1, false)
				if qValuePath == nil {
					return nil, altshiftErrors.NewWithTrace(
						fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, nil_error.New("qvalue path")),
					)
				}

				qValueString := string(abnfUtils.ExtractPathValue(data, qValuePath))
				bitSize := 32
				parsedWeight, err := strconv.ParseFloat(qValueString, bitSize)
				if err != nil {
					return nil, altshiftErrors.NewWithTrace(
						fmt.Errorf(
							"%w: %w: strvconv parse float: %w",
							altshiftErrors.ErrSemanticError, ErrInvalidQvalue, err,
						),
						qValueString, bitSize,
					)
				}

				mediaRange.Weight = float32(parsedWeight)
			} else {
				parameterString := string(abnfUtils.ExtractPathValue(data, parameterPath))
				separator := "='"
				key, value, found := strings.Cut(parameterString, "=")
				if !found {
					return nil, altshiftErrors.NewWithTrace(
						fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, ErrCouldNotSplitParameter),
						parameterString,
						separator,
					)
				}

				mediaRange.Parameters = append(mediaRange.Parameters, [2]string{key, value})
			}
		}

		accept.MediaRanges = append(accept.MediaRanges, mediaRange)
	}

	return accept, nil
}

func init() {
	var err error
	Grammar, err = abnf.ParseABNF(grammar)
	if err != nil {
		panic(fmt.Sprintf("goabnf parse abnf (accept grammar): %v", err))
	}
}
