package github

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/github/github_config"
)

// clientFor returns a client pointed at a server the test controls.
func clientFor(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	serverUrl, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	return NewClient(
		github_config.WithBaseUrl(serverUrl),
		github_config.WithArchiveBaseUrl(serverUrl),
	)
}

func TestLatestCommit(t *testing.T) {
	t.Parallel()

	t.Run("the commit that last touched the path is returned", func(t *testing.T) {
		t.Parallel()

		client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/repos/nmap/nmap/commits" {
				t.Errorf("path = %q", r.URL.Path)
			}
			// The query is what limits the answer to the one file; without it
			// the head of the branch would come back instead.
			if got := r.URL.Query().Get("path"); got != "nmap-service-probes" {
				t.Errorf("path query = %q", got)
			}
			if got := r.URL.Query().Get("per_page"); got != "1" {
				t.Errorf("per_page query = %q", got)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{
				"sha": "919ddb44cb6ef42dbe441724b86ba8aacdff6daf",
				"commit": {
					"committer": {"date": "2026-04-12T09:31:02Z"},
					"tree": {"sha": "treesha123"}
				}
			}]`))
		})

		latest, err := client.LatestCommit(t.Context(), "nmap", "nmap", "nmap-service-probes")
		if err != nil {
			t.Fatalf("LatestCommit: %v", err)
		}

		if latest.Sha != "919ddb44cb6ef42dbe441724b86ba8aacdff6daf" {
			t.Errorf("Sha = %q", latest.Sha)
		}
		if latest.TreeSha() != "treesha123" {
			t.Errorf("TreeSha() = %q", latest.TreeSha())
		}

		when, err := latest.Time()
		if err != nil {
			t.Fatalf("Time: %v", err)
		}
		if got := when.UTC().Format("2006-01-02T15:04:05Z"); got != "2026-04-12T09:31:02Z" {
			t.Errorf("Time() = %q", got)
		}
	})

	t.Run("an answer naming several commits is an error", func(t *testing.T) {
		t.Parallel()

		client := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"sha":"a"},{"sha":"b"}]`))
		})

		_, err := client.LatestCommit(t.Context(), "nmap", "nmap", "file")
		if !errors.Is(err, ErrUnexpectedCommitCount) {
			t.Errorf("error = %v, want %v", err, ErrUnexpectedCommitCount)
		}
	})

	t.Run("an answer naming none is an error", func(t *testing.T) {
		t.Parallel()

		client := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		})

		if _, err := client.LatestCommit(t.Context(), "nmap", "nmap", "file"); !errors.Is(err, ErrUnexpectedCommitCount) {
			t.Errorf("error = %v, want %v", err, ErrUnexpectedCommitCount)
		}
	})
}

func TestTree(t *testing.T) {
	t.Parallel()

	t.Run("the listing is returned and searchable", func(t *testing.T) {
		t.Parallel()

		client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/repos/nmap/nmap/git/trees/treesha" {
				t.Errorf("path = %q", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sha":"treesha","tree":[
				{"path":"README.md","sha":"aaa","type":"blob"},
				{"path":"nmap-service-probes","sha":"bbb","type":"blob"}
			]}`))
		})

		listing, err := client.Tree(t.Context(), "nmap", "nmap", "treesha")
		if err != nil {
			t.Fatalf("Tree: %v", err)
		}

		entry, ok := listing.Find("nmap-service-probes")
		if !ok {
			t.Fatal("the entry was not found")
		}
		if entry.Sha != "bbb" {
			t.Errorf("Sha = %q", entry.Sha)
		}

		if _, ok := listing.Find("missing"); ok {
			t.Error("an entry that is not there was found")
		}
	})

	// A caller searching a truncated listing and finding nothing has not
	// learned that the path is absent, so the flag has to survive.
	t.Run("a truncated listing says so", func(t *testing.T) {
		t.Parallel()

		client := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tree":[],"truncated":true}`))
		})

		listing, err := client.Tree(t.Context(), "nmap", "nmap", "treesha")
		if err != nil {
			t.Fatalf("Tree: %v", err)
		}
		if !listing.Truncated {
			t.Error("Truncated = false, want true")
		}
	})

	// An answer that parsed into nothing must be a failure to report, not an
	// empty listing to hand back.
	t.Run("an answer that parses to nothing is an error", func(t *testing.T) {
		t.Parallel()

		client := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`null`))
		})

		if _, err := client.Tree(t.Context(), "nmap", "nmap", "treesha"); err == nil {
			t.Error("expected an error, got nil")
		}
	})
}

func TestBlob(t *testing.T) {
	t.Parallel()

	const contents = "Probe TCP NULL q||\nmatch http m|^HTTP/1\\.[01]|\n"

	t.Run("the contents are decoded", func(t *testing.T) {
		t.Parallel()

		client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/repos/nmap/nmap/git/blobs/blobsha" {
				t.Errorf("path = %q", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(
				`{"encoding":"base64","content":"` +
					base64.StdEncoding.EncodeToString([]byte(contents)) + `"}`,
			))
		})

		value, err := client.Blob(t.Context(), "nmap", "nmap", "blobsha")
		if err != nil {
			t.Fatalf("Blob: %v", err)
		}

		data, err := value.Data()
		if err != nil {
			t.Fatalf("Data: %v", err)
		}
		if string(data) != contents {
			t.Errorf("got %q, want %q", data, contents)
		}
	})

	// The API wraps encoded contents across lines, which the decoder rejects
	// unless they are taken out first.
	t.Run("wrapped contents are decoded", func(t *testing.T) {
		t.Parallel()

		encoded := base64.StdEncoding.EncodeToString([]byte(contents))
		var wrapped strings.Builder
		for index := 0; index < len(encoded); index += 4 {
			wrapped.WriteString(encoded[index:min(index+4, len(encoded))])
			wrapped.WriteString("\\n")
		}

		client := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"encoding":"base64","content":"` + wrapped.String() + `"}`))
		})

		value, err := client.Blob(t.Context(), "nmap", "nmap", "blobsha")
		if err != nil {
			t.Fatalf("Blob: %v", err)
		}

		data, err := value.Data()
		if err != nil {
			t.Fatalf("Data: %v", err)
		}
		if string(data) != contents {
			t.Errorf("got %q, want %q", data, contents)
		}
	})
}

func TestCommitArchive(t *testing.T) {
	t.Parallel()

	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	file, err := writer.Create("nmap-nmap-919ddb4/README.md")
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := file.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}

	t.Run("the archive is returned", func(t *testing.T) {
		t.Parallel()

		client := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/nmap/nmap/archive/919ddb4.zip" {
				t.Errorf("path = %q", r.URL.Path)
			}
			_, _ = w.Write(archive.Bytes())
		})

		reader, err := client.CommitArchive(t.Context(), "nmap", "nmap", "919ddb4")
		if err != nil {
			t.Fatalf("CommitArchive: %v", err)
		}
		if len(reader.File) != 1 {
			t.Errorf("got %d files, want 1", len(reader.File))
		}
	})

	t.Run("something that is not an archive is an error", func(t *testing.T) {
		t.Parallel()

		client := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not a zip"))
		})

		if _, err := client.CommitArchive(t.Context(), "nmap", "nmap", "919ddb4"); err == nil {
			t.Error("expected an error, got nil")
		}
	})
}

// The arguments name what is fetched, so an empty one would build an address
// that quietly asks for something else.
func TestClientRejectsMissingArguments(t *testing.T) {
	t.Parallel()

	client := NewClient()

	testCases := []struct {
		name string
		call func() error
	}{
		{
			name: "latest commit without an owner",
			call: func() error { _, err := client.LatestCommit(t.Context(), "", "repo", "path"); return err },
		},
		{
			name: "latest commit without a repo",
			call: func() error { _, err := client.LatestCommit(t.Context(), "owner", "", "path"); return err },
		},
		{
			name: "latest commit without a path",
			call: func() error { _, err := client.LatestCommit(t.Context(), "owner", "repo", ""); return err },
		},
		{
			name: "tree without a sha",
			call: func() error { _, err := client.Tree(t.Context(), "owner", "repo", ""); return err },
		},
		{
			name: "blob without a sha",
			call: func() error { _, err := client.Blob(t.Context(), "owner", "repo", ""); return err },
		},
		{
			name: "archive without a reference",
			call: func() error { _, err := client.CommitArchive(t.Context(), "owner", "repo", ""); return err },
		},
		{
			name: "no client at all",
			call: func() error {
				var nilClient *Client
				_, err := nilClient.LatestCommit(t.Context(), "owner", "repo", "path")
				return err
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if err := testCase.call(); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}

func TestNewClientDefaults(t *testing.T) {
	t.Parallel()

	client := NewClient()

	if client.baseUrl.Host != DefaultHost {
		t.Errorf("baseUrl host = %q, want %q", client.baseUrl.Host, DefaultHost)
	}
	if client.archiveBaseUrl.Host != DefaultArchiveHost {
		t.Errorf("archiveBaseUrl host = %q, want %q", client.archiveBaseUrl.Host, DefaultArchiveHost)
	}
	if client.baseUrl.Scheme != "https" {
		t.Errorf("baseUrl scheme = %q, want https", client.baseUrl.Scheme)
	}
}

// A token has to reach the request, or a private repository comes back as a
// missing one.
func TestWithTokenAuthenticatesRequests(t *testing.T) {
	t.Parallel()

	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tree":[]}`))
	}))
	defer server.Close()

	serverUrl, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	client := NewClient(
		github_config.WithBaseUrl(serverUrl),
		github_config.WithToken("secret-token"),
	)

	if _, err := client.Tree(t.Context(), "owner", "repo", "sha"); err != nil {
		t.Fatalf("Tree: %v", err)
	}

	if authorization != "Bearer secret-token" {
		t.Errorf("Authorization = %q", authorization)
	}
}
