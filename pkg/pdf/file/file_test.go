package file

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/altshiftab/utils_go/pkg/pdf/object"
)

// revision is one section of a test file: objects by number, numbers freed, and trailer entries beyond /Size, /Root and /Prev.
type revision struct {
	objects map[int]string
	freed   []int
	trailer string
}

// buildPdf writes the revisions as a file with classic tables, each section listing its own objects.
func buildPdf(size int, revisions ...*revision) []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n")
	previous := -1
	for _, rev := range revisions {
		offsets := map[int]int{}
		for _, number := range slices.Sorted(maps.Keys(rev.objects)) {
			offsets[number] = out.Len()
			fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", number, rev.objects[number])
		}
		xref := out.Len()
		out.WriteString("xref\n")
		if previous < 0 {
			out.WriteString("0 1\n0000000000 65535 f\r\n")
		}
		for _, number := range slices.Sorted(maps.Keys(rev.objects)) {
			fmt.Fprintf(&out, "%d 1\n%010d 00000 n\r\n", number, offsets[number])
		}
		for _, number := range rev.freed {
			fmt.Fprintf(&out, "%d 1\n0000000000 00001 f\r\n", number)
		}
		prev := ""
		if previous >= 0 {
			prev = fmt.Sprintf(" /Prev %d", previous)
		}
		fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R%s %s >>\nstartxref\n%d\n%%%%EOF\n", size, prev, rev.trailer, xref)
		previous = xref
	}
	return out.Bytes()
}

func basicRevision() *revision {
	return &revision{objects: map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 3 /MediaBox [0 0 595 842] /Resources << /Font << >> >> >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>",
		4: "<< /Length 6 0 R >>\nstream\nhello\nendstream",
		5: "<< /Type /Pages /Parent 2 0 R /Kids [7 0 R] /Count 1 /Rotate 90 >>",
		6: "5",
		7: "<< /Type /Page /Parent 5 0 R >>",
	}}
}

func TestOpenAndRead(t *testing.T) {
	t.Parallel()

	file, err := Open(buildPdf(8, basicRevision()))
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	stream, err := file.Object(4)
	if err != nil {
		t.Fatalf("object 4: %v", err)
	}
	if typed, ok := stream.(*object.Stream); !ok || string(typed.Data) != "hello" {
		t.Errorf("object 4 = %#v, want the stream with its indirect length resolved", stream)
	}

	tree, err := file.PageTree()
	if err != nil {
		t.Fatalf("page tree: %v", err)
	}
	if len(tree.Pages) != 2 || len(tree.InnerNode) != 2 || tree.Root.Number != 2 {
		t.Fatalf("page tree has %d pages and %d inner nodes under %d", len(tree.Pages), len(tree.InnerNode), tree.Root.Number)
	}

	testCases := []struct {
		name          string
		page          *Page
		wantNumber    int
		wantInherited []object.Name
	}{
		{name: "own media box", page: tree.Pages[0], wantNumber: 3, wantInherited: []object.Name{"Resources"}},
		{name: "everything from ancestors", page: tree.Pages[1], wantNumber: 7, wantInherited: []object.Name{"MediaBox", "Resources", "Rotate"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if testCase.page.Reference.Number != testCase.wantNumber {
				t.Errorf("%s: page %d, want %d", testCase.name, testCase.page.Reference.Number, testCase.wantNumber)
			}
			if got := slices.Sorted(maps.Keys(testCase.page.Inherited)); !slices.Equal(got, testCase.wantInherited) {
				t.Errorf("%s: inherited %v, want %v", testCase.name, got, testCase.wantInherited)
			}
		})
	}
	if rotate := tree.Pages[1].Inherited["Rotate"]; rotate != object.Integer(90) {
		t.Errorf("the nearest ancestor's /Rotate was not taken: %v", rotate)
	}
}

func TestRevisions(t *testing.T) {
	t.Parallel()

	data := buildPdf(
		9,
		basicRevision(),
		&revision{objects: map[int]string{3: "<< /Type /Page /Parent 2 0 R /Changed true >>"}, freed: []int{4}},
	)
	if _, err := Open(data); err != nil {
		t.Fatalf("open: %v", err)
	}

	testCases := []struct {
		name   string
		number int
		check  func(object.Object) bool
	}{
		{name: "the newer revision wins", number: 3, check: func(value object.Object) bool {
			dictionary, ok := value.(object.Dictionary)
			return ok && dictionary["Changed"] == object.Boolean(true)
		}},
		{name: "a freed object reads as null", number: 4, check: func(value object.Object) bool { return value == nil }},
		{name: "an absent object reads as null", number: 42, check: func(value object.Object) bool { return value == nil }},
		{name: "an older object is still there", number: 7, check: func(value object.Object) bool { return value != nil }},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			// Each case reads its own copy: a File caches what it reads and is not safe for concurrent use.
			file, err := Open(data)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			value, err := file.Object(testCase.number)
			if err != nil || !testCase.check(value) {
				t.Errorf("%s: object %d = %#v, %v", testCase.name, testCase.number, value, err)
			}
		})
	}
	file, err := Open(data)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if file.Size() != 9 {
		t.Errorf("size %d, want 9", file.Size())
	}
}

func TestOpenRefuses(t *testing.T) {
	t.Parallel()

	good := buildPdf(8, basicRevision())
	xrefStream := []byte("%PDF-1.7\n1 0 obj\n<< /Type /XRef >>\nendobj\nstartxref\n9\n%%EOF\n")
	single := buildPdf(8, basicRevision())
	ownXref := bytes.LastIndex(single, []byte("\nxref\n")) + 1
	loop := bytes.Replace(single, []byte("/Root 1 0 R"), fmt.Appendf(nil, "/Root 1 0 R /Prev %d", ownXref), 1)

	testCases := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{name: "empty", data: nil},
		{name: "no header", data: good[1:], wantErr: object.ErrSyntax},
		{name: "a cross-reference stream", data: xrefStream, wantErr: ErrUnsupported},
		{name: "a hybrid file", data: buildPdf(8, &revision{objects: basicRevision().objects, trailer: "/XRefStm 5"}), wantErr: ErrUnsupported},
		{name: "encrypted", data: buildPdf(8, &revision{objects: basicRevision().objects, trailer: "/Encrypt 4 0 R"}), wantErr: ErrUnsupported},
		{name: "a prev that points at its own section", data: loop, wantErr: object.ErrSyntax},
		{name: "no startxref", data: []byte("%PDF-1.7\n"), wantErr: object.ErrSyntax},
		{name: "a size past the largest object number", data: buildPdf(9223372036854775807, basicRevision()), wantErr: object.ErrSyntax},
		{name: "a subsection past the largest object number", data: bytes.Replace(good, []byte("\n1 1\n"), []byte("\n1 9223372036854775807\n"), 1), wantErr: object.ErrSyntax},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := Open(testCase.data)
			if err == nil {
				t.Fatalf("%s: open succeeded", testCase.name)
			}
			if testCase.wantErr != nil && !errors.Is(err, testCase.wantErr) {
				t.Errorf("%s: open = %v, want %v", testCase.name, err, testCase.wantErr)
			}
		})
	}
}

func TestObjectRejectsMisplacedEntry(t *testing.T) {
	t.Parallel()

	rev := basicRevision()
	data := buildPdf(8, rev)
	// Point the entry for 3 at object 1's header.
	index := bytes.Index(data, []byte("3 1\n"))
	entryOffset := bytes.Index(data, []byte("1 0 obj"))
	copy(data[index+4:], fmt.Sprintf("%010d", entryOffset))

	file, err := Open(data)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := file.Object(3); !errors.Is(err, object.ErrSyntax) {
		t.Errorf("object 3 = %v, want a syntax error", err)
	}
}

// TestObjectRefusesSelfReference reads streams whose /Length leads back to an object still being read, which must fail rather than recurse.
func TestObjectRefusesSelfReference(t *testing.T) {
	t.Parallel()

	rev := basicRevision()
	rev.objects[8] = "<< /Length 8 0 R >>\nstream\nhello\nendstream"
	rev.objects[9] = "<< /Length 10 0 R >>\nstream\nhello\nendstream"
	rev.objects[10] = "<< /Length 9 0 R >>\nstream\nhello\nendstream"
	data := buildPdf(11, rev)

	testCases := []struct {
		name   string
		number int
	}{
		{name: "a stream that is its own length", number: 8},
		{name: "two streams that are each other's length", number: 9},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			file, err := Open(data)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			if _, err := file.Object(testCase.number); !errors.Is(err, object.ErrSyntax) {
				t.Errorf("%s: object = %v, want a syntax error", testCase.name, err)
			}
		})
	}
}

func TestResolveGeneration(t *testing.T) {
	t.Parallel()

	file, err := Open(buildPdf(8, basicRevision()))
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	testCases := []struct {
		name      string
		reference *object.Reference
		wantNil   bool
	}{
		{name: "the object's generation", reference: &object.Reference{Number: 3}},
		{name: "another generation reads as null", reference: &object.Reference{Number: 3, Generation: 5}, wantNil: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			value, err := file.Resolve(testCase.reference)
			if err != nil || (value == nil) != testCase.wantNil {
				t.Errorf("%s: resolve = %#v, %v", testCase.name, value, err)
			}
		})
	}
}

func TestResolveLoop(t *testing.T) {
	t.Parallel()

	rev := basicRevision()
	rev.objects[8] = "9 0 R"
	rev.objects[9] = "8 0 R"
	file, err := Open(buildPdf(10, rev))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := file.Resolve(&object.Reference{Number: 8}); !errors.Is(err, object.ErrSyntax) {
		t.Errorf("resolve = %v, want a syntax error", err)
	}
}

func TestUpdate(t *testing.T) {
	t.Parallel()

	base := buildPdf(12, basicRevision())
	file, err := Open(base)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	update := file.NewUpdate()
	added := update.NewObject()
	if added.Number < 12 {
		t.Fatalf("new object %d reuses a number below the trailer's /Size", added.Number)
	}
	if err := update.Set(added, &object.Stream{Dictionary: object.Dictionary{}, Data: []byte("data")}); err != nil {
		t.Fatalf("set stream: %v", err)
	}
	catalog, catalogReference, err := file.Catalog()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	catalog = catalog.Clone()
	catalog["Extra"] = added
	if err := update.Set(catalogReference, catalog); err != nil {
		t.Fatalf("set catalog: %v", err)
	}
	written, err := update.Write()
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !bytes.HasPrefix(written.Data, base) {
		t.Fatal("the update changed the original bytes")
	}

	reopened, err := Open(written.Data)
	if err != nil {
		t.Fatalf("open after write: %v", err)
	}
	newCatalog, _, err := reopened.Catalog()
	if err != nil {
		t.Fatalf("catalog after write: %v", err)
	}
	stream, err := reopened.Resolve(newCatalog["Extra"])
	if err != nil {
		t.Fatalf("resolve extra: %v", err)
	}
	if typed, ok := stream.(*object.Stream); !ok || string(typed.Data) != "data" {
		t.Errorf("extra = %#v", stream)
	}
	if reopened.Size() != added.Number+1 || reopened.Trailer["Prev"] == nil {
		t.Errorf("trailer %v", reopened.Trailer)
	}
	if tree, err := reopened.PageTree(); err != nil || len(tree.Pages) != 2 {
		t.Errorf("page tree after write: %v", err)
	}

	// Every new entry has to point at its object's header.
	header := fmt.Sprintf("%d 0 obj\n", added.Number)
	bodyOffset := written.BodyOffsets[added.Number]
	if got := string(written.Data[bodyOffset-len(header) : bodyOffset]); got != header {
		t.Errorf("%q precedes the body", got)
	}

	if _, err := reopened.NewUpdate().Write(); err == nil {
		t.Error("an empty update was written")
	}
}

func FuzzOpen(f *testing.F) {
	f.Add(buildPdf(8, basicRevision()))
	f.Add(buildPdf(9, basicRevision(), &revision{objects: map[int]string{3: "<< /Type /Page /Parent 2 0 R >>"}, freed: []int{4}}))
	for _, name := range []string{"../pages/testdata/document.pdf", "../pages/testdata/appendix.pdf"} {
		if data, err := os.ReadFile(name); err == nil {
			f.Add(data)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		file, err := Open(data)
		if err != nil {
			return
		}
		_, _ = file.PageTree()
		for number := range min(file.Size(), 4096) {
			_, _ = file.Object(number)
		}
	})
}
