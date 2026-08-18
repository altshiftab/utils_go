// Package tree holds a repository listing as the GitHub API reports one.
package tree

// Entry is one path in a tree.
type Entry struct {
	Path string `json:"path,omitzero"`
	Mode string `json:"mode,omitzero"`
	Type string `json:"type,omitzero"`
	Sha  string `json:"sha,omitzero"`
	Size int    `json:"size,omitzero"`
	Url  string `json:"url,omitzero"`
}

type Tree struct {
	Sha  string   `json:"sha,omitzero"`
	Url  string   `json:"url,omitzero"`
	Tree []*Entry `json:"tree,omitzero"`
	// Truncated reports that the API did not return the whole listing. A
	// caller searching a truncated tree and finding nothing has not learned
	// that the path is absent, only that it was not among what came back.
	Truncated bool `json:"truncated,omitzero"`
}

// Find returns the entry at path, and whether it was there.
func (tree *Tree) Find(path string) (*Entry, bool) {
	if tree == nil {
		return nil, false
	}

	for _, entry := range tree.Tree {
		if entry != nil && entry.Path == path {
			return entry, true
		}
	}

	return nil, false
}
