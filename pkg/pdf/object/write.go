package object

import (
	"bytes"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

const hexDigits = "0123456789ABCDEF"

func appendName(dst []byte, name Name) []byte {
	dst = append(dst, '/')
	for i := range len(name) {
		c := name[i]
		if c < 0x21 || c > 0x7e || c == '#' || isDelimiter(c) {
			dst = append(dst, '#', hexDigits[c>>4], hexDigits[c&0xf])
			continue
		}
		dst = append(dst, c)
	}
	return dst
}

func appendString(dst []byte, value *String) []byte {
	if value.Hex {
		dst = append(dst, '<')
		for _, c := range value.Value {
			dst = append(dst, hexDigits[c>>4], hexDigits[c&0xf])
		}
		return append(dst, '>')
	}

	dst = append(dst, '(')
	for _, c := range value.Value {
		switch {
		case c == '(' || c == ')' || c == '\\':
			dst = append(dst, '\\', c)
		case c == '\n':
			dst = append(dst, '\\', 'n')
		case c == '\r':
			dst = append(dst, '\\', 'r')
		case c < 0x20 || c > 0x7e:
			dst = append(dst, '\\', '0'+c>>6, '0'+(c>>3)&7, '0'+c&7)
		default:
			dst = append(dst, c)
		}
	}
	return append(dst, ')')
}

// Append writes the object in PDF syntax; a stream, which can only be an indirect object, is refused here.
func Append(dst []byte, value Object) ([]byte, error) {
	switch typed := value.(type) {
	case nil:
		return append(dst, "null"...), nil
	case Boolean:
		return strconv.AppendBool(dst, bool(typed)), nil
	case Integer:
		return strconv.AppendInt(dst, int64(typed), 10), nil
	case Real:
		if math.IsNaN(float64(typed)) || math.IsInf(float64(typed), 0) {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: real %v has no PDF form", ErrSyntax, float64(typed)))
		}
		// PDF has no exponent notation, and a real needs its point, or it reads back as an integer.
		start := len(dst)
		dst = strconv.AppendFloat(dst, float64(typed)+0, 'f', -1, 64)
		if !bytes.ContainsRune(dst[start:], '.') {
			dst = append(dst, ".0"...)
		}
		return dst, nil
	case Name:
		return appendName(dst, typed), nil
	case *String:
		if typed == nil {
			return append(dst, "null"...), nil
		}
		return appendString(dst, typed), nil
	case *Reference:
		if typed == nil {
			return append(dst, "null"...), nil
		}
		return fmt.Appendf(dst, "%d %d R", typed.Number, typed.Generation), nil
	case Array:
		dst = append(dst, '[')
		for i, element := range typed {
			if i > 0 {
				dst = append(dst, ' ')
			}
			var err error
			if dst, err = Append(dst, element); err != nil {
				return nil, err
			}
		}
		return append(dst, ']'), nil
	case Dictionary:
		dst = append(dst, "<<"...)
		// Sorted, so that the same dictionary is always written the same way.
		for _, key := range slices.Sorted(maps.Keys(typed)) {
			dst = append(appendName(append(dst, ' '), key), ' ')
			var err error
			if dst, err = Append(dst, typed[key]); err != nil {
				return nil, err
			}
		}
		return append(dst, " >>"...), nil
	case *Stream:
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: a stream cannot be written as a direct object", ErrSyntax))
	default:
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %T is not a PDF object", ErrSyntax, value))
	}
}

// AppendBody writes what goes between "N G obj" and "endobj"; a stream's /Length is set to its data's.
func AppendBody(dst []byte, value Object) ([]byte, error) {
	stream, ok := value.(*Stream)
	if !ok {
		return Append(dst, value)
	}
	if stream == nil {
		return append(dst, "null"...), nil
	}

	dictionary := stream.Dictionary.Clone()
	if dictionary == nil {
		dictionary = Dictionary{}
	}
	dictionary["Length"] = Integer(len(stream.Data))
	dst, err := Append(dst, dictionary)
	if err != nil {
		return nil, err
	}
	dst = append(dst, "\nstream\n"...)
	dst = append(dst, stream.Data...)
	return append(dst, "\nendstream"...), nil
}
