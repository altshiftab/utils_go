package numeric_date

import (
	"encoding/json/v2"
	"fmt"
	"math"
	"strconv"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

var TimePrecision = time.Second

// Date represents a JSON numeric date value, as referenced at
// https://datatracker.ietf.org/doc/html/rfc7519#section-2.
type Date struct {
	time.Time
}

// MarshalJSON is an implementation of the json.RawMessage interface and serializes the UNIX epoch
// represented in NumericDate to a byte array, using the precision specified in TimePrecision.
func (date Date) MarshalJSON() ([]byte, error) {
	var prec int
	if TimePrecision < time.Second {
		prec = int(math.Log10(float64(time.Second) / float64(TimePrecision)))
	}
	truncatedDate := date.Truncate(TimePrecision)

	// For very large timestamps, UnixNano would overflow an int64, but this
	// function requires nanosecond level precision, so we have to use the
	// following technique to get round the issue:
	//
	// 1. Take the normal unix timestamp to form the whole number part of the
	//    output,
	// 2. Take the result of the Nanosecond function, which returns the offset
	//    within the second of the particular unix time instance, to form the
	//    decimal part of the output
	// 3. Concatenate them to produce the final result
	seconds := strconv.FormatInt(truncatedDate.Unix(), 10)
	nanosecondsOffset := strconv.FormatFloat(float64(truncatedDate.Nanosecond())/float64(time.Second), 'f', prec, 64)

	output := append([]byte(seconds), []byte(nanosecondsOffset)[1:]...)

	return output, nil
}

// UnmarshalJSON is an implementation of the json.RawMessage interface and
// deserializes a [NumericDate] from a JSON representation, i.e. a JSON
// number. This number represents a UNIX epoch with either integer or
// non-integer seconds.
func (date *Date) UnmarshalJSON(b []byte) error {
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal: %w", err), b)
	}

	n := NewFromSeconds(f)
	*date = *n

	return nil
}

func NewFromSeconds(f float64) *Date {
	round, frac := math.Modf(f)
	return New(time.Unix(int64(round), int64(frac*1e9)))
}

func New(t time.Time) *Date {
	return &Date{t.Truncate(TimePrecision)}
}

func Convert(value any) (*Date, error) {
	switch typedValue := value.(type) {
	case *Date:
		return typedValue, nil
	case Date:
		return &typedValue, nil
	case float64:
		if typedValue == 0 {
			return nil, nil
		}

		return NewFromSeconds(typedValue), nil
	default:
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %T", altshiftErrors.ErrUnexpectedType, typedValue),
			typedValue,
		)
	}
}
