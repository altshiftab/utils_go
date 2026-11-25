// Package netbindtest gives a program that sends through netbind the test that keeps it that way:
// it fails when any other package of the program opens a socket of its own.
//
// Binding the right source address is the only thing keeping such a program's traffic inside its
// tunnel, so the risk is not that the binding is written wrongly; it is that a later edit, in a
// package with nothing to do with networking, quietly opens a socket that never passes through
// netbind at all. The check is syntactic and errs on the side of reporting: a method named like one
// that listens, in a file that imports an HTTP package, is reported whatever its receiver.
package netbindtest

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	netPath     = "net"
	httpPath    = "net/http"
	servicePath = "pkg/http/service"
)

// forbiddenCalls names package-level functions that create a socket, resolve a name unbound, or ask
// the kernel about interfaces. The interface calls are here for a second reason: they need
// AF_NETLINK, and keeping them out is what lets a systemd unit deny that address family outright.
var forbiddenCalls = map[string]map[string]struct{}{
	netPath: {
		"Dial": {}, "DialTimeout": {}, "DialIP": {}, "DialTCP": {}, "DialUDP": {}, "DialUnix": {},
		"Listen": {}, "ListenIP": {}, "ListenMulticastUDP": {}, "ListenPacket": {}, "ListenTCP": {},
		"ListenUDP": {}, "ListenUnix": {}, "ListenUnixgram": {},
		"FileConn": {}, "FileListener": {}, "FilePacketConn": {},
		"Interfaces": {}, "InterfaceAddrs": {}, "InterfaceByIndex": {}, "InterfaceByName": {},
		"LookupHost": {}, "LookupIP": {}, "LookupIPAddr": {}, "LookupPort": {}, "LookupCNAME": {},
		"LookupAddr": {}, "LookupMX": {}, "LookupNS": {}, "LookupSRV": {}, "LookupTXT": {},
	},
	httpPath: {
		"Get": {}, "Head": {}, "Post": {}, "PostForm": {},
		"ListenAndServe": {}, "ListenAndServeTLS": {},
	},
}

// forbiddenTypes names types whose zero value is already a working, unbound client. Constructing
// one, by a composite literal or a variable of the bare type, is what is refused; a pointer to one
// that netbind handed over is not. http.Server is not here: serving a listener netbind made is the
// right way to serve, and the methods that listen by themselves are in forbiddenMethods.
var forbiddenTypes = map[string]map[string]struct{}{
	netPath:  {"Dialer": {}, "ListenConfig": {}, "Resolver": {}},
	httpPath: {"Client": {}, "Transport": {}},
}

// forbiddenVars names package-level values that are unbound clients ready to use.
var forbiddenVars = map[string]map[string]struct{}{
	netPath:  {"DefaultResolver": {}},
	httpPath: {"DefaultClient": {}, "DefaultTransport": {}},
}

// forbiddenMethods names methods that open a listener of their own, by the number of arguments
// they take, so that http.Server.Serve(listener) stays legal where the service package's
// argument-less Serve does not. A file is checked for them when it imports net/http or a service
// package.
var forbiddenMethods = map[string]int{
	"ListenAndServe":    0,
	"ListenAndServeTLS": 2,
	"Serve":             0,
	"ServeContext":      1,
}

// Violation is one reference to a way of opening a socket outside netbind.
type Violation struct {
	Position string
	Name     string
}

func (violation *Violation) String() string {
	return violation.Position + ": " + violation.Name
}

// Find returns every violation in the non-test Go files under the given directories of root. A
// directory that does not exist is skipped. allowed names files or directories, relative to root,
// that may open sockets of their own; it is a list rather than a comment convention, so that an
// exemption is something a reviewer reads in one place.
func Find(root string, directories []string, allowed ...string) ([]*Violation, error) {
	fileSet := token.NewFileSet()

	var violations []*Violation

	for _, directory := range directories {
		base := filepath.Join(root, directory)

		err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				if path == base && errors.Is(err, fs.ErrNotExist) {
					return filepath.SkipDir
				}

				return err
			}

			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			relative, err := filepath.Rel(root, path)
			if err != nil {
				return fmt.Errorf("filepath rel: %w", err)
			}

			if isAllowed(relative, allowed) {
				return nil
			}

			file, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return fmt.Errorf("parser parse file: %w", err)
			}

			violations = append(violations, inspect(fileSet, file, relative)...)

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("filepath walk dir %s: %w", base, err)
		}
	}

	return violations, nil
}

// Check fails t for every violation Find reports. Tests are exempt from the rule: they stand up
// loopback listeners to exercise the very code it protects, and they do not run in production.
func Check(t testing.TB, root string, directories []string, allowed ...string) {
	t.Helper()

	violations, err := Find(root, directories, allowed...)
	if err != nil {
		t.Fatalf("find: %v", err)
	}

	for _, violation := range violations {
		t.Errorf("%s -- sockets belong in netbind", violation)
	}
}

func isAllowed(relative string, allowed []string) bool {
	for _, entry := range allowed {
		entry = filepath.Clean(entry)
		if relative == entry || strings.HasPrefix(relative, entry+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

// inspect finds the forbidden references in one file.
func inspect(fileSet *token.FileSet, file *ast.File, relative string) []*Violation {
	// Resolve the local name of each watched package, so that a variable called "net" is not taken
	// for the package and an aliased import is not missed.
	packages := make(map[string]string)
	checkMethods := false

	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}

		switch {
		case path == netPath || path == httpPath:
			checkMethods = checkMethods || path == httpPath
		case strings.HasSuffix(path, "/"+servicePath):
			checkMethods = true

			continue
		default:
			continue
		}

		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}

		packages[name] = path
	}

	if len(packages) == 0 && !checkMethods {
		return nil
	}

	var violations []*Violation

	report := func(node ast.Node, name string, why string) {
		position := fileSet.Position(node.Pos())
		violations = append(violations, &Violation{
			Position: relative + ":" + strconv.Itoa(position.Line),
			Name:     name + " " + why,
		})
	}

	// match reports the qualified name when a selector names something in the given set.
	match := func(expression ast.Expr, sets map[string]map[string]struct{}) (string, bool) {
		selector, ok := expression.(*ast.SelectorExpr)
		if !ok {
			return "", false
		}

		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return "", false
		}

		path, ok := packages[identifier.Name]
		if !ok {
			return "", false
		}

		if _, ok := sets[path][selector.Sel.Name]; !ok {
			return "", false
		}

		return identifier.Name + "." + selector.Sel.Name, true
	}

	// A bare type behind a pointer is a handle somebody else made; only the pointer form is allowed,
	// so the selectors under a StarExpr are collected and skipped below.
	behindPointer := make(map[ast.Expr]struct{})

	ast.Inspect(file, func(node ast.Node) bool {
		if star, ok := node.(*ast.StarExpr); ok {
			behindPointer[star.X] = struct{}{}
		}

		return true
	})

	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.CallExpr:
			if name, ok := match(typed.Fun, forbiddenCalls); ok {
				report(typed, name, "opens an unbound socket")

				return true
			}

			if !checkMethods {
				return true
			}

			selector, ok := typed.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			if _, isPackage := packages[identifierName(selector.X)]; isPackage {
				return true
			}

			if arguments, ok := forbiddenMethods[selector.Sel.Name]; ok && arguments == len(typed.Args) {
				report(typed, "."+selector.Sel.Name, "listens on an address of its own")
			}
		case *ast.CompositeLit:
			if name, ok := match(typed.Type, forbiddenTypes); ok {
				report(typed, name, "constructs an unbound client")
			}
		case *ast.SelectorExpr:
			if name, ok := match(typed, forbiddenVars); ok {
				report(typed, name, "is an unbound client")

				return true
			}

			// A value of one of these types is usable at its zero value, so declaring one is as good
			// as constructing it, unless it is a pointer, which is only a reference.
			if _, pointer := behindPointer[ast.Expr(typed)]; pointer {
				return true
			}

			if name, ok := match(typed, forbiddenTypes); ok {
				report(typed, name, "declares an unbound client by value")
			}
		}

		return true
	})

	return violations
}

func identifierName(expression ast.Expr) string {
	if identifier, ok := expression.(*ast.Ident); ok {
		return identifier.Name
	}

	return ""
}
