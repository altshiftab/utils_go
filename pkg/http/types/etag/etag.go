package etag

import (
	_ "embed"
	"fmt"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	"github.com/altshiftab/utils_go/pkg/abnf"
	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

//go:embed grammar.abnf
var grammar []byte

var Grammar *abnf.Grammar

func Parse(data []byte) (*altshiftHttpTypes.ETag, error) {
	paths, err := abnfUtils.GetParsedDataPaths(Grammar, data, "ETag")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError, data)
	}

	var etag altshiftHttpTypes.ETag

	if weakPath := abnfUtils.SearchPathSingleName(paths[0], "weak", 2, false); weakPath != nil {
		etag.Weak = true
	}

	opaqueTagPath := abnfUtils.SearchPathSingleName(paths[0], "opaque-tag", 2, false)
	if opaqueTagPath == nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, nil_error.New("opaque-tag path")),
			data,
		)
	}

	opaqueTag := abnfUtils.ExtractPathValue(data, opaqueTagPath)
	if len(opaqueTag) >= 2 {
		etag.Tag = string(opaqueTag[1 : len(opaqueTag)-1])
	}

	return &etag, nil
}

func init() {
	var err error
	Grammar, err = abnf.ParseABNF(grammar)
	if err != nil {
		panic(fmt.Sprintf("goabnf parse abnf (etag grammar): %v", err))
	}
}
