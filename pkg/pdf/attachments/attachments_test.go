package attachments

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/pdf/file"
	"github.com/altshiftab/utils_go/pkg/pdf/object"
)

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

func plainPdf() []byte {
	return buildPdf(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] >>",
	})
}

// withAttachment already embeds a file, through an indirect /Names, and lists it in /AF.
func withAttachment() []byte {
	return buildPdf(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /Names 4 0 R /AF [6 0 R] >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] >>",
		4: "<< /EmbeddedFiles 5 0 R >>",
		5: "<< /Names [(zeta.txt) 6 0 R] >>",
		6: "<< /Type /Filespec /F (zeta.txt) /UF (zeta.txt) /EF << /F 7 0 R >> >>",
		7: "<< /Type /EmbeddedFile /Length 3 >>\nstream\nold\nendstream",
	})
}

// embedded reads the embedded files back by name, with the data each holds and the relationship it declares.
func embedded(t *testing.T, data []byte) (map[string][]byte, map[string]object.Name, int) {
	t.Helper()
	document, err := file.Open(data)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	catalog, _, err := document.Catalog()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	names, err := document.ResolveDictionary(catalog["Names"])
	if err != nil {
		t.Fatalf("names: %v", err)
	}
	entries := map[string]object.Object{}
	if err := readNameTree(document, names["EmbeddedFiles"], entries, map[int]bool{}, 0); err != nil {
		t.Fatalf("read name tree: %v", err)
	}
	contents := map[string][]byte{}
	relationships := map[string]object.Name{}
	for key, value := range entries {
		specification, err := document.ResolveDictionary(value)
		if err != nil {
			t.Fatalf("specification: %v", err)
		}
		files, err := document.ResolveDictionary(specification["EF"])
		if err != nil {
			t.Fatalf("ef: %v", err)
		}
		stream, err := document.Resolve(files["F"])
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		typed, ok := stream.(*object.Stream)
		if !ok {
			t.Fatalf("%q: %T is not a stream", key, stream)
		}
		contents[key] = typed.Data
		relationships[key], _ = specification.Name("AFRelationship")
	}
	associated, err := document.ResolveArray(catalog["AF"])
	if err != nil {
		t.Fatalf("af: %v", err)
	}
	return contents, relationships, len(associated)
}

func TestAttach(t *testing.T) {
	t.Parallel()

	receipt := &File{Name: "signing-receipt.json", MimeType: "application/json", Relationship: RelationshipData, ModifiedAt: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC), Data: []byte(`{"version":1}`)}

	testCases := []struct {
		name              string
		document          []byte
		files             []*File
		wantContents      map[string]string
		wantRelationships map[string]object.Name
		wantAssociated    int
	}{
		{
			name:              "into a document with none",
			document:          plainPdf(),
			files:             []*File{receipt},
			wantContents:      map[string]string{"signing-receipt.json": `{"version":1}`},
			wantRelationships: map[string]object.Name{"signing-receipt.json": RelationshipData},
			wantAssociated:    1,
		},
		{
			name:     "beside a file already embedded",
			document: withAttachment(),
			files:    []*File{receipt, {Name: "notes.txt", Data: []byte("note")}},
			wantContents: map[string]string{
				"signing-receipt.json": `{"version":1}`,
				"notes.txt":            "note",
				"zeta.txt":             "old",
			},
			wantRelationships: map[string]object.Name{"signing-receipt.json": RelationshipData, "notes.txt": RelationshipUnspecified},
			wantAssociated:    3,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			attached, err := Attach(testCase.document, testCase.files...)
			if err != nil {
				t.Fatalf("%s: attach: %v", testCase.name, err)
			}
			if !bytes.HasPrefix(attached, testCase.document) {
				t.Fatalf("%s: the document's bytes are not a prefix", testCase.name)
			}
			contents, relationships, associated := embedded(t, attached)
			if len(contents) != len(testCase.wantContents) {
				t.Errorf("%s: %d embedded files, want %d", testCase.name, len(contents), len(testCase.wantContents))
			}
			for name, want := range testCase.wantContents {
				if got := string(contents[name]); got != want {
					t.Errorf("%s: %q holds %q, want %q", testCase.name, name, got, want)
				}
			}
			for name, want := range testCase.wantRelationships {
				if relationships[name] != want {
					t.Errorf("%s: %q relationship %q, want %q", testCase.name, name, relationships[name], want)
				}
			}
			if associated != testCase.wantAssociated {
				t.Errorf("%s: %d associated files, want %d", testCase.name, associated, testCase.wantAssociated)
			}
		})
	}
}

// TestAttachNonAsciiName keys the tree by the name as the file specification spells it, UTF-16 with its byte order mark.
func TestAttachNonAsciiName(t *testing.T) {
	t.Parallel()

	attached, err := Attach(plainPdf(), &File{Name: "kvitto-ö.json", Data: []byte("{}")})
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	contents, _, _ := embedded(t, attached)
	key := string(textString("kvitto-ö.json").Value)
	if !bytes.HasPrefix([]byte(key), []byte{0xfe, 0xff}) {
		t.Fatalf("key %x has no byte order mark", key)
	}
	if string(contents[key]) != "{}" {
		t.Errorf("embedded files %v", slices.Collect(maps.Keys(contents)))
	}
}

func TestAttachRefuses(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		document []byte
		files    []*File
		wantErr  error
	}{
		{name: "nothing to attach", document: plainPdf()},
		{name: "a name already embedded", document: withAttachment(), files: []*File{{Name: "zeta.txt", Data: []byte("x")}}, wantErr: ErrDuplicateName},
		{name: "a name given twice", document: plainPdf(), files: []*File{{Name: "a", Data: []byte("x")}, {Name: "a", Data: []byte("y")}}, wantErr: ErrDuplicateName},
		{name: "no name", document: plainPdf(), files: []*File{{Data: []byte("x")}}, wantErr: ErrInvalidFile},
		{name: "an unknown relationship", document: plainPdf(), files: []*File{{Name: "a", Relationship: "Friend", Data: []byte("x")}}, wantErr: ErrInvalidFile},
		{name: "not a pdf", document: []byte("hello"), files: []*File{{Name: "a", Data: []byte("x")}}, wantErr: object.ErrSyntax},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := Attach(testCase.document, testCase.files...)
			if err == nil {
				t.Fatalf("%s: attach succeeded", testCase.name)
			}
			if testCase.wantErr != nil && !errors.Is(err, testCase.wantErr) {
				t.Errorf("%s: attach = %v, want %v", testCase.name, err, testCase.wantErr)
			}
		})
	}
}
