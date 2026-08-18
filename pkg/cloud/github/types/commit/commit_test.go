package commit

import "testing"

func TestTime(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		commit  *Commit
		want    string
		wantErr bool
	}{
		{
			name:   "a date the api wrote",
			commit: &Commit{Commit: &Details{Committer: &Committer{Date: "2026-04-12T09:31:02Z"}}},
			want:   "2026-04-12T09:31:02Z",
		},
		{
			name:    "a date in another form",
			commit:  &Commit{Commit: &Details{Committer: &Committer{Date: "yesterday"}}},
			wantErr: true,
		},
		{
			name:    "no committer",
			commit:  &Commit{Commit: &Details{}},
			wantErr: true,
		},
		{
			name:    "no details",
			commit:  &Commit{},
			wantErr: true,
		},
		{
			name:    "no commit at all",
			commit:  nil,
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			when, err := testCase.commit.Time()
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Time: %v", err)
			}
			if got := when.UTC().Format(TimeLayout); got != testCase.want {
				t.Errorf("got %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestTreeSha(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		commit *Commit
		want   string
	}{
		{
			name:   "a commit naming its tree",
			commit: &Commit{Commit: &Details{Tree: &TreeReference{Sha: "treesha"}}},
			want:   "treesha",
		},
		{name: "no tree", commit: &Commit{Commit: &Details{}}},
		{name: "no details", commit: &Commit{}},
		{name: "no commit at all", commit: nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.commit.TreeSha(); got != testCase.want {
				t.Errorf("TreeSha() = %q, want %q", got, testCase.want)
			}
		})
	}
}
