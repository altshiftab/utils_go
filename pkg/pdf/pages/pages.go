// Package pages counts a PDF's pages and appends another PDF's pages to it as an incremental update, so the first document's bytes stay exactly as they were.
//
// The appended pages bring everything they reference, renumbered. When both documents are tagged, the appendix's structure tree is joined to the document's: its elements go under the document's own top-level element and its parent-tree keys are moved past the document's, so assistive technology reads one document rather than two.
package pages

import (
	"fmt"
	"maps"
	"slices"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/pdf/file"
	"github.com/altshiftab/utils_go/pkg/pdf/object"
)

// maxNumberTreeDepth bounds a number tree's nesting.
const maxNumberTreeDepth = 32

// Count returns the number of pages in a document.
func Count(data []byte) (int, error) {
	document, err := file.Open(data)
	if err != nil {
		return 0, fmt.Errorf("file open: %w", err)
	}
	tree, err := document.PageTree()
	if err != nil {
		return 0, fmt.Errorf("page tree: %w", err)
	}
	return len(tree.Pages), nil
}

func syntaxError(format string, arguments ...any) error {
	return altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s", object.ErrSyntax, fmt.Sprintf(format, arguments...)))
}

// copier copies objects from the appendix into the update, giving each the next free number once.
type copier struct {
	source *file.File
	update *file.Update
	mapped map[int]*object.Reference
	queue  []int
	// extra holds entries an object gets before it is copied, in the appendix's numbering, when it lacks them.
	extra map[int]object.Dictionary
	// after holds entries an object gets once copied, in the document's numbering.
	after              map[int]object.Dictionary
	structParentOffset int
	dropStructParents  bool
}

func (c *copier) reference(source *object.Reference) (object.Object, error) {
	if mapped, ok := c.mapped[source.Number]; ok {
		return mapped, nil
	}
	value, err := c.source.Resolve(source)
	if err != nil {
		return nil, fmt.Errorf("source resolve (%d): %w", source.Number, err)
	}
	// A reference to nothing, or to another generation, is null, and stays null rather than becoming a reference to an empty object.
	if value == nil {
		return nil, nil
	}
	destination := c.update.NewObject()
	c.mapped[source.Number] = destination
	c.queue = append(c.queue, source.Number)
	return destination, nil
}

func (c *copier) dictionary(source object.Dictionary) (object.Dictionary, error) {
	out := make(object.Dictionary, len(source))
	for key, value := range source {
		// Keys into the parent tree move with it.
		if key == "StructParent" || key == "StructParents" {
			if integer, ok := value.(object.Integer); ok {
				if !c.dropStructParents {
					out[key] = integer + object.Integer(c.structParentOffset)
				}
				continue
			}
		}
		transformed, err := c.transform(value)
		if err != nil {
			return nil, err
		}
		out[key] = transformed
	}
	return out, nil
}

func (c *copier) transform(value object.Object) (object.Object, error) {
	switch typed := value.(type) {
	case *object.Reference:
		if typed == nil {
			return nil, nil
		}
		return c.reference(typed)
	case object.Array:
		out := make(object.Array, 0, len(typed))
		for _, element := range typed {
			transformed, err := c.transform(element)
			if err != nil {
				return nil, err
			}
			out = append(out, transformed)
		}
		return out, nil
	case object.Dictionary:
		return c.dictionary(typed)
	case *object.Stream:
		if typed == nil {
			return nil, nil
		}
		dictionary, err := c.dictionary(typed.Dictionary)
		if err != nil {
			return nil, err
		}
		return &object.Stream{Dictionary: dictionary, Data: typed.Data}, nil
	default:
		return value, nil
	}
}

func withEntries(value object.Object, entries object.Dictionary, overwrite bool) object.Object {
	dictionary, ok := value.(object.Dictionary)
	if !ok || len(entries) == 0 {
		return value
	}
	dictionary = dictionary.Clone()
	for key, entry := range entries {
		if _, present := dictionary[key]; overwrite || !present {
			dictionary[key] = entry
		}
	}
	return dictionary
}

// run copies every object reached so far, and whatever they reach in turn.
func (c *copier) run() error {
	for len(c.queue) > 0 {
		number := c.queue[0]
		c.queue = c.queue[1:]

		value, err := c.source.Object(number)
		if err != nil {
			return fmt.Errorf("source object (%d): %w", number, err)
		}
		transformed, err := c.transform(withEntries(value, c.extra[number], false))
		if err != nil {
			return fmt.Errorf("transform (%d): %w", number, err)
		}
		transformed = withEntries(transformed, c.after[number], true)
		if err := c.update.Set(c.mapped[number], transformed); err != nil {
			return fmt.Errorf("update set (%d): %w", number, err)
		}
	}
	return nil
}

// kidsOf reads a /K entry as a list of kids: an array, possibly indirect, or a single kid.
func kidsOf(document *file.File, value object.Object) (object.Array, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case object.Array:
		return typed, nil
	case *object.Reference:
		resolved, err := document.Resolve(typed)
		if err != nil {
			return nil, fmt.Errorf("resolve (kids): %w", err)
		}
		// A reference to an array is the list itself; a reference to anything else is one kid.
		if array, ok := resolved.(object.Array); ok {
			return array, nil
		}
		return object.Array{typed}, nil
	default:
		return object.Array{typed}, nil
	}
}

// structureTree is a document's StructTreeRoot and, when it has exactly one top-level element, that element.
type structureTree struct {
	reference  *object.Reference
	dictionary object.Dictionary
	top        *object.Reference
	topDict    object.Dictionary
	entries    map[int]object.Object
}

func readStructureTree(document *file.File, catalog object.Dictionary) (*structureTree, error) {
	value, ok := catalog["StructTreeRoot"]
	if !ok {
		return nil, nil
	}
	dictionary, err := document.ResolveDictionary(value)
	if err != nil {
		return nil, fmt.Errorf("resolve dictionary (struct tree root): %w", err)
	}
	tree := &structureTree{dictionary: dictionary, entries: map[int]object.Object{}}
	tree.reference, _ = value.(*object.Reference)

	kids, err := kidsOf(document, dictionary["K"])
	if err != nil {
		return nil, fmt.Errorf("kids of (struct tree root): %w", err)
	}
	if len(kids) == 1 {
		if top, ok := kids[0].(*object.Reference); ok {
			topDict, err := document.ResolveDictionary(top)
			if err != nil {
				return nil, fmt.Errorf("resolve dictionary (top structure element): %w", err)
			}
			tree.top, tree.topDict = top, topDict
		}
	}

	if err := readNumberTree(document, dictionary["ParentTree"], tree.entries, map[int]bool{}, 0); err != nil {
		return nil, fmt.Errorf("read number tree (parent tree): %w", err)
	}
	return tree, nil
}

// readNumberTree gathers a number tree's entries; a node reached twice is refused, so a tree that shares nodes cannot multiply the work.
func readNumberTree(document *file.File, node object.Object, entries map[int]object.Object, visited map[int]bool, depth int) error {
	if depth > maxNumberTreeDepth {
		return syntaxError("a number tree deeper than %d", maxNumberTreeDepth)
	}
	if node == nil {
		return nil
	}
	if reference, ok := node.(*object.Reference); ok && reference != nil {
		if visited[reference.Number] {
			return syntaxError("a number tree reaches %d twice", reference.Number)
		}
		visited[reference.Number] = true
	}
	dictionary, err := document.ResolveDictionary(node)
	if err != nil {
		return fmt.Errorf("resolve dictionary (number tree node): %w", err)
	}
	if nums, ok := dictionary["Nums"]; ok {
		array, err := document.ResolveArray(nums)
		if err != nil {
			return fmt.Errorf("resolve array (nums): %w", err)
		}
		if len(array)%2 != 0 {
			return syntaxError("a number tree with an odd /Nums")
		}
		for i := 0; i < len(array); i += 2 {
			key, ok := array[i].(object.Integer)
			if !ok {
				return syntaxError("a number tree key is a %T", array[i])
			}
			entries[int(key)] = array[i+1]
		}
	}
	if kids, ok := dictionary["Kids"]; ok {
		array, err := document.ResolveArray(kids)
		if err != nil {
			return fmt.Errorf("resolve array (kids): %w", err)
		}
		for _, kid := range array {
			if err := readNumberTree(document, kid, entries, visited, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func nextKey(tree *structureTree) int {
	next := 0
	for key := range tree.entries {
		next = max(next, key+1)
	}
	if declared, ok := tree.dictionary.Integer("ParentTreeNextKey"); ok {
		next = max(next, int(declared))
	}
	return next
}

// structurePlan is how the appendix's structure joins the document's, decided before anything is copied so that whatever a page reaches maps onto the document's tree rather than dragging the appendix's along.
type structurePlan struct {
	target     *structureTree
	source     *structureTree
	splice     bool
	sourceKids object.Array
}

func planStructure(c *copier, target *structureTree, source *structureTree) (*structurePlan, error) {
	if _, ok := source.dictionary["IDTree"]; ok {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: an appendix whose structure tree has an /IDTree", file.ErrUnsupported))
	}
	plan := &structurePlan{target: target, source: source}
	c.structParentOffset = nextKey(target)

	if source.reference != nil && target.reference != nil {
		c.mapped[source.reference.Number] = target.reference
	}

	// One top-level element on each side, holding only elements: the appendix's go under the document's.
	plan.splice = target.top != nil && source.top != nil
	if plan.splice {
		kids, err := kidsOf(c.source, source.topDict["K"])
		if err != nil {
			return nil, fmt.Errorf("kids of (appendix top element): %w", err)
		}
		for _, kid := range kids {
			if _, ok := kid.(*object.Reference); !ok {
				plan.splice = false
				break
			}
		}
		plan.sourceKids = kids
	}
	if plan.splice {
		c.mapped[source.top.Number] = target.top
		if language, ok := source.topDict["Lang"]; ok && language != nil {
			for _, kid := range plan.sourceKids {
				reference, _ := kid.(*object.Reference)
				c.extra[reference.Number] = object.Dictionary{"Lang": language}
			}
		}
		return plan, nil
	}

	kids, err := kidsOf(c.source, source.dictionary["K"])
	if err != nil {
		return nil, fmt.Errorf("kids of (appendix struct tree root): %w", err)
	}
	plan.sourceKids = kids
	return plan, nil
}

// joinStructure copies the appendix's elements and parent-tree entries into the document's tree, once the pages are copied.
func joinStructure(c *copier, update *file.Update, destination *file.File, catalog object.Dictionary, catalogReference *object.Reference, plan *structurePlan) error {
	target, source := plan.target, plan.source

	copiedKids := make(object.Array, 0, len(plan.sourceKids))
	for _, kid := range plan.sourceKids {
		copied, err := c.transform(kid)
		if err != nil {
			return fmt.Errorf("transform (structure element): %w", err)
		}
		copiedKids = append(copiedKids, copied)
	}

	entries := make(map[int]object.Object, len(target.entries)+len(source.entries))
	maps.Copy(entries, target.entries)
	offset := c.structParentOffset
	for _, key := range slices.Sorted(maps.Keys(source.entries)) {
		copied, err := c.transform(source.entries[key])
		if err != nil {
			return fmt.Errorf("transform (parent tree entry %d): %w", key, err)
		}
		entries[key+offset] = copied
	}
	if err := c.run(); err != nil {
		return fmt.Errorf("run: %w", err)
	}

	// The parent tree is written again whole, flat.
	nums := make(object.Array, 0, 2*len(entries))
	for _, key := range slices.Sorted(maps.Keys(entries)) {
		nums = append(nums, object.Integer(key), entries[key])
	}
	parentTree := update.NewObject()
	if err := update.Set(parentTree, object.Dictionary{"Nums": nums}); err != nil {
		return fmt.Errorf("update set (parent tree): %w", err)
	}

	root := target.dictionary.Clone()
	root["ParentTree"] = parentTree
	root["ParentTreeNextKey"] = object.Integer(offset + nextKey(source))
	for _, key := range []object.Name{"RoleMap", "ClassMap"} {
		merged, err := mergeMaps(destination, root[key], source.dictionary[key], c)
		if err != nil {
			return fmt.Errorf("merge maps (%s): %w", key, err)
		}
		if merged != nil {
			root[key] = merged
		}
	}

	if plan.splice {
		top := target.topDict.Clone()
		existing, err := kidsOf(destination, top["K"])
		if err != nil {
			return fmt.Errorf("kids of (document top element): %w", err)
		}
		top["K"] = append(slices.Clone(existing), copiedKids...)
		if err := update.Set(target.top, top); err != nil {
			return fmt.Errorf("update set (top structure element): %w", err)
		}
	} else {
		existing, err := kidsOf(destination, root["K"])
		if err != nil {
			return fmt.Errorf("kids of (document struct tree root): %w", err)
		}
		root["K"] = append(slices.Clone(existing), copiedKids...)
	}

	if target.reference != nil {
		if err := update.Set(target.reference, root); err != nil {
			return fmt.Errorf("update set (struct tree root): %w", err)
		}
		return nil
	}
	catalog = catalog.Clone()
	catalog["StructTreeRoot"] = root
	if err := update.Set(catalogReference, catalog); err != nil {
		return fmt.Errorf("update set (catalog): %w", err)
	}
	return nil
}

// mergeMaps joins a role or class map, the document's entries winning.
func mergeMaps(destination *file.File, target object.Object, sourceValue object.Object, c *copier) (object.Object, error) {
	if sourceValue == nil {
		return nil, nil
	}
	sourceMap, err := c.source.ResolveDictionary(sourceValue)
	if err != nil {
		return nil, fmt.Errorf("resolve dictionary (appendix map): %w", err)
	}
	merged := object.Dictionary{}
	if target != nil {
		targetMap, err := destination.ResolveDictionary(target)
		if err != nil {
			return nil, fmt.Errorf("resolve dictionary (document map): %w", err)
		}
		merged = targetMap.Clone()
	}
	for key, value := range sourceMap {
		if _, present := merged[key]; present {
			continue
		}
		copied, err := c.transform(value)
		if err != nil {
			return nil, err
		}
		merged[key] = copied
	}
	if err := c.run(); err != nil {
		return nil, fmt.Errorf("run: %w", err)
	}
	return merged, nil
}

// pageEntries is what an appended page gets before it is copied: what it inherited in the appendix, and explicit defaults for what it would otherwise inherit from the document's root.
func pageEntries(page *file.Page, destinationRoot object.Dictionary) object.Dictionary {
	entries := page.Inherited.Clone()
	has := func(key object.Name) bool {
		_, own := page.Dictionary[key]
		_, inherited := entries[key]
		return own || inherited
	}
	if _, ok := destinationRoot["Rotate"]; ok && !has("Rotate") {
		entries["Rotate"] = object.Integer(0)
	}
	if _, ok := destinationRoot["Resources"]; ok && !has("Resources") {
		entries["Resources"] = object.Dictionary{}
	}
	if _, ok := destinationRoot["CropBox"]; ok && !has("CropBox") {
		// An absent crop box is the media box.
		mediaBox, ok := page.Dictionary["MediaBox"]
		if !ok {
			mediaBox = entries["MediaBox"]
		}
		if mediaBox != nil {
			entries["CropBox"] = mediaBox
		}
	}
	return entries
}

// Append returns the document with the appendix's pages after its own, written as an incremental update; the document's bytes are an exact prefix of the result.
func Append(document []byte, appendix []byte) ([]byte, error) {
	if len(appendix) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("appendix"))
	}

	destination, err := file.Open(document)
	if err != nil {
		return nil, fmt.Errorf("file open (document): %w", err)
	}
	source, err := file.Open(appendix)
	if err != nil {
		return nil, fmt.Errorf("file open (appendix): %w", err)
	}

	catalog, catalogReference, err := destination.Catalog()
	if err != nil {
		return nil, fmt.Errorf("catalog (document): %w", err)
	}
	pageTree, err := destination.PageTree()
	if err != nil {
		return nil, fmt.Errorf("page tree (document): %w", err)
	}
	destinationRoot, err := destination.ResolveDictionary(pageTree.Root)
	if err != nil {
		return nil, fmt.Errorf("resolve dictionary (page tree root): %w", err)
	}
	sourceCatalog, sourceCatalogReference, err := source.Catalog()
	if err != nil {
		return nil, fmt.Errorf("catalog (appendix): %w", err)
	}
	sourcePageTree, err := source.PageTree()
	if err != nil {
		return nil, fmt.Errorf("page tree (appendix): %w", err)
	}
	if len(sourcePageTree.Pages) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("appendix pages"))
	}

	update := destination.NewUpdate()
	c := &copier{
		source: source,
		update: update,
		mapped: map[int]*object.Reference{sourceCatalogReference.Number: catalogReference},
		extra:  map[int]object.Dictionary{},
		after:  map[int]object.Dictionary{},
	}
	// Nothing copied may drag the appendix's page tree along: its nodes become the document's root.
	for _, node := range sourcePageTree.InnerNode {
		c.mapped[node.Number] = pageTree.Root
	}

	target, err := readStructureTree(destination, catalog)
	if err != nil {
		return nil, fmt.Errorf("read structure tree (document): %w", err)
	}
	sourceStructure, err := readStructureTree(source, sourceCatalog)
	if err != nil {
		return nil, fmt.Errorf("read structure tree (appendix): %w", err)
	}
	var plan *structurePlan
	if sourceStructure == nil || target == nil {
		// Unless both are tagged, the appendix's keys into a parent tree would point at nothing.
		c.dropStructParents = true
	} else {
		plan, err = planStructure(c, target, sourceStructure)
		if err != nil {
			return nil, fmt.Errorf("plan structure: %w", err)
		}
	}

	newPages := make(object.Array, 0, len(sourcePageTree.Pages))
	for _, page := range sourcePageTree.Pages {
		c.extra[page.Reference.Number] = pageEntries(page, destinationRoot)
		c.after[page.Reference.Number] = object.Dictionary{"Parent": pageTree.Root}
		copied, err := c.reference(page.Reference)
		if err != nil {
			return nil, fmt.Errorf("reference (page %d): %w", page.Reference.Number, err)
		}
		newPages = append(newPages, copied)
	}
	if err := c.run(); err != nil {
		return nil, fmt.Errorf("run (pages): %w", err)
	}

	if plan != nil {
		if err := joinStructure(c, update, destination, catalog, catalogReference, plan); err != nil {
			return nil, fmt.Errorf("join structure: %w", err)
		}
	}

	root := destinationRoot.Clone()
	count, _ := root.Integer("Count")
	root["Count"] = count + object.Integer(len(newPages))
	switch kids := root["Kids"].(type) {
	case *object.Reference:
		array, err := destination.ResolveArray(kids)
		if err != nil {
			return nil, fmt.Errorf("resolve array (kids): %w", err)
		}
		if err := update.Set(kids, append(slices.Clone(array), newPages...)); err != nil {
			return nil, fmt.Errorf("update set (kids): %w", err)
		}
	case object.Array:
		root["Kids"] = append(slices.Clone(kids), newPages...)
	default:
		return nil, syntaxError("the page tree root's /Kids is a %T", kids)
	}
	if err := update.Set(pageTree.Root, root); err != nil {
		return nil, fmt.Errorf("update set (page tree root): %w", err)
	}

	written, err := update.Write()
	if err != nil {
		return nil, fmt.Errorf("update write: %w", err)
	}
	return written.Data, nil
}
