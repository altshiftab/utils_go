// Command csvtojson converts CSV to JSON: one object per record, one object per line (NDJSON), keyed by the header
// record's column names in column order. Values are strings unless --parse-values is given.
package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	argumentParser "github.com/altshiftab/utils_go/pkg/cli/argument_parser"
	argumentParserErrors "github.com/altshiftab/utils_go/pkg/cli/argument_parser/errors"
	"github.com/altshiftab/utils_go/pkg/cli/argument_parser/option"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

const (
	programName = "csvtojson"

	exitClean = 0
	exitError = 1
	exitUsage = 2

	byteOrderMark = "\uFEFF"
)

// numberPattern is the JSON number grammar. A value such as "007" or "1." does not match it and stays a string.
var numberPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

type arguments struct {
	input       string
	output      string
	delimiter   string
	snakeCase   bool
	parseValues bool
}

type conversion struct {
	delimiter   rune
	snakeCase   bool
	parseValues bool
}

func newParser(args *arguments, output io.Writer) *argumentParser.Parser {
	return &argumentParser.Parser{
		ProgramName: programName,
		Description: "Convert CSV to JSON, one object per record and per line, keyed by the header record's column names.",
		Output:      output,
		Options: []option.Option{
			option.WithMetavar(
				option.WithDefault(
					option.NewStringOption('d', "delimiter", `the field delimiter, one character; \t for a tab`, false, &args.delimiter),
					",",
				),
				"CHAR",
			),
			option.NewBoolOption('s', "snake-case", `write the column names in snake_case: "Order ID", "orderId" and "OrderID" all become order_id`, false, &args.snakeCase),
			option.NewBoolOption('p', "parse-values", "write values that are JSON numbers, true, false or null as such, rather than as strings", false, &args.parseValues),
			option.WithMetavar(
				option.NewStringOption('o', "output", "the file to write to, replaced only once the conversion has succeeded, as a go:generate line needs in place of a redirection (default: standard output)", false, &args.output),
				"PATH",
			),
		},
		Positionals: []option.Option{
			option.WithNargs(
				option.WithMetavar(option.NewStringOption(0, "", "the CSV file to read (default: standard input)", false, &args.input), "FILE"),
				option.NargsOptional,
			),
		},
		DisableAbbrev: true,
	}
}

// parseDelimiter reads the --delimiter argument: one character, or \t for a tab.
func parseDelimiter(delimiter string) (rune, error) {
	if delimiter == `\t` {
		return '\t', nil
	}

	r, size := utf8.DecodeRuneInString(delimiter)
	if size == 0 || size != len(delimiter) || r == utf8.RuneError || r == '"' || r == '\r' || r == '\n' {
		return 0, fmt.Errorf("%w: the delimiter must be one character other than a quote or a line break: %q", altshiftErrors.ErrValidationError, delimiter)
	}

	return r, nil
}

// toSnakeCase lowercases a column name and joins its words with underscores. Words are separated by anything other
// than a letter or a digit, and by the case changes of camelCase and PascalCase: "Order ID", "order-id", "orderId"
// and "OrderID" all become order_id, and "HTTPStatus" becomes http_status.
func toSnakeCase(name string) string {
	runes := []rune(name)

	var builder strings.Builder
	separate := false
	for i, r := range runes {
		if unicode.IsMark(r) {
			// A combining mark belongs to the letter before it, as the ring of a decomposed å does.
			builder.WriteRune(r)
			continue
		}

		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			separate = builder.Len() > 0
			continue
		}

		if i > 0 && unicode.IsUpper(r) {
			previous := runes[i-1]
			nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(previous) || unicode.IsDigit(previous) || (unicode.IsUpper(previous) && nextIsLower) {
				separate = builder.Len() > 0
			}
		}

		if separate {
			builder.WriteByte('_')
			separate = false
		}
		builder.WriteRune(unicode.ToLower(r))
	}

	return builder.String()
}

// makeKeys returns the object keys for the header record's column names, refusing two columns that would share a key.
func makeKeys(header []string, snakeCase bool) ([]string, error) {
	keys := make([]string, len(header))
	columns := make(map[string]int, len(header))
	for i, name := range header {
		key := name
		if snakeCase {
			key = toSnakeCase(name)
		}

		if column, ok := columns[key]; ok {
			return nil, fmt.Errorf(
				"%w: columns %d and %d would both be written as %q",
				altshiftErrors.ErrValidationError, column+1, i+1, key,
			)
		}
		columns[key] = i
		keys[i] = key
	}

	return keys, nil
}

func writeRecord(encoder *jsontext.Encoder, keys []string, record []string, parseValues bool) error {
	if err := encoder.WriteToken(jsontext.BeginObject); err != nil {
		return fmt.Errorf("encoder write token (begin object): %w", err)
	}

	for i, value := range record {
		if err := encoder.WriteToken(jsontext.String(keys[i])); err != nil {
			return fmt.Errorf("encoder write token (key): %w", err)
		}

		if parseValues && (value == "true" || value == "false" || value == "null" || numberPattern.MatchString(value)) {
			if err := encoder.WriteValue(jsontext.Value(value)); err != nil {
				return fmt.Errorf("encoder write value: %w", err)
			}
			continue
		}

		if err := encoder.WriteToken(jsontext.String(value)); err != nil {
			return fmt.Errorf("encoder write token (value): %w", err)
		}
	}

	if err := encoder.WriteToken(jsontext.EndObject); err != nil {
		return fmt.Errorf("encoder write token (end object): %w", err)
	}

	return nil
}

// convert writes a JSON object for each record of the CSV input after its header record. Every record must have as
// many fields as the header record. A quoted field may span lines, and a byte order mark opening the input is skipped.
func convert(input io.Reader, output io.Writer, conversion *conversion) error {
	bufferedInput := bufio.NewReader(input)
	if prefix, err := bufferedInput.Peek(len(byteOrderMark)); err == nil && string(prefix) == byteOrderMark {
		_, _ = bufferedInput.Discard(len(byteOrderMark))
	}

	reader := csv.NewReader(bufferedInput)
	reader.Comma = conversion.delimiter
	reader.LazyQuotes = true
	reader.ReuseRecord = true

	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return altshiftErrors.NewWithTrace(fmt.Errorf("%w: csv reader read (header): %w", altshiftErrors.ErrParseError, err))
	}

	keys, err := makeKeys(header, conversion.snakeCase)
	if err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("make keys: %w", err), header)
	}

	encoder := jsontext.NewEncoder(output)
	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return altshiftErrors.NewWithTrace(fmt.Errorf("%w: csv reader read: %w", altshiftErrors.ErrParseError, err))
		}

		if err := writeRecord(encoder, keys, record, conversion.parseValues); err != nil {
			line, _ := reader.FieldPos(0)
			return altshiftErrors.NewWithTrace(fmt.Errorf("write record (line %d): %w", line, err), record)
		}
	}
}

// run does what the command line asks and returns the exit code and, for a failure, the error to report.
func run(argv []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	args := &arguments{}
	parser := newParser(args, stdout)
	if err := parser.Validate(); err != nil {
		return exitError, altshiftErrors.New(fmt.Errorf("parser validate: %w", err))
	}
	if err := parser.ParseArgs(argv); err != nil {
		if errors.Is(err, argumentParserErrors.ErrHelp) || errors.Is(err, argumentParserErrors.ErrCompletion) {
			return exitClean, nil
		}
		fmt.Fprint(stderr, parser.FormatError(err))
		return exitUsage, nil
	}

	delimiter, err := parseDelimiter(args.delimiter)
	if err != nil {
		fmt.Fprint(stderr, parser.FormatError(err))
		return exitUsage, nil
	}

	input := stdin
	if args.input != "" && args.input != "-" {
		file, err := os.Open(args.input)
		if err != nil {
			return exitError, altshiftErrors.NewWithTrace(fmt.Errorf("os open: %w", err), args.input)
		}
		defer func() { _ = file.Close() }()
		input = file
	}

	conversion := &conversion{delimiter: delimiter, snakeCase: args.snakeCase, parseValues: args.parseValues}

	if args.output != "" {
		if err := convertToFile(input, args.output, conversion); err != nil {
			return exitError, fmt.Errorf("convert to file: %w", err)
		}
		return exitClean, nil
	}

	bufferedOutput := bufio.NewWriter(stdout)
	convertErr := convert(input, bufferedOutput, conversion)
	// The records converted before a failure are written all the same.
	if err := bufferedOutput.Flush(); err != nil && convertErr == nil {
		return exitError, altshiftErrors.NewWithTrace(fmt.Errorf("write stdout: %w", err))
	}
	if convertErr != nil {
		return exitError, fmt.Errorf("convert: %w", convertErr)
	}

	return exitClean, nil
}

// convertToFile writes the conversion to a temporary file beside path and moves it into place only once it is
// complete, so that a failed run leaves whatever was at path as it was.
func convertToFile(input io.Reader, path string, conversion *conversion) (err error) {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("os create temp: %w", err), path)
	}
	temporaryPath := file.Name()
	defer func() {
		if err != nil {
			_ = file.Close()
			_ = os.Remove(temporaryPath)
		}
	}()

	bufferedOutput := bufio.NewWriter(file)
	if err := convert(input, bufferedOutput, conversion); err != nil {
		return fmt.Errorf("convert: %w", err)
	}
	if err := bufferedOutput.Flush(); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("bufio writer flush: %w", err), temporaryPath)
	}
	if err := file.Close(); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("file close: %w", err), temporaryPath)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("os rename: %w", err), temporaryPath, path)
	}

	return nil
}

func main() {
	code, err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: error: %v\n", programName, err)
	}
	os.Exit(code)
}
