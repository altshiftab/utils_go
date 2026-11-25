package netbindtest

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

// TestInspectCatchesBypasses checks the checker. A rule that has never been seen to fire is not
// evidence of anything, so each shape of bypass is put in front of it, along with the shapes that
// must stay legal or the rule would be unusable.
func TestInspectCatchesBypasses(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		source   string
		expected bool
	}{
		{name: "net.Dial", source: `package p; import "net"; func f() { net.Dial("tcp", "x") }`, expected: true},
		{name: "net.Listen", source: `package p; import "net"; func f() { net.Listen("tcp", "x") }`, expected: true},
		{name: "net.ListenPacket", source: `package p; import "net"; func f() { net.ListenPacket("udp", "x") }`, expected: true},
		{name: "net.Interfaces needs netlink", source: `package p; import "net"; func f() { net.Interfaces() }`, expected: true},
		{name: "net.LookupHost resolves unbound", source: `package p; import "net"; func f() { net.LookupHost("x") }`, expected: true},
		{name: "net.DefaultResolver", source: `package p; import "net"; func f() { net.DefaultResolver.LookupHost(nil, "x") }`, expected: true},
		{name: "an aliased import does not hide it", source: `package p; import n "net"; func f() { n.Dial("tcp", "x") }`, expected: true},
		{name: "http.Get", source: `package p; import "net/http"; func f() { http.Get("x") }`, expected: true},
		{name: "http.DefaultClient", source: `package p; import "net/http"; func f() { http.DefaultClient.Do(nil) }`, expected: true},
		{name: "constructing an http.Client", source: `package p; import "net/http"; var c = http.Client{}`, expected: true},
		{name: "constructing a net.Dialer", source: `package p; import "net"; var d = net.Dialer{}`, expected: true},
		{name: "constructing a net.Resolver", source: `package p; import "net"; var r = &net.Resolver{}`, expected: true},
		{name: "an http.Client by value is usable at its zero value", source: `package p; import "net/http"; type s struct { c http.Client }`, expected: true},
		{name: "http.ListenAndServe", source: `package p; import "net/http"; func f() { http.ListenAndServe(":80", nil) }`, expected: true},
		{name: "http.ListenAndServeTLS", source: `package p; import "net/http"; func f() { http.ListenAndServeTLS(":443", "c", "k", nil) }`, expected: true},
		{name: "a server's ListenAndServe", source: `package p; import "net/http"; func f(s *http.Server) { s.ListenAndServe() }`, expected: true},
		{name: "a server's ListenAndServeTLS", source: `package p; import "net/http"; func f(s *http.Server) { s.ListenAndServeTLS("c", "k") }`, expected: true},
		{
			name:     "the service package's Serve",
			source:   `package p; import "github.com/altshiftab/utils_go/pkg/http/service"; func f(s *service.Service) { s.Serve() }`,
			expected: true,
		},
		{
			name:     "the service package's ServeContext",
			source:   `package p; import "github.com/altshiftab/utils_go/pkg/http/service"; func f(s *service.Service) { s.ServeContext(nil) }`,
			expected: true,
		},
		{
			name:     "the service package at its old path",
			source:   `package p; import "github.com/Motmedel/utils_go/pkg/http/service"; func f(s *service.Service) { s.Serve() }`,
			expected: true,
		},
		{
			name:     "the service package serving a listener",
			source:   `package p; import "github.com/altshiftab/utils_go/pkg/http/service"; func f(s *service.Service) { s.ServeListener(nil, nil) }`,
			expected: false,
		},
		{name: "a server serving a listener", source: `package p; import "net/http"; func f(s *http.Server) { s.Serve(nil) }`, expected: false},
		{name: "http.Serve on a listener", source: `package p; import "net/http"; func f() { http.Serve(nil, nil) }`, expected: false},
		{name: "an http.Server value", source: `package p; import "net/http"; var s = &http.Server{}`, expected: false},
		{name: "holding a *http.Client is a handle, not a socket", source: `package p; import "net/http"; type s struct { c *http.Client }`, expected: false},
		{name: "a *net.Dialer field", source: `package p; import "net"; type s struct { d *net.Dialer }`, expected: false},
		{name: "returning a *http.Client", source: `package p; import "net/http"; func f() *http.Client { return nil }`, expected: false},
		{name: "harmless net helpers", source: `package p; import "net"; func f() { net.JoinHostPort("a", "b"); net.ParseIP("x") }`, expected: false},
		{name: "net.Pipe makes no socket", source: `package p; import "net"; func f() { net.Pipe() }`, expected: false},
		{name: "a local variable named net is not the package", source: `package p; func f() { var net struct{ Dial func() }; net.Dial() }`, expected: false},
		{name: "http types used only for serving", source: `package p; import "net/http"; func f(w http.ResponseWriter, r *http.Request) {}`, expected: false},
		{name: "a Serve method in a file with no http import", source: `package p; type t struct{}; func (t) Serve() {}; func f() { t{}.Serve() }`, expected: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fileSet := token.NewFileSet()

			file, err := parser.ParseFile(fileSet, "synthetic.go", testCase.source, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("%s: parse file: %v", testCase.name, err)
			}

			violations := inspect(fileSet, file, "synthetic.go")

			if got := len(violations) != 0; got != testCase.expected {
				t.Errorf("%s: flagged=%t, want %t (violations: %v)", testCase.name, got, testCase.expected, violations)
			}
		})
	}
}

func TestFindWalksTheTree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	files := map[string]string{
		"internal/peer/peer.go":      `package peer; import "net"; func f() { net.Dial("tcp", "x") }`,
		"internal/peer/peer_test.go": `package peer; import "net"; func g() { net.Listen("tcp", "x") }`,
		"internal/api/api.go":        `package api; import "net"; func f() { net.Listen("tcp", "127.0.0.1:0") }`,
		"internal/clean/clean.go":    `package clean; func f() {}`,
		"cmd/tool/main.go":           `package main; import "net/http"; func main() { http.Get("x") }`,
		"vendored/other/other.go":    `package other; import "net"; func f() { net.Dial("tcp", "x") }`,
	}

	for path, source := range files {
		full := filepath.Join(root, path)

		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatalf("mkdir all: %v", err)
		}

		if err := os.WriteFile(full, []byte(source), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	testCases := []struct {
		name        string
		directories []string
		allowed     []string
		expected    []string
	}{
		{
			name:        "internal and cmd",
			directories: []string{"internal", "cmd"},
			expected:    []string{"internal/api/api.go:1", "internal/peer/peer.go:1", "cmd/tool/main.go:1"},
		},
		{
			name:        "an allowed file",
			directories: []string{"internal", "cmd"},
			allowed:     []string{"internal/api/api.go"},
			expected:    []string{"internal/peer/peer.go:1", "cmd/tool/main.go:1"},
		},
		{
			name:        "an allowed directory",
			directories: []string{"internal"},
			allowed:     []string{"internal/peer", "internal/api"},
			expected:    nil,
		},
		{
			name:        "a directory that does not exist",
			directories: []string{"pkg"},
			expected:    nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			violations, err := Find(root, testCase.directories, testCase.allowed...)
			if err != nil {
				t.Fatalf("%s: find: %v", testCase.name, err)
			}

			got := make(map[string]bool)
			for _, violation := range violations {
				got[violation.Position] = true
			}

			if len(got) != len(testCase.expected) {
				t.Errorf("%s: got %v, want %v", testCase.name, violations, testCase.expected)
			}

			for _, position := range testCase.expected {
				if !got[filepath.FromSlash(position)] {
					t.Errorf("%s: missing %s in %v", testCase.name, position, violations)
				}
			}
		})
	}
}
