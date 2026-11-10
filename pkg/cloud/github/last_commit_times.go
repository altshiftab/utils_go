package github

import (
	"context"
	"errors"
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
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

// GraphqlPath is where the GraphQL API is served.
const GraphqlPath = "/graphql"

// LastCommitBatchSize is how many files one request to LastCommitTimes asks about.
//
// GitHub abandons a GraphQL query after ten seconds, answering 502, and walking a file's history
// takes a tenth of a second for an ordinary repository; fifty took five or six seconds, and one
// slow file took a batch past the limit. Twenty leaves room for a slow one.
const LastCommitBatchSize = 20

// File names a file in a repository: the account the repository belongs to, its name, and the
// file's path in it.
type File struct {
	Owner string
	Repo  string
	Path  string
}

// LastCommitTimes returns when each file was last committed to on its repository's default
// branch, which is the branch code search indexes, and the files GitHub could not date.
//
// It is LatestCommit for many files at once. Asked through REST, every file is a request against
// an allowance of five thousand an hour, and the files a code search turns up for one organisation
// run to hundreds, spread over as many repositories. Through GraphQL a request asks about a batch
// of files, and costs about one point of that allowance however many it names.
//
// A file the API could not resolve -- its repository deleted, renamed or made private since it was
// found, or a path no commit on the default branch touches -- is in neither answer. A batch GitHub
// could not run in the time it gives a query is split and asked again, until the file it could
// not run is asked about alone; one it cannot answer for even then is undated. That is a property
// of the file rather than a failure of the request: a mirror of a package repository has a history
// GitHub cannot walk for one path in ten seconds, through GraphQL or REST, however often it is
// asked. How many undated files are too many -- GitHub not answering at all, rather than a few files
// it cannot date -- is the caller's to say. Anything else fails the call: a request refused
// outright, or an error of any other kind.
func (c *Client) LastCommitTimes(
	ctx context.Context,
	files []*File,
	options ...fetch_config.Option,
) (map[File]*time.Time, []*File, error) {
	if c == nil {
		return nil, nil, altshiftErrors.NewWithTrace(nil_error.New("client"))
	}

	unique := make([]File, 0, len(files))
	seen := make(map[File]struct{}, len(files))
	for _, file := range files {
		if file == nil {
			return nil, nil, altshiftErrors.NewWithTrace(nil_error.New("file"))
		}
		if err := checkRepository(file.Owner, file.Repo); err != nil {
			return nil, nil, err
		}
		if file.Path == "" {
			return nil, nil, altshiftErrors.NewWithTrace(empty_error.New("path"))
		}

		if _, ok := seen[*file]; ok {
			continue
		}
		seen[*file] = struct{}{}
		unique = append(unique, *file)
	}

	lookup := &lastCommitLookup{
		client:    c,
		urlString: rest.Url(c.baseUrl, nil, strings.TrimPrefix(GraphqlPath, "/")),
		// slices.Concat rather than append, for the reason SearchCode gives.
		options: slices.Concat(c.config.FetchOptions, options),
		times:   make(map[File]*time.Time, len(unique)),
	}

	for start := 0; start < len(unique); start += LastCommitBatchSize {
		if err := lookup.ask(ctx, unique[start:min(start+LastCommitBatchSize, len(unique))]); err != nil {
			return nil, nil, err
		}
	}

	return lookup.times, lookup.undated, nil
}

// lastCommitLookup is one call to LastCommitTimes: where it asks, with what, and what it has learnt.
type lastCommitLookup struct {
	client    *Client
	urlString string
	options   []fetch_config.Option
	times     map[File]*time.Time
	undated   []*File
}

// ask asks about a batch, and splits it where GitHub could not run it.
func (lookup *lastCommitLookup) ask(ctx context.Context, batch []File) error {
	response, err := rest.SendJson[lastCommitResponse](
		ctx,
		http.MethodPost,
		lookup.urlString,
		lastCommitRequest(batch),
		lookup.options,
	)
	if err != nil {
		// A server error is GitHub abandoning the query, which a slow file in the batch is what
		// makes it do.
		if statusErr, ok := errors.AsType[*altshiftHttpErrors.Non2xxStatusCodeError](err); ok &&
			statusErr.StatusCode >= http.StatusInternalServerError {
			return lookup.split(ctx, batch)
		}

		return fmt.Errorf("send json: %w", err)
	}

	// GraphQL answers a query it could partly run with the parts it could and an error for each
	// part it could not. A repository it could not resolve is an answer -- the file is left out. A
	// part it could not run says so with no type, or as unavailable, and the batch is split. Any
	// other error fails the call: a part it was refused would otherwise read as a file nobody has
	// committed to.
	ran := true
	for _, graphqlErr := range response.Errors {
		switch {
		case graphqlErr == nil, graphqlErr.Type == graphqlNotFound:
		case graphqlErr.Type == "", graphqlErr.Type == graphqlServiceUnavailable:
			ran = false
		default:
			return altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %s: %s", ErrGraphql, graphqlErr.Type, graphqlErr.Message),
				len(batch),
			)
		}
	}
	if !ran {
		return lookup.split(ctx, batch)
	}

	// With nothing answered at all, there was nothing to answer.
	if len(response.Data) == 0 {
		return altshiftErrors.NewWithTrace(nil_error.New("graphql data"))
	}

	for index, file := range batch {
		at, err := response.Data[fileAlias(index)].lastCommitted()
		if err != nil {
			return altshiftErrors.NewWithTrace(err, file.Owner, file.Repo, file.Path)
		}
		if at != nil {
			lookup.times[file] = at
		}
	}

	return nil
}

// split asks about the halves of a batch GitHub could not run, until the file it could not run is
// asked about alone and, failing again, is undated.
func (lookup *lastCommitLookup) split(ctx context.Context, batch []File) error {
	if len(batch) == 1 {
		lookup.undated = append(lookup.undated, &batch[0])
		return nil
	}

	half := len(batch) / 2
	if err := lookup.ask(ctx, batch[:half]); err != nil {
		return err
	}

	return lookup.ask(ctx, batch[half:])
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

// graphqlNotFound is the type of the error GraphQL gives for a repository it could not resolve, and
// graphqlServiceUnavailable the one it gives for a part of a query it could not run.
const (
	graphqlNotFound           = "NOT_FOUND"
	graphqlServiceUnavailable = "SERVICE_UNAVAILABLE"
)

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
