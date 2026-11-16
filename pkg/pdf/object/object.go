// Package object is the PDF object syntax (ISO 32000-2 §7.3): the values a PDF file is made of, read from and written to their textual form.
package object

import (
	"fmt"
	"maps"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// ErrSyntax is text that is not PDF object syntax; it is a parse error, so an endpoint answers it as bad input.
var ErrSyntax = fmt.Errorf("pdf syntax error: %w", altshiftErrors.ErrParseError)

// Object is one of nil (null), Boolean, Integer, Real, Name, *String, Array, Dictionary, *Reference or *Stream.
type Object any

type Boolean bool

type Integer int64

type Real float64

type Name string

// String is a string object; Hex records whether it was written in hexadecimal, so it is written back the same way.
type String struct {
	Value []byte
	Hex   bool
}

type Array []Object

type Dictionary map[Name]Object

// Reference points at an indirect object.
type Reference struct {
	Number     int
	Generation int
}

// Stream is a dictionary and its data as stored, still encoded by whatever filters the dictionary names.
type Stream struct {
	Dictionary Dictionary
	Data       []byte
}

// Indirect is a numbered object as it stands in a file between "obj" and "endobj".
type Indirect struct {
	Number     int
	Generation int
	Value      Object
}

// Name returns the entry as a name, if it is one.
func (d Dictionary) Name(key Name) (Name, bool) {
	value, ok := d[key].(Name)
	return value, ok
}

// Integer returns the entry as an integer, if it is one.
func (d Dictionary) Integer(key Name) (Integer, bool) {
	value, ok := d[key].(Integer)
	return value, ok
}

// Reference returns the entry as a reference, if it is one.
func (d Dictionary) Reference(key Name) (*Reference, bool) {
	value, ok := d[key].(*Reference)
	return value, ok && value != nil
}

// Clone copies the dictionary itself, not the values in it; the copy of a nil dictionary is an empty one, ready to be written to.
func (d Dictionary) Clone() Dictionary {
	clone := make(Dictionary, len(d))
	maps.Copy(clone, d)
	return clone
}
