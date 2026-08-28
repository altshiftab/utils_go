// Package code_search holds what GitHub's code search answers with.
package code_search

// Owner is the account a repository belongs to.
type Owner struct {
	Login string `json:"login,omitzero"`
	Type  string `json:"type,omitzero"`
}

// Repository is the repository a match was found in.
type Repository struct {
	Name string `json:"name,omitzero"`
	// FullName is "owner/repo", which is how a repository is named to a person.
	FullName string  `json:"full_name,omitzero"`
	HtmlUrl  string  `json:"html_url,omitzero"`
	Private  bool    `json:"private,omitzero"`
	Fork     bool    `json:"fork,omitzero"`
	Owner    *Owner  `json:"owner,omitzero"`
	Score    float64 `json:"score,omitzero"`
}

// Match is where inside a fragment the search term was found. The indices are into the fragment,
// which is what lets a consumer highlight the term rather than the whole line.
type Match struct {
	Text    string `json:"text,omitzero"`
	Indices []int  `json:"indices,omitzero"`
}

// TextMatch is one excerpt of a file around a match.
//
// It arrives only when the request asks for the text-match media type. Without it a result says
// which file matched but not what in it did, and the excerpt is the whole difference between a
// finding an operator can judge and one they must go and open.
type TextMatch struct {
	ObjectUrl  string   `json:"object_url,omitzero"`
	ObjectType string   `json:"object_type,omitzero"`
	Property   string   `json:"property,omitzero"`
	Fragment   string   `json:"fragment,omitzero"`
	Matches    []*Match `json:"matches,omitzero"`
}

// Item is one file the search matched.
type Item struct {
	Name string `json:"name,omitzero"`
	Path string `json:"path,omitzero"`
	Sha  string `json:"sha,omitzero"`
	// Url is the API address of the file's contents; HtmlUrl is the page a person would open.
	Url         string       `json:"url,omitzero"`
	GitUrl      string       `json:"git_url,omitzero"`
	HtmlUrl     string       `json:"html_url,omitzero"`
	Repository  *Repository  `json:"repository,omitzero"`
	Score       float64      `json:"score,omitzero"`
	TextMatches []*TextMatch `json:"text_matches,omitzero"`
}

// Response is one page of a code search.
type Response struct {
	TotalCount int `json:"total_count,omitzero"`
	// IncompleteResults is GitHub saying it gave up before searching everything, which it does
	// under load. The results are still results; there may simply be more.
	IncompleteResults bool    `json:"incomplete_results,omitzero"`
	Items             []*Item `json:"items,omitzero"`
}
