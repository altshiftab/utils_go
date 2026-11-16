// Package file reads a PDF file through its cross-reference tables and appends incremental updates to it.
//
// It reads what Chrome writes and what its own updates add: classic cross-reference tables, chained by /Prev. Cross-reference streams, compressed object streams and encryption are refused rather than half-read.
package file

import (
	"bytes"
	"fmt"
	"strconv"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/pdf/object"
)

// ErrUnsupported is a valid PDF feature this package does not read; it is a validation error, so an endpoint answers it as input it cannot take.
var ErrUnsupported = fmt.Errorf("unsupported pdf feature: %w", altshiftErrors.ErrValidationError)

const (
	startXrefKeyword = "startxref"
	// maxSections bounds the /Prev chain, so a loop cannot keep the reader going.
	maxSections = 1024
	// maxResolveSteps bounds a chain of references to references.
	maxResolveSteps = 32
	// maxPageTreeDepth bounds the page tree's nesting.
	maxPageTreeDepth = 64
	// MaxObjectNumber is the largest object number a PDF may use (ISO 32000-1, Annex C).
	MaxObjectNumber = 8388607
)

type entry struct {
	offset     int
	generation int
	free       bool
}

// File is a parsed PDF file as it stands after its last update; it caches what it reads, so it is not safe for concurrent use.
type File struct {
	Data    []byte
	Trailer object.Dictionary

	entries  map[int]*entry
	cache    map[int]object.Object
	reading  map[int]bool
	lastXref int
	size     int
}

func syntaxError(format string, arguments ...any) error {
	return altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s", object.ErrSyntax, fmt.Sprintf(format, arguments...)))
}

func unsupported(format string, arguments ...any) error {
	return altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s", ErrUnsupported, fmt.Sprintf(format, arguments...)))
}

func lastStartXref(data []byte) (int, error) {
	index := bytes.LastIndex(data, []byte(startXrefKeyword))
	if index < 0 {
		return 0, syntaxError("no startxref")
	}
	fields := bytes.Fields(data[index+len(startXrefKeyword):])
	if len(fields) == 0 {
		return 0, syntaxError("startxref without an offset")
	}
	offset, err := strconv.Atoi(string(fields[0]))
	if err != nil || offset < 0 || offset >= len(data) {
		return 0, syntaxError("startxref offset %q", fields[0])
	}
	return offset, nil
}

// tokens splits the start of data into whitespace-separated fields, stopping at "trailer".
type xrefReader struct {
	data     []byte
	position int
}

func (r *xrefReader) next() []byte {
	for r.position < len(r.data) && (r.data[r.position] == ' ' || r.data[r.position] == '\r' || r.data[r.position] == '\n' || r.data[r.position] == '\t') {
		r.position++
	}
	start := r.position
	for r.position < len(r.data) && r.data[r.position] != ' ' && r.data[r.position] != '\r' && r.data[r.position] != '\n' && r.data[r.position] != '\t' {
		r.position++
	}
	return r.data[start:r.position]
}

func (r *xrefReader) integer() (int, error) {
	token := r.next()
	value, err := strconv.Atoi(string(token))
	if err != nil || value < 0 {
		return 0, syntaxError("cross-reference table: %q is not a number", token)
	}
	return value, nil
}

// readSection reads the classic table at offset and the trailer after it.
func readSection(data []byte, offset int) (map[int]*entry, object.Dictionary, error) {
	if !bytes.HasPrefix(data[offset:], []byte("xref")) {
		return nil, nil, unsupported("the cross-reference section at %d is not a classic table (a cross-reference stream?)", offset)
	}

	reader := &xrefReader{data: data, position: offset + len("xref")}
	entries := map[int]*entry{}
	for {
		save := reader.position
		if token := reader.next(); string(token) == "trailer" || bytes.HasPrefix(token, []byte("trailer")) {
			reader.position = save + bytes.Index(data[save:], []byte("trailer")) + len("trailer")
			break
		}
		reader.position = save

		first, err := reader.integer()
		if err != nil {
			return nil, nil, err
		}
		count, err := reader.integer()
		if err != nil {
			return nil, nil, err
		}
		if first > MaxObjectNumber || count > MaxObjectNumber+1-first {
			return nil, nil, syntaxError("cross-reference subsection %d %d runs past object number %d", first, count, MaxObjectNumber)
		}
		for number := first; number < first+count; number++ {
			entryOffset, err := reader.integer()
			if err != nil {
				return nil, nil, err
			}
			generation, err := reader.integer()
			if err != nil {
				return nil, nil, err
			}
			switch kind := string(reader.next()); kind {
			case "n":
				entries[number] = &entry{offset: entryOffset, generation: generation}
			case "f":
				entries[number] = &entry{generation: generation, free: true}
			default:
				return nil, nil, syntaxError("cross-reference entry %d has type %q", number, kind)
			}
		}
	}

	trailerObject, _, err := object.Parse(data[reader.position:])
	if err != nil {
		return nil, nil, fmt.Errorf("object parse (trailer): %w", err)
	}
	trailer, ok := trailerObject.(object.Dictionary)
	if !ok {
		return nil, nil, syntaxError("the trailer is not a dictionary")
	}
	return entries, trailer, nil
}

// Open reads the cross-reference chain from the last section back to the first, the newest entry for each object winning.
func Open(data []byte) (*File, error) {
	if len(data) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("pdf data"))
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, syntaxError("no %%PDF- header")
	}

	lastXref, err := lastStartXref(data)
	if err != nil {
		return nil, fmt.Errorf("last startxref: %w", err)
	}

	file := &File{Data: data, entries: map[int]*entry{}, cache: map[int]object.Object{}, reading: map[int]bool{}, lastXref: lastXref}
	visited := map[int]bool{}
	for offset, hasPrevious := lastXref, true; hasPrevious; {
		if visited[offset] || len(visited) == maxSections {
			return nil, syntaxError("the /Prev chain loops or is longer than %d", maxSections)
		}
		visited[offset] = true

		entries, trailer, err := readSection(data, offset)
		if err != nil {
			return nil, fmt.Errorf("read section at %d: %w", offset, err)
		}
		if file.Trailer == nil {
			file.Trailer = trailer
		}
		if _, ok := trailer["XRefStm"]; ok {
			return nil, unsupported("a hybrid file with a cross-reference stream")
		}
		if size, ok := trailer.Integer("Size"); ok {
			if size > MaxObjectNumber+1 {
				return nil, syntaxError("trailer /Size %d is past the largest object number %d", size, MaxObjectNumber)
			}
			file.size = max(file.size, int(size))
		}
		for number, value := range entries {
			if _, known := file.entries[number]; !known {
				file.entries[number] = value
			}
			file.size = max(file.size, number+1)
		}

		previous, ok := trailer.Integer("Prev")
		hasPrevious = ok
		offset = int(previous)
		if hasPrevious && (offset < 0 || offset >= len(data)) {
			return nil, syntaxError("/Prev %d is outside the file", offset)
		}
	}

	if _, ok := file.Trailer["Encrypt"]; ok {
		return nil, unsupported("an encrypted file")
	}
	if _, ok := file.Trailer.Reference("Root"); !ok {
		return nil, syntaxError("the trailer has no /Root reference")
	}
	return file, nil
}

// Size is the first object number no revision has used.
func (f *File) Size() int {
	return f.size
}

func (f *File) resolveLength(length object.Object) (int, error) {
	resolved, err := f.Resolve(length)
	if err != nil {
		return 0, fmt.Errorf("resolve: %w", err)
	}
	integer, ok := resolved.(object.Integer)
	if !ok {
		return 0, syntaxError("stream /Length is a %T", resolved)
	}
	return int(integer), nil
}

// Object returns the indirect object with that number, or nil for one that is free or absent, as the syntax has a reference to it read.
func (f *File) Object(number int) (object.Object, error) {
	if cached, ok := f.cache[number]; ok {
		return cached, nil
	}
	value, ok := f.entries[number]
	if !ok || value.free {
		return nil, nil
	}

	// A stream whose /Length leads back to an object still being read would recurse without end.
	if f.reading[number] {
		return nil, syntaxError("object %d is needed to read itself", number)
	}
	f.reading[number] = true
	defer delete(f.reading, number)

	indirect, err := object.ParseIndirect(f.Data, value.offset, f.resolveLength)
	if err != nil {
		return nil, fmt.Errorf("object parse indirect (%d): %w", number, err)
	}
	if indirect.Number != number || indirect.Generation != value.generation {
		return nil, syntaxError("the entry for %d %d points at %d %d", number, value.generation, indirect.Number, indirect.Generation)
	}
	f.cache[number] = indirect.Value
	return indirect.Value, nil
}

// Generation is the generation the object with that number has in the newest revision.
func (f *File) Generation(number int) int {
	if value, ok := f.entries[number]; ok {
		return value.generation
	}
	return 0
}

// Resolve follows references until it reaches a direct object; a reference whose generation is not the object's reads as null.
func (f *File) Resolve(value object.Object) (object.Object, error) {
	for range maxResolveSteps {
		reference, ok := value.(*object.Reference)
		if !ok || reference == nil {
			return value, nil
		}
		if f.Generation(reference.Number) != reference.Generation {
			return nil, nil
		}
		resolved, err := f.Object(reference.Number)
		if err != nil {
			return nil, err
		}
		value = resolved
	}
	return nil, syntaxError("more than %d references in a chain", maxResolveSteps)
}

// ResolveDictionary resolves to a dictionary, or a stream's dictionary.
func (f *File) ResolveDictionary(value object.Object) (object.Dictionary, error) {
	resolved, err := f.Resolve(value)
	if err != nil {
		return nil, err
	}
	switch typed := resolved.(type) {
	case object.Dictionary:
		return typed, nil
	case *object.Stream:
		if typed != nil {
			return typed.Dictionary, nil
		}
	}
	return nil, syntaxError("%T is not a dictionary", resolved)
}

// ResolveArray resolves to an array.
func (f *File) ResolveArray(value object.Object) (object.Array, error) {
	resolved, err := f.Resolve(value)
	if err != nil {
		return nil, err
	}
	array, ok := resolved.(object.Array)
	if !ok {
		return nil, syntaxError("%T is not an array", resolved)
	}
	return array, nil
}

// Catalog returns the document catalog and its reference.
func (f *File) Catalog() (object.Dictionary, *object.Reference, error) {
	reference, ok := f.Trailer.Reference("Root")
	if !ok {
		return nil, nil, syntaxError("the trailer has no /Root reference")
	}
	catalog, err := f.ResolveDictionary(reference)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve dictionary (catalog): %w", err)
	}
	return catalog, reference, nil
}

// inheritable are the page attributes a page takes from its ancestors when it has none of its own.
var inheritable = []object.Name{"Resources", "MediaBox", "CropBox", "Rotate"}

// Page is a leaf of the page tree; Inherited holds the attributes it takes from its ancestors.
type Page struct {
	Reference  *object.Reference
	Dictionary object.Dictionary
	Inherited  object.Dictionary
}

// PageTree is the document's pages in order, and the references of the tree's inner nodes.
type PageTree struct {
	Root      *object.Reference
	Pages     []*Page
	InnerNode []*object.Reference
}

func (f *File) walkPages(tree *PageTree, reference *object.Reference, inherited object.Dictionary, visited map[int]bool, depth int) error {
	if depth > maxPageTreeDepth {
		return syntaxError("the page tree is deeper than %d", maxPageTreeDepth)
	}
	if reference == nil {
		return syntaxError("a page tree node is not a reference")
	}
	if visited[reference.Number] {
		return syntaxError("the page tree visits %d twice", reference.Number)
	}
	visited[reference.Number] = true

	node, err := f.ResolveDictionary(reference)
	if err != nil {
		return fmt.Errorf("resolve dictionary (page tree node %d): %w", reference.Number, err)
	}

	if kind, _ := node.Name("Type"); kind == "Page" || node["Kids"] == nil {
		page := &Page{Reference: reference, Dictionary: node, Inherited: object.Dictionary{}}
		for _, key := range inheritable {
			if _, own := node[key]; !own {
				if value, ok := inherited[key]; ok {
					page.Inherited[key] = value
				}
			}
		}
		tree.Pages = append(tree.Pages, page)
		return nil
	}

	tree.InnerNode = append(tree.InnerNode, reference)
	passed := inherited.Clone()
	for _, key := range inheritable {
		if value, ok := node[key]; ok {
			passed[key] = value
		}
	}
	kids, err := f.ResolveArray(node["Kids"])
	if err != nil {
		return fmt.Errorf("resolve array (kids of %d): %w", reference.Number, err)
	}
	for _, kid := range kids {
		kidReference, _ := kid.(*object.Reference)
		if err := f.walkPages(tree, kidReference, passed, visited, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// PageTree walks the page tree from the catalog's /Pages.
func (f *File) PageTree() (*PageTree, error) {
	catalog, _, err := f.Catalog()
	if err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	root, ok := catalog.Reference("Pages")
	if !ok {
		return nil, syntaxError("the catalog has no /Pages reference")
	}
	tree := &PageTree{Root: root}
	if err := f.walkPages(tree, root, object.Dictionary{}, map[int]bool{}, 0); err != nil {
		return nil, err
	}
	return tree, nil
}
