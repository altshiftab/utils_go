package github

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// graphqlServer answers LastCommitTimes' queries from dates keyed by path. A path absent from dates
// is answered as a repository that is not there, with an error beside it, as GitHub answers one.
type graphqlServer struct {
	t     *testing.T
	dates map[string]string

	mu       sync.Mutex
	requests []*graphqlRequest
}

func (server *graphqlServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != GraphqlPath {
		server.t.Errorf("request = %s %s", r.Method, r.URL.Path)
	}

	var request graphqlRequest
	if err := json.UnmarshalRead(r.Body, &request); err != nil {
		server.t.Errorf("unmarshal: %v", err)
	}

	server.mu.Lock()
	server.requests = append(server.requests, &request)
	server.mu.Unlock()

	data := make(map[string]any)
	var errs []map[string]any
	for index := 0; ; index++ {
		path, ok := request.Variables["p"+strconv.Itoa(index)]
		if !ok {
			break
		}

		date, ok := server.dates[path]
		if !ok {
			data[fileAlias(index)] = nil
			errs = append(errs, map[string]any{"type": "NOT_FOUND", "message": "Could not resolve to a Repository"})
			continue
		}

		data[fileAlias(index)] = map[string]any{
			"object": map[string]any{"history": map[string]any{"nodes": []any{map[string]any{"committedDate": date}}}},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.MarshalWrite(w, map[string]any{"data": data, "errors": errs}); err != nil {
		server.t.Errorf("marshal: %v", err)
	}
}

func TestLastCommitTimes(t *testing.T) {
	t.Parallel()

	many := make([]*File, 0, 2*LastCommitBatchSize+1)
	manyDates := make(map[string]string, cap(many))
	for index := range cap(many) {
		path := fmt.Sprintf("file%d.yaml", index)
		many = append(many, &File{Owner: "owner", Repo: "repo", Path: path})
		manyDates[path] = "2026-10-07T12:00:00Z"
	}

	testCases := []struct {
		name         string
		files        []*File
		dates        map[string]string
		wantRequests int
		// wantAsked is how many files the requests asked about between them, where it matters.
		wantAsked int
		wantTimes map[string]string
	}{
		{
			name: "each file dated by its last commit",
			files: []*File{
				{Owner: "acme", Repo: "infra", Path: "deploy/config.yaml"},
				{Owner: "someone", Repo: "dotfiles", Path: ".env"},
			},
			dates:        map[string]string{"deploy/config.yaml": "2026-10-07T12:34:56Z", ".env": "2019-02-01T08:00:00Z"},
			wantRequests: 1,
			wantTimes:    map[string]string{"deploy/config.yaml": "2026-10-07T12:34:56Z", ".env": "2019-02-01T08:00:00Z"},
		},
		{
			// A repository deleted since the search found it is left out, not a failure of the rest.
			name: "a file the API could not answer for is absent",
			files: []*File{
				{Owner: "acme", Repo: "infra", Path: "deploy/config.yaml"},
				{Owner: "gone", Repo: "repo", Path: "missing.txt"},
			},
			dates:        map[string]string{"deploy/config.yaml": "2026-10-07T12:34:56Z"},
			wantRequests: 1,
			wantTimes:    map[string]string{"deploy/config.yaml": "2026-10-07T12:34:56Z"},
		},
		{
			name: "the same file twice is asked about once",
			files: []*File{
				{Owner: "acme", Repo: "infra", Path: "a.txt"},
				{Owner: "acme", Repo: "infra", Path: "a.txt"},
			},
			dates:        map[string]string{"a.txt": "2026-10-07T12:34:56Z"},
			wantRequests: 1,
			wantAsked:    1,
			wantTimes:    map[string]string{"a.txt": "2026-10-07T12:34:56Z"},
		},
		{
			name:         "more files than a batch take a request per batch",
			files:        many,
			dates:        manyDates,
			wantRequests: 3,
			wantAsked:    2*LastCommitBatchSize + 1,
		},
		{
			name:         "nothing to ask about asks nothing",
			files:        nil,
			wantRequests: 0,
			wantTimes:    map[string]string{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := &graphqlServer{t: t, dates: testCase.dates}
			client := clientFor(t, server.handle)

			times, err := client.LastCommitTimes(t.Context(), testCase.files)
			if err != nil {
				t.Fatalf("LastCommitTimes: %v", err)
			}

			if len(server.requests) != testCase.wantRequests {
				t.Errorf("requests = %d, want %d", len(server.requests), testCase.wantRequests)
			}

			if testCase.wantAsked != 0 {
				asked := 0
				for _, request := range server.requests {
					asked += len(request.Variables) / 3
				}
				if asked != testCase.wantAsked {
					t.Errorf("asked about %d files, want %d", asked, testCase.wantAsked)
				}
			}

			if testCase.wantTimes == nil {
				if len(times) != len(testCase.files) {
					t.Errorf("dated %d files, want %d", len(times), len(testCase.files))
				}
				return
			}

			got := make(map[string]string, len(times))
			for file, at := range times {
				got[file.Path] = at.UTC().Format(time.RFC3339)
			}
			if len(got) != len(testCase.wantTimes) {
				t.Errorf("times = %v, want %v", got, testCase.wantTimes)
			}
			for path, want := range testCase.wantTimes {
				if got[path] != want {
					t.Errorf("%s: %q, want %q", path, got[path], want)
				}
			}
		})
	}
}

// TestLastCommitTimesPassesNamesAsVariables holds that a name or a path is never spliced into the
// query's text, where a quote in a path would end the string and the rest be read as GraphQL.
func TestLastCommitTimesPassesNamesAsVariables(t *testing.T) {
	t.Parallel()

	path := `docs/"quoted") { x } #.md`
	server := &graphqlServer{t: t, dates: map[string]string{path: "2026-10-07T12:34:56Z"}}
	client := clientFor(t, server.handle)

	times, err := client.LastCommitTimes(t.Context(), []*File{{Owner: "acme", Repo: "infra", Path: path}})
	if err != nil {
		t.Fatalf("LastCommitTimes: %v", err)
	}
	if len(times) != 1 {
		t.Errorf("times = %v", times)
	}

	request := server.requests[0]
	if strings.Contains(request.Query, "quoted") || strings.Contains(request.Query, "acme") {
		t.Errorf("query carries a name: %s", request.Query)
	}
	if request.Variables["p0"] != path || request.Variables["o0"] != "acme" || request.Variables["n0"] != "infra" {
		t.Errorf("variables = %v", request.Variables)
	}
}

func TestLastCommitTimesRefused(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		body string
	}{
		{name: "errors and no data", body: `{"errors": [{"type": "RATE_LIMITED", "message": "API rate limit exceeded"}]}`},
		{name: "neither data nor errors", body: `{}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(testCase.body)); err != nil {
					t.Errorf("write: %v", err)
				}
			})

			_, err := client.LastCommitTimes(t.Context(), []*File{{Owner: "acme", Repo: "infra", Path: "a.txt"}})
			if err == nil {
				t.Fatal("expected an error")
			}
			if testCase.name == "errors and no data" && !errors.Is(err, ErrGraphql) {
				t.Errorf("err = %v, want ErrGraphql", err)
			}
		})
	}
}

func TestLastCommitTimesRejectsIncompleteFiles(t *testing.T) {
	t.Parallel()

	client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request should be made, got %s", r.URL.Path)
	})

	testCases := []struct {
		name string
		file *File
	}{
		{name: "nil", file: nil},
		{name: "no owner", file: &File{Repo: "infra", Path: "a.txt"}},
		{name: "no repository", file: &File{Owner: "acme", Path: "a.txt"}},
		{name: "no path", file: &File{Owner: "acme", Repo: "infra"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := client.LastCommitTimes(t.Context(), []*File{testCase.file}); err == nil {
				t.Error("expected an error")
			}
		})
	}
}
