// Package commit holds a commit as the GitHub API reports one.
package commit

import (
	"fmt"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

// TimeLayout is how the API writes a commit date.
const TimeLayout = "2006-01-02T15:04:05Z"

type Committer struct {
	Name  string `json:"name,omitzero"`
	Email string `json:"email,omitzero"`
	Date  string `json:"date,omitzero"`
}

type TreeReference struct {
	Sha string `json:"sha,omitzero"`
	Url string `json:"url,omitzero"`
}

type Details struct {
	Message   string         `json:"message,omitzero"`
	Committer *Committer     `json:"committer,omitzero"`
	Tree      *TreeReference `json:"tree,omitzero"`
}

type Commit struct {
	Sha    string   `json:"sha,omitzero"`
	Url    string   `json:"url,omitzero"`
	Commit *Details `json:"commit,omitzero"`
}

// Time returns when the commit was made.
func (commit *Commit) Time() (time.Time, error) {
	if commit == nil {
		return time.Time{}, altshiftErrors.NewWithTrace(nil_error.New("commit"))
	}
	if commit.Commit == nil {
		return time.Time{}, altshiftErrors.NewWithTrace(nil_error.New("commit details"))
	}
	if commit.Commit.Committer == nil {
		return time.Time{}, altshiftErrors.NewWithTrace(nil_error.New("commit committer"))
	}

	date := commit.Commit.Committer.Date
	parsed, err := time.Parse(TimeLayout, date)
	if err != nil {
		return time.Time{}, altshiftErrors.NewWithTrace(fmt.Errorf("time parse: %w", err), date)
	}

	return parsed, nil
}

// TreeSha returns the sha of the tree the commit points at, which is what a
// listing of the repository at that commit is asked for by.
func (commit *Commit) TreeSha() string {
	if commit == nil || commit.Commit == nil || commit.Commit.Tree == nil {
		return ""
	}

	return commit.Commit.Tree.Sha
}
