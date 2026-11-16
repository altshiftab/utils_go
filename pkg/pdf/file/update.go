package file

import (
	"bytes"
	"fmt"
	"maps"
	"slices"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/pdf/object"
)

type pending struct {
	generation int
	body       []byte
}

// Update is an incremental update being assembled over a file: new and replaced objects, written after every byte that is already there.
type Update struct {
	file    *File
	next    int
	objects map[int]*pending
}

// NewUpdate starts an update that numbers new objects past every number the file has used.
func (f *File) NewUpdate() *Update {
	return &Update{file: f, next: f.size, objects: map[int]*pending{}}
}

// NewObject reserves the next object number.
func (u *Update) NewObject() *object.Reference {
	reference := &object.Reference{Number: u.next}
	u.next++
	return reference
}

// Set writes the value as the object the reference names, replacing it if the file has one.
func (u *Update) Set(reference *object.Reference, value object.Object) error {
	if reference == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("reference"))
	}
	body, err := object.AppendBody(nil, value)
	if err != nil {
		return fmt.Errorf("object append body (%d): %w", reference.Number, err)
	}
	u.SetRaw(reference, body)
	return nil
}

// SetRaw writes the object's body, between "N G obj" and "endobj", as given; for a body whose byte positions the caller has to know.
func (u *Update) SetRaw(reference *object.Reference, body []byte) {
	u.objects[reference.Number] = &pending{generation: reference.Generation, body: body}
}

// Written is the file with the update appended, and where each object's body begins in it.
type Written struct {
	Data        []byte
	BodyOffsets map[int]int
}

// Write appends the objects, a classic cross-reference section for them and a trailer pointing back at the previous section.
func (u *Update) Write() (*Written, error) {
	if len(u.objects) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("update objects"))
	}
	if u.next-1 > MaxObjectNumber || slices.Max(slices.Collect(maps.Keys(u.objects))) > MaxObjectNumber {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: the update needs object numbers past %d", ErrUnsupported, MaxObjectNumber))
	}

	var out bytes.Buffer
	out.Write(u.file.Data)
	if !bytes.HasSuffix(u.file.Data, []byte("\n")) {
		out.WriteByte('\n')
	}

	type entry struct {
		number     int
		generation int
		offset     int
	}
	entries := make([]*entry, 0, len(u.objects))
	bodyOffsets := map[int]int{}
	for _, number := range slices.Sorted(maps.Keys(u.objects)) {
		value := u.objects[number]
		if value == nil {
			continue
		}
		entries = append(entries, &entry{number: number, generation: value.generation, offset: out.Len()})
		fmt.Fprintf(&out, "%d %d obj\n", number, value.generation)
		bodyOffsets[number] = out.Len()
		out.Write(value.body)
		out.WriteString("\nendobj\n")
	}

	xrefOffset := out.Len()
	out.WriteString("xref\n")
	for start := 0; start < len(entries); {
		end := start + 1
		for end < len(entries) && entries[end].number == entries[end-1].number+1 {
			end++
		}
		fmt.Fprintf(&out, "%d %d\n", entries[start].number, end-start)
		for _, item := range entries[start:end] {
			// Each entry is exactly 20 bytes, its end-of-line included.
			fmt.Fprintf(&out, "%010d %05d n\r\n", item.offset, item.generation)
		}
		start = end
	}

	// An update's trailer carries the previous one's entries, with its own /Size and a /Prev to the section before it.
	trailer := u.file.Trailer.Clone()
	trailer["Size"] = object.Integer(max(u.next, entries[len(entries)-1].number+1))
	trailer["Prev"] = object.Integer(u.file.lastXref)
	trailerBody, err := object.Append(nil, trailer)
	if err != nil {
		return nil, fmt.Errorf("object append (trailer): %w", err)
	}
	fmt.Fprintf(&out, "trailer\n%s\nstartxref\n%d\n%%%%EOF\n", trailerBody, xrefOffset)

	return &Written{Data: out.Bytes(), BodyOffsets: bodyOffsets}, nil
}
