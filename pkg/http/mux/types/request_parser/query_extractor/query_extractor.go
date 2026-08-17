package query_extractor

import (
	"errors"
	"fmt"
	"go/ast"
	"net/http"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"strconv"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/query_extractor/query_extractor_config"
	queryTag "github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/query_extractor/tag"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
	motmedelJsonTag "github.com/altshiftab/utils_go/pkg/json/types/tag"
	motmedelReflect "github.com/altshiftab/utils_go/pkg/reflect"
	motmedelReflectErrors "github.com/altshiftab/utils_go/pkg/reflect/errors"
)

var uuidRegexp = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var urlSchemeRegexp = regexp.MustCompile(`^https?://`)

// Static errors for unsupported struct definitions/field types. These represent
// programming errors (not client input), so they are kept distinct from
// motmedelErrors.ErrValidationError.
var (
	errUnsupportedFormat        = errors.New("unsupported format")
	errUnsupportedScalarKind    = errors.New("unsupported scalar kind")
	errPointerFieldNotSupported = errors.New("pointer field not supported")
)

func validateFormat(value string, format string) error {
	switch format {
	case "email":
		_, err := mail.ParseAddress(value)
		if err != nil {
			return fmt.Errorf("%w: invalid email format: %q", motmedelErrors.ErrValidationError, value)
		}
		return nil
	case "uuid":
		if !uuidRegexp.MatchString(value) {
			return fmt.Errorf("%w: invalid uuid format: %q", motmedelErrors.ErrValidationError, value)
		}
		return nil
	case "url":
		_, err := url.ParseRequestURI(value)
		if err != nil || !urlSchemeRegexp.MatchString(value) {
			return fmt.Errorf("%w: invalid url format: %q", motmedelErrors.ErrValidationError, value)
		}
		return nil
	default:
		return fmt.Errorf("%w: %q", errUnsupportedFormat, format)
	}
}

type Parser[T any] struct {
	config *query_extractor_config.Config
}

func (p *Parser[T]) Parse(request *http.Request) (T, *response_error.ResponseError) {
	var zero T

	tType := reflect.TypeOf((*T)(nil)).Elem()
	targetType := motmedelReflect.RemoveIndirection(tType)
	if targetType.Kind() != reflect.Struct {
		return zero, &response_error.ResponseError{
			ServerError: motmedelErrors.NewWithTrace(motmedelReflectErrors.ErrNotStruct),
		}
	}

	if request == nil {
		return zero, &response_error.ResponseError{
			ServerError: motmedelErrors.NewWithTrace(nil_error.New("request")),
		}
	}

	requestUrl := request.URL
	if requestUrl == nil {
		return zero, &response_error.ResponseError{
			ServerError: motmedelErrors.NewWithTrace(nil_error.New("request url")),
		}
	}

	query, err := url.ParseQuery(requestUrl.RawQuery)
	if err != nil {
		return zero, &response_error.ResponseError{
			ProblemDetail: problem_detail.New(
				http.StatusBadRequest,
				problem_detail_config.WithDetail("Malformed query."),
			),
		}
	}

	// Allocate a new value of type T (supports struct and *struct)
	var result reflect.Value
	if tType.Kind() == reflect.Pointer {
		result = reflect.New(targetType) // *struct
	} else {
		result = reflect.New(targetType).Elem() // struct
	}
	// structVal refers to the underlying struct to populate
	var structVal reflect.Value
	if result.Kind() == reflect.Pointer {
		structVal = result.Elem()
	} else {
		structVal = result
	}

	// Keep track of known parameters and errors
	known := map[string]struct{}{}
	var parseErrs []error

	// Helpers

	setScalar := func(v reflect.Value, s string) error {
		switch v.Kind() {
		case reflect.String:
			v.SetString(s)
			return nil
		case reflect.Bool:
			if s == "" {
				v.SetBool(true)
			} else {
				b, err := strconv.ParseBool(s)
				if err != nil {
					return fmt.Errorf("%w: invalid bool value: %q", motmedelErrors.ErrValidationError, s)
				}
				v.SetBool(b)
			}
			return nil
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			var bitSize int
			switch v.Kind() {
			case reflect.Int8:
				bitSize = 8
			case reflect.Int16:
				bitSize = 16
			case reflect.Int32:
				bitSize = 32
			case reflect.Int64:
				bitSize = 64
			default:
				bitSize = 0 // int
			}
			n, err := strconv.ParseInt(s, 10, bitSize)
			if err != nil {
				return fmt.Errorf("%w: invalid integer value: %q", motmedelErrors.ErrValidationError, s)
			}
			v.SetInt(n)
			return nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			var bitSize int
			switch v.Kind() {
			case reflect.Uint8:
				bitSize = 8
			case reflect.Uint16:
				bitSize = 16
			case reflect.Uint32:
				bitSize = 32
			case reflect.Uint64:
				bitSize = 64
			default:
				bitSize = 0 // uint
			}
			n, err := strconv.ParseUint(s, 10, bitSize)
			if err != nil {
				return fmt.Errorf("%w: invalid unsigned integer value: %q", motmedelErrors.ErrValidationError, s)
			}
			v.SetUint(n)
			return nil
		case reflect.Float32, reflect.Float64:
			bitSize := 64
			if v.Kind() == reflect.Float32 {
				bitSize = 32
			}
			f, err := strconv.ParseFloat(s, bitSize)
			if err != nil {
				return fmt.Errorf("%w: invalid float value: %q", motmedelErrors.ErrValidationError, s)
			}
			v.SetFloat(f)
			return nil
		default:
			return fmt.Errorf("%w: %s", errUnsupportedScalarKind, v.Kind())
		}
	}

	setFromValues := func(fieldVal reflect.Value, fieldType reflect.Type, identifier string, values []string) error {
		// Pointers are not supported for fields
		if fieldType.Kind() == reflect.Pointer {
			return fmt.Errorf("%w: parameter %s", errPointerFieldNotSupported, identifier)
		}

		switch fieldType.Kind() {
		case reflect.Slice:
			// Special case: []byte from a single string
			if fieldType.Elem().Kind() == reflect.Uint8 {
				if len(values) != 1 {
					return fmt.Errorf("%w: parameter %s expects a single value", motmedelErrors.ErrValidationError, identifier)
				}
				fieldVal.SetBytes([]byte(values[0]))
				return nil
			}
			slice := reflect.MakeSlice(fieldType, 0, len(values))
			for _, s := range values {
				elem := reflect.New(fieldType.Elem()).Elem()
				if err := setScalar(elem, s); err != nil {
					return fmt.Errorf("parameter %s: %w", identifier, err)
				}
				slice = reflect.Append(slice, elem)
			}
			fieldVal.Set(slice)
			return nil
		case reflect.Array:
			if len(values) != fieldType.Len() {
				return fmt.Errorf("%w: parameter %s expects %d values", motmedelErrors.ErrValidationError, identifier, fieldType.Len())
			}
			for i := range fieldType.Len() {
				elem := fieldVal.Index(i)
				if err := setScalar(elem, values[i]); err != nil {
					return fmt.Errorf("parameter %s: %w", identifier, err)
				}
			}
			return nil
		default:
			// Scalar
			if len(values) != 1 {
				return fmt.Errorf("%w: parameter %s expects a single value", motmedelErrors.ErrValidationError, identifier)
			}
			if err := setScalar(fieldVal, values[0]); err != nil {
				return fmt.Errorf("parameter %s: %w", identifier, err)
			}
			return nil
		}
	}

	for i := range targetType.NumField() {
		field := targetType.Field(i)

		identifier := field.Name

		if len(identifier) == 0 || !ast.IsExported(identifier) {
			continue
		}

		fieldType := field.Type
		fieldTypeKind := fieldType.Kind()

		if fieldTypeKind == reflect.Pointer {
			return zero, &response_error.ResponseError{
				ServerError: motmedelErrors.NewWithTrace(fmt.Errorf("%w: %s", errPointerFieldNotSupported, identifier)),
			}
		}

		optional := false
		var format string

		qt := queryTag.New(field.Tag.Get("query"))
		if qt != nil {
			if qt.Skip {
				continue
			}
			if name := qt.Name; name != "" {
				identifier = name
			}
			optional = qt.OmitEmpty || qt.OmitZero
			format = qt.Format
		} else {
			jsonTag := motmedelJsonTag.New(field.Tag.Get("json"))
			if jsonTag != nil {
				if jsonTag.Skip {
					continue
				}
				if name := jsonTag.Name; name != "" {
					identifier = name
				}
				optional = jsonTag.OmitEmpty || jsonTag.OmitZero
			}
		}

		known[identifier] = struct{}{}

		values, ok := query[identifier]
		if !ok {
			if optional {
				continue
			}
			parseErrs = append(parseErrs, fmt.Errorf("%w: missing parameter: %s", motmedelErrors.ErrValidationError, identifier))
			continue
		}

		if len(values) > 1 && fieldTypeKind != reflect.Slice && fieldTypeKind != reflect.Array {
			parseErrs = append(parseErrs, fmt.Errorf("%w: multiple values for parameter: %s", motmedelErrors.ErrValidationError, identifier))
			continue
		}

		if format != "" {
			for _, v := range values {
				if err := validateFormat(v, format); err != nil {
					parseErrs = append(parseErrs, fmt.Errorf("parameter %s: %w", identifier, err))
				}
			}
		}

		targetField := structVal.Field(i)
		if err := setFromValues(targetField, fieldType, identifier, values); err != nil {
			parseErrs = append(parseErrs, err)
			continue
		}
	}

	if !p.config.AllowAdditionalParameters {
		for key := range query {
			if _, ok := known[key]; !ok {
				parseErrs = append(parseErrs, fmt.Errorf("%w: unknown parameter: %s", motmedelErrors.ErrValidationError, key))
			}
		}
	}

	if len(parseErrs) > 0 {
		var errorStrings []string
		for _, err := range parseErrs {
			errorStrings = append(errorStrings, err.Error())
		}
		return zero, &response_error.ResponseError{
			ProblemDetail: problem_detail.New(
				http.StatusBadRequest,
				problem_detail_config.WithDetail("Bad query."),
				problem_detail_config.WithExtension(map[string]any{"errors": errorStrings}),
			),
		}
	}

	return result.Interface().(T), nil
}

func New[T any](options ...query_extractor_config.Option) *Parser[T] {
	return &Parser[T]{config: query_extractor_config.New(options...)}
}

// Empty is a reusable parser that accepts only requests without query parameters.
// Because struct{} has no fields and additional parameters are disallowed by
// default, any non-empty query produces a 400 response error, while an empty
// query yields struct{}{}. It holds no mutable state and is safe for concurrent use.
var Empty = New[struct{}]()
