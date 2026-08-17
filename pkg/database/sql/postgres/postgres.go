// Package postgres reads PostgreSQL-specific values and errors through database/sql without
// requiring a driver dependency.
package postgres

import (
	"errors"
	"fmt"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

var ErrMalformedTextArray = errors.New("malformed text array")

// SqlStateUniqueViolation is the SQLSTATE code of unique constraint violations.
const SqlStateUniqueViolation = "23505"

// sqlStateError is implemented by driver errors that carry a PostgreSQL SQLSTATE code, notably
// pgx's pgconn.PgError.
type sqlStateError interface {
	error
	SQLState() string
}

// SqlState returns the PostgreSQL SQLSTATE code carried by err, or false when err carries none.
func SqlState(err error) (string, bool) {
	sqlErr, ok := errors.AsType[sqlStateError](err)
	if !ok {
		return "", false
	}

	return sqlErr.SQLState(), true
}

// TextArrayScanner scans a one-dimensional PostgreSQL text array in its output format
// ("{a,b,...}", elements double-quoted with backslash escaping when needed) into Target. A SQL
// NULL scans as nil; null array elements are rejected.
type TextArrayScanner struct {
	Target *[]string
}

func (s TextArrayScanner) Scan(value any) error {
	if s.Target == nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("%w: nil target", ErrMalformedTextArray))
	}

	var data string
	switch typedValue := value.(type) {
	case nil:
		*s.Target = nil
		return nil
	case []byte:
		data = string(typedValue)
	case string:
		data = typedValue
	default:
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: unsupported source type %T", ErrMalformedTextArray, value),
		)
	}

	elements, err := ParseTextArray(data)
	if err != nil {
		return altshiftErrors.New(fmt.Errorf("parse text array: %w", err), data)
	}

	*s.Target = elements
	return nil
}

// ParseTextArray parses the output format of a one-dimensional PostgreSQL text array.
func ParseTextArray(data string) ([]string, error) {
	// A non-default lower bound prefixes the array with its dimensions ("[0:1]={a,b}").
	if strings.HasPrefix(data, "[") {
		if _, rest, found := strings.Cut(data, "="); found {
			data = rest
		}
	}

	if len(data) < 2 || data[0] != '{' || data[len(data)-1] != '}' {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: missing braces", ErrMalformedTextArray))
	}
	data = data[1 : len(data)-1]

	if data == "" {
		return []string{}, nil
	}

	var elements []string
	for {
		if data == "" {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: empty element", ErrMalformedTextArray))
		}

		if data[0] == '"' {
			var builder strings.Builder
			i := 1
			closed := false
			for i < len(data) {
				switch data[i] {
				case '\\':
					if i+1 >= len(data) {
						return nil, altshiftErrors.NewWithTrace(
							fmt.Errorf("%w: trailing escape", ErrMalformedTextArray),
						)
					}
					builder.WriteByte(data[i+1])
					i += 2
				case '"':
					closed = true
				default:
					builder.WriteByte(data[i])
					i++
				}
				if closed {
					break
				}
			}
			if !closed {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: unterminated quoted element", ErrMalformedTextArray),
				)
			}

			elements = append(elements, builder.String())
			data = data[i+1:]
		} else {
			element := data
			if end := strings.IndexByte(data, ','); end != -1 {
				element = data[:end]
			}
			data = data[len(element):]

			if element == "" || strings.ContainsAny(element, "{}\"\\") {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: malformed unquoted element", ErrMalformedTextArray),
				)
			}
			if element == "NULL" {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: null element", ErrMalformedTextArray),
				)
			}

			elements = append(elements, element)
		}

		if data == "" {
			return elements, nil
		}
		if data[0] != ',' {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: missing element separator", ErrMalformedTextArray),
			)
		}
		data = data[1:]
	}
}
