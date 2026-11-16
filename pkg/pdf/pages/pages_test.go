package pages

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/altshiftab/utils_go/pkg/pdf/file"
	"github.com/altshiftab/utils_go/pkg/pdf/object"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

// buildPdf writes one revision with a classic table.
func buildPdf(objects map[int]string) []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	offsets := map[int]int{}
	for _, number := range slices.Sorted(maps.Keys(objects)) {
		offsets[number] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", number, objects[number])
	}
	xref := out.Len()
	out.WriteString("xref\n0 1\n0000000000 65535 f\r\n")
	for _, number := range slices.Sorted(maps.Keys(objects)) {
		fmt.Fprintf(&out, "%d 1\n%010d 00000 n\r\n", number, offsets[number])
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", slices.Max(slices.Collect(maps.Keys(objects)))+1, xref)
	return out.Bytes()
}

// danglingReferences walks everything reachable from the catalog and lists references to objects that are not there.
func danglingReferences(t *testing.T, document *file.File) []int {
	t.Helper()
	_, root, err := document.Catalog()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	var dangling []int
	seen := map[int]bool{}
	queue := []object.Object{root}
	for len(queue) > 0 {
		value := queue[0]
		queue = queue[1:]
		switch typed := value.(type) {
		case *object.Reference:
			if seen[typed.Number] {
				continue
			}
			seen[typed.Number] = true
			resolved, err := document.Object(typed.Number)
			if err != nil {
				t.Fatalf("object %d: %v", typed.Number, err)
			}
			if resolved == nil {
				dangling = append(dangling, typed.Number)
			}
			queue = append(queue, resolved)
		case object.Array:
			for _, element := range typed {
				queue = append(queue, element)
			}
		case object.Dictionary:
			for _, element := range typed {
				queue = append(queue, element)
			}
		case *object.Stream:
			queue = append(queue, typed.Dictionary)
		}
	}
	return dangling
}

func parentTree(t *testing.T, document *file.File) (map[int]object.Object, object.Dictionary) {
	t.Helper()
	catalog, _, err := document.Catalog()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	root, err := document.ResolveDictionary(catalog["StructTreeRoot"])
	if err != nil {
		t.Fatalf("struct tree root: %v", err)
	}
	entries := map[int]object.Object{}
	if err := readNumberTree(document, root["ParentTree"], entries, map[int]bool{}, 0); err != nil {
		t.Fatalf("read number tree: %v", err)
	}
	return entries, root
}

func TestAppendTagged(t *testing.T) {
	t.Parallel()

	document, appendix := readFixture(t, "document.pdf"), readFixture(t, "appendix.pdf")
	combined, err := Append(document, appendix)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if !bytes.HasPrefix(combined, document) {
		t.Fatal("the document's bytes are not a prefix of the result")
	}

	result, err := file.Open(combined)
	if err != nil {
		t.Fatalf("open result: %v", err)
	}
	tree, err := result.PageTree()
	if err != nil {
		t.Fatalf("page tree: %v", err)
	}
	if len(tree.Pages) != 3 {
		t.Fatalf("%d pages, want 3", len(tree.Pages))
	}
	rootDictionary, err := result.ResolveDictionary(tree.Root)
	if err != nil {
		t.Fatalf("page tree root: %v", err)
	}
	if count, _ := rootDictionary.Integer("Count"); count != 3 {
		t.Errorf("/Count %d, want 3", count)
	}
	last := tree.Pages[2]
	if parent, _ := last.Dictionary.Reference("Parent"); parent == nil || parent.Number != tree.Root.Number {
		t.Errorf("the appended page's /Parent is %v, not the document's root", last.Dictionary["Parent"])
	}
	if _, ok := last.Dictionary["MediaBox"]; !ok {
		t.Error("the appended page has no /MediaBox")
	}

	if dangling := danglingReferences(t, result); len(dangling) != 0 {
		t.Errorf("references to missing objects: %v", dangling)
	}

	reopen := func(t *testing.T) (*file.File, *file.PageTree) {
		t.Helper()
		result, err := file.Open(combined)
		if err != nil {
			t.Fatalf("open result: %v", err)
		}
		tree, err := result.PageTree()
		if err != nil {
			t.Fatalf("page tree: %v", err)
		}
		return result, tree
	}

	t.Run("every key into the parent tree is distinct and present", func(t *testing.T) {
		t.Parallel()
		result, tree := reopen(t)
		entries, structRoot := parentTree(t, result)
		used := map[int]string{}
		claim := func(key object.Integer, owner string) {
			if previous, ok := used[int(key)]; ok {
				t.Errorf("key %d is both %s and %s", key, previous, owner)
			}
			used[int(key)] = owner
			if _, ok := entries[int(key)]; !ok {
				t.Errorf("key %d of %s has no parent tree entry", key, owner)
			}
		}
		for i, page := range tree.Pages {
			key, ok := page.Dictionary.Integer("StructParents")
			if !ok {
				t.Errorf("page %d has no /StructParents", i+1)
				continue
			}
			claim(key, fmt.Sprintf("page %d", i+1))
			annotations, err := result.ResolveArray(page.Dictionary["Annots"])
			if err != nil {
				continue
			}
			for _, annotation := range annotations {
				dictionary, err := result.ResolveDictionary(annotation)
				if err != nil {
					t.Fatalf("annotation: %v", err)
				}
				if key, ok := dictionary.Integer("StructParent"); ok {
					claim(key, fmt.Sprintf("an annotation on page %d", i+1))
				}
			}
		}
		if next, _ := structRoot.Integer("ParentTreeNextKey"); int(next) <= slices.Max(slices.Collect(maps.Keys(entries))) {
			t.Errorf("/ParentTreeNextKey %d is not past every key", next)
		}
	})

	t.Run("the appendix's elements sit under the document's one top element", func(t *testing.T) {
		t.Parallel()
		result, _ := reopen(t)
		_, structRoot := parentTree(t, result)
		kids, err := kidsOf(result, structRoot["K"])
		if err != nil {
			t.Fatalf("kids of: %v", err)
		}
		if len(kids) != 1 {
			t.Fatalf("%d top-level elements, want 1", len(kids))
		}
		top, err := result.ResolveDictionary(kids[0])
		if err != nil {
			t.Fatalf("top element: %v", err)
		}
		sourceDocument, err := file.Open(document)
		if err != nil {
			t.Fatalf("open document: %v", err)
		}
		sourceAppendix, err := file.Open(appendix)
		if err != nil {
			t.Fatalf("open appendix: %v", err)
		}
		documentCatalog, _, _ := sourceDocument.Catalog()
		appendixCatalog, _, _ := sourceAppendix.Catalog()
		documentTree, _ := readStructureTree(sourceDocument, documentCatalog)
		appendixTree, _ := readStructureTree(sourceAppendix, appendixCatalog)
		documentKids, _ := kidsOf(sourceDocument, documentTree.topDict["K"])
		appendixKids, _ := kidsOf(sourceAppendix, appendixTree.topDict["K"])
		topKids, _ := kidsOf(result, top["K"])
		want := len(documentKids) + len(appendixKids)
		if len(topKids) != want {
			t.Errorf("the top element has %d kids, want %d", len(topKids), want)
		}
		// The appendix's elements now name the document's top element as their parent.
		appended, err := result.ResolveDictionary(topKids[want-1])
		if err != nil {
			t.Fatalf("appended element: %v", err)
		}
		if parent, _ := appended.Reference("P"); parent == nil || parent.Number != kids[0].(*object.Reference).Number {
			t.Errorf("the appended element's /P is %v", appended["P"])
		}
	})

	t.Run("a second append keeps the keys apart", func(t *testing.T) {
		t.Parallel()
		twice, err := Append(combined, appendix)
		if err != nil {
			t.Fatalf("append again: %v", err)
		}
		again, err := file.Open(twice)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		againTree, err := again.PageTree()
		if err != nil {
			t.Fatalf("page tree: %v", err)
		}
		keys := map[object.Integer]bool{}
		for _, page := range againTree.Pages {
			key, _ := page.Dictionary.Integer("StructParents")
			if keys[key] {
				t.Errorf("/StructParents %d is used twice", key)
			}
			keys[key] = true
		}
		if dangling := danglingReferences(t, again); len(dangling) != 0 {
			t.Errorf("references to missing objects: %v", dangling)
		}
	})
}

func TestAppendSynthetic(t *testing.T) {
	t.Parallel()

	untagged := buildPdf(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] >>",
	})
	// The page takes its media box and resources from its parent, and its content from a stream.
	inheriting := buildPdf(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 300 300] /Resources << /Font << /F1 5 0 R >> >> >>",
		3: "<< /Type /Page /Parent 2 0 R /Contents 4 0 R /StructParents 0 >>",
		4: "<< /Length 3 >>\nstream\nq Q\nendstream",
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	})

	testCases := []struct {
		name     string
		document []byte
		appendix []byte
		check    func(t *testing.T, page *file.Page, result *file.File)
	}{
		{
			name:     "inherited attributes are carried onto the page",
			document: untagged,
			appendix: inheriting,
			check: func(t *testing.T, page *file.Page, result *file.File) {
				if box, err := result.ResolveArray(page.Dictionary["MediaBox"]); err != nil || len(box) != 4 || box[2] != object.Integer(300) {
					t.Errorf("media box %v, %v", page.Dictionary["MediaBox"], err)
				}
				resources, err := result.ResolveDictionary(page.Dictionary["Resources"])
				if err != nil {
					t.Fatalf("resources: %v", err)
				}
				fonts, err := result.ResolveDictionary(resources["Font"])
				if err != nil {
					t.Fatalf("fonts: %v", err)
				}
				font, err := result.ResolveDictionary(fonts["F1"])
				if err != nil || font["BaseFont"] != object.Name("Helvetica") {
					t.Errorf("font %v, %v", font, err)
				}
				// The document is untagged, so a key into the appendix's parent tree would point at nothing.
				if _, ok := page.Dictionary["StructParents"]; ok {
					t.Error("the page kept /StructParents in an untagged document")
				}
			},
		},
		{
			name:     "an untagged appendix",
			document: inheriting,
			appendix: untagged,
			check: func(t *testing.T, page *file.Page, _ *file.File) {
				if _, ok := page.Dictionary["MediaBox"]; !ok {
					t.Error("the page lost its media box")
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			combined, err := Append(testCase.document, testCase.appendix)
			if err != nil {
				t.Fatalf("%s: append: %v", testCase.name, err)
			}
			if !bytes.HasPrefix(combined, testCase.document) {
				t.Fatalf("%s: the document's bytes are not a prefix", testCase.name)
			}
			result, err := file.Open(combined)
			if err != nil {
				t.Fatalf("%s: open: %v", testCase.name, err)
			}
			tree, err := result.PageTree()
			if err != nil || len(tree.Pages) != 2 {
				t.Fatalf("%s: page tree: %v", testCase.name, err)
			}
			if dangling := danglingReferences(t, result); len(dangling) != 0 {
				t.Errorf("%s: references to missing objects: %v", testCase.name, dangling)
			}
			testCase.check(t, tree.Pages[1], result)
		})
	}
}

// taggedDocument is a one-page tagged document whose page-tree root sets attributes its pages inherit.
func taggedDocument(rootExtra string, parentTree string) []byte {
	return buildPdf(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 10 0 R /MarkInfo << /Marked true >> >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 595 842] " + rootExtra + " >>",
		3:  "<< /Type /Page /Parent 2 0 R /StructParents 0 >>",
		10: "<< /Type /StructTreeRoot /K 11 0 R /ParentTree " + parentTree + " /ParentTreeNextKey 1 >>",
		11: "<< /Type /StructElem /S /Document /P 10 0 R /K [13 0 R] >>",
		12: "<< /Nums [0 [13 0 R]] >>",
		13: "<< /Type /StructElem /S /P /P 11 0 R /Pg 3 0 R /K 0 >>",
	})
}

// taggedAppendix keeps its top element's kids in an indirect array, and has a link whose action names a structure element.
func taggedAppendix() []byte {
	return buildPdf(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 10 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 300 400] >>",
		3:  "<< /Type /Page /Parent 2 0 R /StructParents 0 /Annots [15 0 R] >>",
		10: "<< /Type /StructTreeRoot /K 11 0 R /ParentTree 12 0 R /ParentTreeNextKey 2 >>",
		11: "<< /Type /StructElem /S /Document /P 10 0 R /K 14 0 R >>",
		12: "<< /Nums [0 [13 0 R] 1 13 0 R] >>",
		13: "<< /Type /StructElem /S /Link /P 11 0 R /Pg 3 0 R /K [0 << /Type /OBJR /Obj 15 0 R >>] >>",
		14: "[13 0 R]",
		15: "<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] /P 3 0 R /StructParent 1 /A << /S /GoTo /SD [13 0 R /Fit] >> >>",
	})
}

func countObjects(t *testing.T, result *file.File, match func(object.Dictionary) bool) int {
	t.Helper()
	count := 0
	for number := range result.Size() {
		value, err := result.Object(number)
		if err != nil {
			t.Fatalf("object %d: %v", number, err)
		}
		if dictionary, ok := value.(object.Dictionary); ok && match(dictionary) {
			count++
		}
	}
	return count
}

func TestAppendStructureEdges(t *testing.T) {
	t.Parallel()

	combined, err := Append(taggedDocument("/Rotate 90 /CropBox [0 0 50 50] /Resources << /Font << >> >>", "12 0 R"), taggedAppendix())
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	reopen := func(t *testing.T) (*file.File, *file.Page) {
		t.Helper()
		result, err := file.Open(combined)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		tree, err := result.PageTree()
		if err != nil || len(tree.Pages) != 2 {
			t.Fatalf("page tree: %v", err)
		}
		return result, tree.Pages[1]
	}

	t.Run("nothing the appendix's pages reach brings a second tree along", func(t *testing.T) {
		t.Parallel()
		result, _ := reopen(t)
		roots := countObjects(t, result, func(d object.Dictionary) bool { kind, _ := d.Name("Type"); return kind == "StructTreeRoot" })
		documents := countObjects(t, result, func(d object.Dictionary) bool { kind, _ := d.Name("S"); return kind == "Document" })
		if roots != 1 || documents != 1 {
			t.Errorf("%d structure tree roots and %d /Document elements, want one of each", roots, documents)
		}
		if dangling := danglingReferences(t, result); len(dangling) != 0 {
			t.Errorf("references to missing objects: %v", dangling)
		}
	})

	t.Run("the indirect kids are spliced in, not nested", func(t *testing.T) {
		t.Parallel()
		result, _ := reopen(t)
		entries, structRoot := parentTree(t, result)
		top, err := result.ResolveDictionary(structRoot["K"])
		if err != nil {
			t.Fatalf("top element: %v", err)
		}
		kids, ok := top["K"].(object.Array)
		if !ok || len(kids) != 2 {
			t.Fatalf("the top element's /K is %#v, want a direct array of two elements", top["K"])
		}
		for _, kid := range kids {
			if _, isReference := kid.(*object.Reference); !isReference {
				t.Errorf("kid %#v is not an element reference", kid)
			}
		}
		if len(entries) != 3 {
			t.Errorf("%d parent tree entries, want 3", len(entries))
		}
	})

	t.Run("attributes the document's root sets are not inherited", func(t *testing.T) {
		t.Parallel()
		result, appended := reopen(t)
		if rotate := appended.Dictionary["Rotate"]; rotate != object.Integer(0) {
			t.Errorf("/Rotate %v, want an explicit 0", rotate)
		}
		cropBox, err := result.ResolveArray(appended.Dictionary["CropBox"])
		if err != nil || len(cropBox) != 4 || cropBox[3] != object.Integer(400) {
			t.Errorf("/CropBox %v, %v, want the appended page's own media box", appended.Dictionary["CropBox"], err)
		}
		if resources, err := result.ResolveDictionary(appended.Dictionary["Resources"]); err != nil || len(resources) != 0 {
			t.Errorf("/Resources %v, %v, want an explicit empty dictionary", appended.Dictionary["Resources"], err)
		}
	})
}

// TestAppendSharedNumberTree refuses a parent tree that reaches one node from many places, which would otherwise multiply the work per level.
func TestAppendSharedNumberTree(t *testing.T) {
	t.Parallel()

	document := buildPdf(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 10 0 R >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 595 842] >>",
		3:  "<< /Type /Page /Parent 2 0 R /StructParents 0 >>",
		10: "<< /Type /StructTreeRoot /K 11 0 R /ParentTree 20 0 R >>",
		11: "<< /Type /StructElem /S /Document /P 10 0 R >>",
		20: "<< /Kids [21 0 R 21 0 R 21 0 R 21 0 R] >>",
		21: "<< /Kids [22 0 R 22 0 R 22 0 R 22 0 R] >>",
		22: "<< /Nums [0 null] >>",
	})
	if _, err := Append(document, taggedAppendix()); !errors.Is(err, object.ErrSyntax) {
		t.Errorf("append = %v, want a syntax error", err)
	}
}

func FuzzAppend(f *testing.F) {
	document, err := os.ReadFile("testdata/document.pdf")
	if err != nil {
		f.Fatalf("read fixture: %v", err)
	}
	appendix, err := os.ReadFile("testdata/appendix.pdf")
	if err != nil {
		f.Fatalf("read fixture: %v", err)
	}
	f.Add(appendix)
	f.Add(taggedAppendix())
	f.Fuzz(func(t *testing.T, data []byte) {
		if combined, err := Append(document, data); err == nil {
			if !bytes.HasPrefix(combined, document) {
				t.Fatal("the document's bytes are not a prefix")
			}
			if _, err := file.Open(combined); err != nil {
				t.Fatalf("the result does not open: %v", err)
			}
		}
		_, _ = Append(data, appendix)
	})
}

func TestAppendRefuses(t *testing.T) {
	t.Parallel()

	document := readFixture(t, "document.pdf")
	noPages := buildPdf(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [] /Count 0 >>",
	})

	testCases := []struct {
		name     string
		document []byte
		appendix []byte
		wantErr  error
	}{
		{name: "no appendix", document: document},
		{name: "an appendix without pages", document: document, appendix: noPages},
		{name: "a document that is not a pdf", document: []byte("hello"), appendix: document, wantErr: object.ErrSyntax},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := Append(testCase.document, testCase.appendix)
			if err == nil {
				t.Fatalf("%s: append succeeded", testCase.name)
			}
			if testCase.wantErr != nil && !errors.Is(err, testCase.wantErr) {
				t.Errorf("%s: append = %v, want %v", testCase.name, err, testCase.wantErr)
			}
		})
	}
}

func TestCount(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		data    []byte
		want    int
		wantErr bool
	}{
		{name: "two pages", data: readFixture(t, "document.pdf"), want: 2},
		{name: "one page", data: readFixture(t, "appendix.pdf"), want: 1},
		{name: "not a pdf", data: []byte("hello"), wantErr: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			count, err := Count(testCase.data)
			if (err != nil) != testCase.wantErr || count != testCase.want {
				t.Errorf("%s: count = %d, %v", testCase.name, count, err)
			}
		})
	}
}
