package github

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

// GraphqlPath is where the GraphQL API is served.
const GraphqlPath = "/graphql"

// LastCommitBatchSize is how many files one request to LastCommitTimes asks about. It keeps a
// request well inside the time GitHub gives a GraphQL query before abandoning it.
const LastCommitBatchSize = 50

// File names a file in a repository: the account the repository belongs to, its name, and the
// file's path in it.
type File struct {
	Owner string
	Repo  string
	Path  string
}

// LastCommitTimes returns when each file was last committed to on its repository's default
// branch, which is the branch code search indexes.
//
// It is LatestCommit for many files at once. Asked through REST, every file is a request against
// an allowance of five thousand an hour, and the files a code search turns up for one organisation
// run to hundreds, spread over as many repositories. Through GraphQL a request asks about a batch
// of files, and costs about one point of that allowance however many it names.
//
// A file the API could not answer for -- its repository deleted, renamed or made private since it
// was found, or a path no commit on the default branch touches -- is absent from the result rather
// than failing the rest. A request the API refused outright is an error.
func (c *Client) LastCommitTimes(
	ctx context.Context,
	files []*File,
	options ...fetch_config.Option,
) (map[File]*time.Time, error) {
	if c == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("client"))
	}

	unique := make([]File, 0, len(files))
	seen := make(map[File]struct{}, len(files))
	for _, file := range files {
		if file == nil {
			return nil, altshiftErrors.NewWithTrace(nil_error.New("file"))
		}
		if err := checkRepository(file.Owner, file.Repo); err != nil {
			return nil, err
		}
		if file.Path == "" {
			return nil, altshiftErrors.NewWithTrace(empty_error.New("path"))
		}

		if _, ok := seen[*file]; ok {
			continue
		}
		seen[*file] = struct{}{}
		unique = append(unique, *file)
	}

	times := make(map[File]*time.Time, len(unique))

	urlString := rest.Url(c.baseUrl, nil, strings.TrimPrefix(GraphqlPath, "/"))

	for start := 0; start < len(unique); start += LastCommitBatchSize {
		batch := unique[start:min(start+LastCommitBatchSize, len(unique))]

		response, err := rest.SendJson[lastCommitResponse](
			ctx,
			http.MethodPost,
			urlString,
			lastCommitRequest(batch),
			// slices.Concat rather than append, for the reason SearchCode gives.
			slices.Concat(c.config.FetchOptions, options),
		)
		if err != nil {
			return nil, fmt.Errorf("send json: %w", err)
		}

		// GraphQL answers a query it could partly run with the parts it could and an error for
		// each part it could not. With nothing answered at all, the errors are about the request.
		if len(response.Data) == 0 {
			if len(response.Errors) != 0 && response.Errors[0] != nil {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: %s", ErrGraphql, response.Errors[0].Message),
					len(batch),
				)
			}

			return nil, altshiftErrors.NewWithTrace(nil_error.New("graphql data"))
		}

		for index, file := range batch {
			at, err := response.Data[fileAlias(index)].lastCommitted()
			if err != nil {
				return nil, altshiftErrors.NewWithTrace(err, file.Owner, file.Repo, file.Path)
			}
			if at != nil {
				times[file] = at
			}
		}
	}

	return times, nil
}

// lastCommitRequest asks for the last commit touching each file of a batch. The names and paths go
// in as variables rather than into the query's text, so that nothing in them is read as GraphQL.
func lastCommitRequest(batch []File) *graphqlRequest {
	var query strings.Builder
	variables := make(map[string]string, 3*len(batch))

	query.WriteString("query(")
	for index := range batch {
		suffix := strconv.Itoa(index)
		if index > 0 {
			query.WriteString(", ")
		}
		query.WriteString("$o" + suffix + ": String!, $n" + suffix + ": String!, $p" + suffix + ": String!")
	}
	query.WriteString(") {")

	for index, file := range batch {
		suffix := strconv.Itoa(index)
		variables["o"+suffix] = file.Owner
		variables["n"+suffix] = file.Repo
		variables["p"+suffix] = file.Path

		query.WriteString(" " + fileAlias(index) + ": repository(owner: $o" + suffix + ", name: $n" + suffix + ")")
		query.WriteString(` { object(expression: "HEAD") { ... on Commit { history(first: 1, path: $p` + suffix + `)`)
		query.WriteString(" { nodes { committedDate } } } } }")
	}
	query.WriteString(" }")

	return &graphqlRequest{Query: query.String(), Variables: variables}
}

// fileAlias is what the answer about a batch's index-th file is filed under.
func fileAlias(index int) string {
	return "f" + strconv.Itoa(index)
}

type graphqlRequest struct {
	Query     string            `json:"query"`
	Variables map[string]string `json:"variables"`
}

type graphqlError struct {
	Message string `json:"message,omitzero"`
	Type    string `json:"type,omitzero"`
}

type lastCommitNode struct {
	CommittedDate string `json:"committedDate,omitzero"`
}

type lastCommitHistory struct {
	Nodes []*lastCommitNode `json:"nodes,omitzero"`
}

type lastCommitObject struct {
	History *lastCommitHistory `json:"history,omitzero"`
}

type lastCommitRepository struct {
	Object *lastCommitObject `json:"object,omitzero"`
}

// lastCommitted is when the file was last committed to, or nil where the answer has no commit:
// the repository was not found, or no commit touches the path.
func (repository *lastCommitRepository) lastCommitted() (*time.Time, error) {
	if repository == nil || repository.Object == nil || repository.Object.History == nil {
		return nil, nil
	}

	nodes := repository.Object.History.Nodes
	if len(nodes) == 0 || nodes[0] == nil || nodes[0].CommittedDate == "" {
		return nil, nil
	}

	at, err := time.Parse(time.RFC3339, nodes[0].CommittedDate)
	if err != nil {
		return nil, fmt.Errorf("time parse: %w", err)
	}

	return &at, nil
}

type lastCommitResponse struct {
	Data   map[string]*lastCommitRepository `json:"data,omitzero"`
	Errors []*graphqlError                  `json:"errors,omitzero"`
}
