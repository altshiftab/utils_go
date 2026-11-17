// Package attachments embeds files in a PDF as an incremental update: each goes in the catalog's embedded files and among the files associated with the document (/AF), where a reader lists it and a program finds it without reading the pages.
package attachments

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/pdf/file"
	"github.com/altshiftab/utils_go/pkg/pdf/object"
)

// maxNameTreeDepth bounds a name tree's nesting.
const maxNameTreeDepth = 32

// The relationships ISO 32000-2 names between a document and a file associated with it.
const (
	RelationshipSource      = "Source"
	RelationshipData        = "Data"
	RelationshipAlternative = "Alternative"
	RelationshipSupplement  = "Supplement"
	RelationshipUnspecified = "Unspecified"
)

var relationships = []string{RelationshipSource, RelationshipData, RelationshipAlternative, RelationshipSupplement, RelationshipUnspecified}

var (
	ErrDuplicateName = errors.New("an embedded file of that name is already in the document")
	ErrInvalidFile   = errors.New("invalid embedded file")
)

// File is a file to embed.
type File struct {
	Name         string
	Description  string
	MimeType     string
	Relationship string
	ModifiedAt   time.Time
	Data         []byte
}

func (f *File) validate() error {
	if f == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("file"))
	}
	if f.Name == "" || !utf8.ValidString(f.Name) {
		return altshiftErrors.NewWithTrace(fmt.Errorf("%w: name %q", ErrInvalidFile, f.Name))
	}
	if f.Relationship != "" && !slices.Contains(relationships, f.Relationship) {
		return altshiftErrors.NewWithTrace(fmt.Errorf("%w: relationship %q", ErrInvalidFile, f.Relationship))
	}
	return nil
}

func isAscii(text string) bool {
	for i := range len(text) {
		if text[i] >= 0x80 {
			return false
		}
	}
	return true
}

// textString is a PDF text string: as it is for ASCII, otherwise UTF-16BE behind its byte order mark.
func textString(text string) *object.String {
	if isAscii(text) {
		return &object.String{Value: []byte(text)}
	}
	value := []byte{0xfe, 0xff}
	for _, unit := range utf16.Encode([]rune(text)) {
		value = binary.BigEndian.AppendUint16(value, unit)
	}
	return &object.String{Value: value, Hex: true}
}

// readNameTree gathers a name tree's entries by their keys' bytes; a node reached twice is refused.
func readNameTree(document *file.File, node object.Object, entries map[string]object.Object, visited map[int]bool, depth int) error {
	if depth > maxNameTreeDepth {
		return altshiftErrors.NewWithTrace(fmt.Errorf("%w: a name tree deeper than %d", object.ErrSyntax, maxNameTreeDepth))
	}
	if node == nil {
		return nil
	}
	if reference, ok := node.(*object.Reference); ok && reference != nil {
		if visited[reference.Number] {
			return altshiftErrors.NewWithTrace(fmt.Errorf("%w: a name tree reaches %d twice", object.ErrSyntax, reference.Number))
		}
		visited[reference.Number] = true
	}
	dictionary, err := document.ResolveDictionary(node)
	if err != nil {
		return fmt.Errorf("resolve dictionary (name tree node): %w", err)
	}
	if names, ok := dictionary["Names"]; ok {
		array, err := document.ResolveArray(names)
		if err != nil {
			return fmt.Errorf("resolve array (names): %w", err)
		}
		if len(array)%2 != 0 {
			return altshiftErrors.NewWithTrace(fmt.Errorf("%w: a name tree with an odd /Names", object.ErrSyntax))
		}
		for i := 0; i < len(array); i += 2 {
			key, ok := array[i].(*object.String)
			if !ok || key == nil {
				return altshiftErrors.NewWithTrace(fmt.Errorf("%w: a name tree key is a %T", object.ErrSyntax, array[i]))
			}
			entries[string(key.Value)] = array[i+1]
		}
	}
	if kids, ok := dictionary["Kids"]; ok {
		array, err := document.ResolveArray(kids)
		if err != nil {
			return fmt.Errorf("resolve array (kids): %w", err)
		}
		for _, kid := range array {
			if err := readNameTree(document, kid, entries, visited, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func pdfDate(t time.Time) string {
	return t.UTC().Format("D:20060102150405Z")
}

// Attach returns the document with the files embedded in an incremental update; the document's bytes are an exact prefix of the result.
func Attach(data []byte, files ...*File) ([]byte, error) {
	if len(files) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("files"))
	}
	seen := map[string]bool{}
	for _, f := range files {
		if err := f.validate(); err != nil {
			return nil, fmt.Errorf("file validate: %w", err)
		}
		if seen[f.Name] {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %q given twice", ErrDuplicateName, f.Name))
		}
		seen[f.Name] = true
	}

	document, err := file.Open(data)
	if err != nil {
		return nil, fmt.Errorf("file open: %w", err)
	}
	catalog, catalogReference, err := document.Catalog()
	if err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}

	names := object.Dictionary{}
	if value, ok := catalog["Names"]; ok {
		resolved, err := document.ResolveDictionary(value)
		if err != nil {
			return nil, fmt.Errorf("resolve dictionary (names): %w", err)
		}
		names = resolved.Clone()
	}
	entries := map[string]object.Object{}
	if err := readNameTree(document, names["EmbeddedFiles"], entries, map[int]bool{}, 0); err != nil {
		return nil, fmt.Errorf("read name tree (embedded files): %w", err)
	}

	update := document.NewUpdate()
	var associated object.Array
	if value, ok := catalog["AF"]; ok {
		existing, err := document.ResolveArray(value)
		if err != nil {
			return nil, fmt.Errorf("resolve array (associated files): %w", err)
		}
		associated = slices.Clone(existing)
	}

	for _, f := range files {
		// A name tree is keyed by bytes, so the key is the name as the file specification spells it.
		key := string(textString(f.Name).Value)
		if _, taken := entries[key]; taken {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %q", ErrDuplicateName, f.Name))
		}

		parameters := object.Dictionary{"Size": object.Integer(len(f.Data))}
		if !f.ModifiedAt.IsZero() {
			parameters["ModDate"] = &object.String{Value: []byte(pdfDate(f.ModifiedAt))}
		}
		streamDictionary := object.Dictionary{"Type": object.Name("EmbeddedFile"), "Params": parameters}
		if f.MimeType != "" {
			streamDictionary["Subtype"] = object.Name(f.MimeType)
		}
		streamReference := update.NewObject()
		if err := update.Set(streamReference, &object.Stream{Dictionary: streamDictionary, Data: f.Data}); err != nil {
			return nil, fmt.Errorf("update set (embedded file): %w", err)
		}

		relationship := f.Relationship
		if relationship == "" {
			relationship = RelationshipUnspecified
		}
		specification := object.Dictionary{
			"Type":           object.Name("Filespec"),
			"F":              textString(f.Name),
			"UF":             textString(f.Name),
			"EF":             object.Dictionary{"F": streamReference, "UF": streamReference},
			"AFRelationship": object.Name(relationship),
		}
		if f.Description != "" {
			specification["Desc"] = textString(f.Description)
		}
		specificationReference := update.NewObject()
		if err := update.Set(specificationReference, specification); err != nil {
			return nil, fmt.Errorf("update set (file specification): %w", err)
		}

		entries[key] = specificationReference
		associated = append(associated, specificationReference)
	}

	// The tree is written again whole and flat, its keys in the byte order a name tree requires.
	flat := make(object.Array, 0, 2*len(entries))
	for _, key := range slices.Sorted(maps.Keys(entries)) {
		flat = append(flat, &object.String{Value: []byte(key), Hex: !isAscii(key) || bytes.ContainsAny([]byte(key), "()\\")}, entries[key])
	}
	treeReference := update.NewObject()
	if err := update.Set(treeReference, object.Dictionary{"Names": flat}); err != nil {
		return nil, fmt.Errorf("update set (name tree): %w", err)
	}
	names["EmbeddedFiles"] = treeReference

	catalog = catalog.Clone()
	if reference, ok := catalog["Names"].(*object.Reference); ok && reference != nil {
		if err := update.Set(reference, names); err != nil {
			return nil, fmt.Errorf("update set (names): %w", err)
		}
	} else {
		catalog["Names"] = names
	}
	catalog["AF"] = associated
	if err := update.Set(catalogReference, catalog); err != nil {
		return nil, fmt.Errorf("update set (catalog): %w", err)
	}

	written, err := update.Write()
	if err != nil {
		return nil, fmt.Errorf("update write: %w", err)
	}
	return written.Data, nil
}
