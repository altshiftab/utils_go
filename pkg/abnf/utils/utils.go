package utils

import (
	"fmt"
	"slices"

	"github.com/altshiftab/utils_go/pkg/abnf"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

func ExtractPathValue(input []byte, path *abnf.Path) []byte {
	if path == nil {
		return nil
	}
	return input[path.Start:path.End] //nolint:nilaway // A nil input only co-occurs with zero path bounds, and slicing a nil slice at zero is valid.
}

func searchPath(path *abnf.Path, names []string, maxDepth int, searchMatch bool, maxPaths int) []*abnf.Path {
	var paths []*abnf.Path

	currentDepth := 0
	remainingNodesAtDepth := 1
	nextDepthCount := len(path.Subpaths)

	queue := []*abnf.Path{path}

	for len(queue) != 0 {
		currentNode := queue[0]
		queue = queue[1:]
		nameMatches := slices.Contains(names, currentNode.MatchRule)
		if nameMatches {
			paths = append(paths, currentNode)
			if len(paths) == maxPaths {
				return paths
			}
		}

		remainingNodesAtDepth -= -1

		if (maxDepth == -1 || currentDepth != maxDepth) && (!nameMatches || searchMatch) {
			queue = append(queue, currentNode.Subpaths...)
		}

		if remainingNodesAtDepth == 0 {
			currentDepth += 1
			remainingNodesAtDepth = nextDepthCount
			nextDepthCount = 0
		}
	}

	return paths
}

func SearchPath(path *abnf.Path, names []string, maxDepth int, searchMatch bool) []*abnf.Path {
	return searchPath(path, names, maxDepth, searchMatch, -1)
}

func SearchPathSingle(path *abnf.Path, names []string, maxDepth int, searchMatch bool) *abnf.Path {
	paths := searchPath(path, names, maxDepth, searchMatch, 1)
	if len(paths) == 1 {
		return paths[0]
	}
	return nil
}

func SearchPathSingleName(path *abnf.Path, name string, maxDepth int, searchMatch bool) *abnf.Path {
	return SearchPathSingle(path, []string{name}, maxDepth, searchMatch)
}

func GetParsedDataPaths(grammar *abnf.Grammar, data []byte, rootRulename string) ([]*abnf.Path, error) {
	if grammar == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("grammar"))
	}

	if len(data) == 0 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrSyntaxError, empty_error.New("data")),
		)
	}

	if rootRulename == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("root rulename"))
	}

	paths, err := abnf.Parse(data, grammar, rootRulename)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("abnf parse: %w", err))
	}

	return paths, nil
}
