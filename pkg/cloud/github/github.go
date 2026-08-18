// Package github reads repository contents from the GitHub API.
//
// It covers what a generator needs to track a file published upstream: find the
// commit that last touched it, list the tree at that commit, and fetch a file or
// the whole repository at it. Going through the API rather than cloning keeps
// the fetch to what is wanted, and yields a commit SHA to record alongside
// whatever was generated so a result can be traced back to what produced it.
package github

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net/url"

	"github.com/altshiftab/utils_go/pkg/cloud/github/github_config"
	"github.com/altshiftab/utils_go/pkg/cloud/github/types/blob"
	"github.com/altshiftab/utils_go/pkg/cloud/github/types/commit"
	"github.com/altshiftab/utils_go/pkg/cloud/github/types/tree"
	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

const (
	DefaultHost        = "api.github.com"
	DefaultArchiveHost = "github.com"
)

type Client struct {
	baseUrl        *url.URL
	archiveBaseUrl *url.URL
	config         *github_config.Config
}

func NewClient(options ...github_config.Option) *Client {
	config := github_config.New(options...)

	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = &url.URL{Scheme: "https", Host: DefaultHost}
	}

	archiveBaseUrl := config.ArchiveBaseUrl
	if archiveBaseUrl == nil {
		archiveBaseUrl = &url.URL{Scheme: "https", Host: DefaultArchiveHost}
	}

	base := *baseUrl
	archive := *archiveBaseUrl

	return &Client{baseUrl: &base, archiveBaseUrl: &archive, config: config}
}

// repositoryUrl returns the address of a path under a repository's API.
func (c *Client) repositoryUrl(owner string, repo string, elements ...string) *url.URL {
	u := *c.baseUrl
	u.Path += "/repos/" + owner + "/" + repo
	for _, element := range elements {
		u.Path += "/" + element
	}

	return &u
}

// checkRepository returns an error if the repository is not named.
func checkRepository(owner string, repo string) error {
	if owner == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("owner"))
	}
	if repo == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("repo"))
	}

	return nil
}

// LatestCommit returns the commit that last touched path.
//
// Asking for the commit behind a single path, rather than the head of the
// branch, is what makes a generated file's provenance meaningful: the SHA names
// the revision that last changed the thing it was generated from.
func (c *Client) LatestCommit(
	ctx context.Context,
	owner string,
	repo string,
	path string,
	options ...fetch_config.Option,
) (*commit.Commit, error) {
	if c == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("client"))
	}
	if err := checkRepository(owner, repo); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("path"))
	}

	u := c.repositoryUrl(owner, repo, "commits")
	query := u.Query()
	query.Set("per_page", "1")
	query.Set("path", path)
	u.RawQuery = query.Encode()

	urlString := u.String()

	commits, err := rest.GetJson[[]*commit.Commit](ctx, urlString, append(c.config.FetchOptions, options...))
	if err != nil {
		return nil, fmt.Errorf("get json: %w", err)
	}

	// Exactly one commit was asked for. Taking the first of several would pin
	// whatever is generated to a revision that was not the one requested.
	if len(*commits) != 1 {
		return nil, altshiftErrors.NewWithTrace(ErrUnexpectedCommitCount, len(*commits), urlString)
	}

	latest := (*commits)[0]
	if latest == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("commit"))
	}

	return latest, nil
}

// Tree returns the repository listing a tree sha names.
func (c *Client) Tree(
	ctx context.Context,
	owner string,
	repo string,
	treeSha string,
	options ...fetch_config.Option,
) (*tree.Tree, error) {
	if c == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("client"))
	}
	if err := checkRepository(owner, repo); err != nil {
		return nil, err
	}
	if treeSha == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("tree sha"))
	}

	urlString := c.repositoryUrl(owner, repo, "git", "trees", treeSha).String()

	value, err := rest.GetJson[tree.Tree](ctx, urlString, append(c.config.FetchOptions, options...))
	if err != nil {
		return nil, fmt.Errorf("get json: %w", err)
	}

	return value, nil
}

// Blob returns a file's contents.
func (c *Client) Blob(
	ctx context.Context,
	owner string,
	repo string,
	fileSha string,
	options ...fetch_config.Option,
) (*blob.Blob, error) {
	if c == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("client"))
	}
	if err := checkRepository(owner, repo); err != nil {
		return nil, err
	}
	if fileSha == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("file sha"))
	}

	urlString := c.repositoryUrl(owner, repo, "git", "blobs", fileSha).String()

	value, err := rest.GetJson[blob.Blob](ctx, urlString, append(c.config.FetchOptions, options...))
	if err != nil {
		return nil, fmt.Errorf("get json: %w", err)
	}

	return value, nil
}

// CommitArchive returns the repository at a revision, as an archive.
func (c *Client) CommitArchive(
	ctx context.Context,
	owner string,
	repo string,
	reference string,
	options ...fetch_config.Option,
) (*zip.Reader, error) {
	if c == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("client"))
	}
	if err := checkRepository(owner, repo); err != nil {
		return nil, err
	}
	if reference == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("reference"))
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	u := *c.archiveBaseUrl
	u.Path += "/" + owner + "/" + repo + "/archive/" + reference + ".zip"
	urlString := u.String()

	_, body, err := altshiftHttpUtils.Fetch(ctx, urlString, append(c.config.FetchOptions, options...)...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch: %w", err), urlString)
	}

	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("zip new reader: %w", err), urlString)
	}

	return reader, nil
}
