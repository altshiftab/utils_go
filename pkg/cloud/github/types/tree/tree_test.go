package tree

import "testing"

func TestFind(t *testing.T) {
	t.Parallel()

	listing := &Tree{Tree: []*Entry{
		nil,
		{Path: "README.md", Sha: "aaa"},
		{Path: "nmap-service-probes", Sha: "bbb"},
	}}

	testCases := []struct {
		name    string
		tree    *Tree
		path    string
		wantSha string
		wantOk  bool
	}{
		{name: "an entry that is there", tree: listing, path: "nmap-service-probes", wantSha: "bbb", wantOk: true},
		{name: "the first entry", tree: listing, path: "README.md", wantSha: "aaa", wantOk: true},
		{name: "an entry that is not", tree: listing, path: "missing"},
		{name: "no tree at all", tree: nil, path: "README.md"},
		{name: "an empty tree", tree: &Tree{}, path: "README.md"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			entry, ok := testCase.tree.Find(testCase.path)
			if ok != testCase.wantOk {
				t.Fatalf("ok = %v, want %v", ok, testCase.wantOk)
			}
			if !ok {
				return
			}
			if entry.Sha != testCase.wantSha {
				t.Errorf("Sha = %q, want %q", entry.Sha, testCase.wantSha)
			}
		})
	}
}
