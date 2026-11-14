package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func TestToSnakeCase(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "words", input: "Order ID", expected: "order_id"},
		{name: "kebab", input: "order-id", expected: "order_id"},
		{name: "camel", input: "orderId", expected: "order_id"},
		{name: "pascal with initialism", input: "OrderID", expected: "order_id"},
		{name: "initialism first", input: "HTTPStatus", expected: "http_status"},
		{name: "already snake", input: "order_id", expected: "order_id"},
		{name: "punctuation", input: "Belopp (SEK)", expected: "belopp_sek"},
		{name: "repeated and surrounding separators", input: "  a -- b  ", expected: "a_b"},
		{name: "digit before capital", input: "item2Name", expected: "item2_name"},
		{name: "non-ascii letters", input: "Ändrad Är", expected: "ändrad_är"},
		{name: "combining mark kept with its letter", input: "Ma\u030al", expected: "ma\u030al"},
		{name: "nothing but separators", input: "#", expected: ""},
		{name: "empty", input: "", expected: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if actual := toSnakeCase(testCase.input); actual != testCase.expected {
				t.Errorf("toSnakeCase(%q) = %q, want %q", testCase.input, actual, testCase.expected)
			}
		})
	}
}

func TestParseDelimiter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected rune
		wantErr  bool
	}{
		{name: "comma", input: ",", expected: ','},
		{name: "semicolon", input: ";", expected: ';'},
		{name: "escaped tab", input: `\t`, expected: '\t'},
		{name: "literal tab", input: "\t", expected: '\t'},
		{name: "non-ascii", input: "§", expected: '§'},
		{name: "empty", input: "", wantErr: true},
		{name: "two characters", input: ",;", wantErr: true},
		{name: "quote", input: `"`, wantErr: true},
		{name: "newline", input: "\n", wantErr: true},
		{name: "invalid utf-8", input: "\xff", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			actual, err := parseDelimiter(testCase.input)
			if testCase.wantErr {
				if !errors.Is(err, altshiftErrors.ErrValidationError) {
					t.Fatalf("parseDelimiter(%q) error = %v, want a validation error", testCase.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDelimiter(%q) error = %v", testCase.input, err)
			}
			if actual != testCase.expected {
				t.Errorf("parseDelimiter(%q) = %q, want %q", testCase.input, actual, testCase.expected)
			}
		})
	}
}

func TestRun(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		args       []string
		stdin      string
		wantCode   int
		wantStdout string
		// wantErr is a part of the reported error; empty when none is expected.
		wantErr    string
		wantStderr string
	}{
		{
			name:       "strings in column order",
			stdin:      "b,a\n1,2\n3,4\n",
			wantStdout: `{"b":"1","a":"2"}` + "\n" + `{"b":"3","a":"4"}` + "\n",
		},
		{
			name:       "crlf line endings",
			stdin:      "a,b\r\n1,2\r\n",
			wantStdout: `{"a":"1","b":"2"}` + "\n",
		},
		{
			name:       "quoted field spanning lines",
			stdin:      "a,b\n\"one\r\ntwo\",3\n4,5\n",
			wantStdout: `{"a":"one\ntwo","b":"3"}` + "\n" + `{"a":"4","b":"5"}` + "\n",
		},
		{
			name:       "escaped and stray quotes",
			stdin:      "a,b\n\"say \"\"hi\"\"\",5\"6\n",
			wantStdout: `{"a":"say \"hi\"","b":"5\"6"}` + "\n",
		},
		{
			name:       "byte order mark skipped",
			stdin:      "\uFEFF\"a\",b\n1,2\n",
			wantStdout: `{"a":"1","b":"2"}` + "\n",
		},
		{
			name:       "blank lines skipped",
			stdin:      "a\n\n1\n\n2\n",
			wantStdout: `{"a":"1"}` + "\n" + `{"a":"2"}` + "\n",
		},
		{
			name:  "empty input",
			stdin: "",
		},
		{
			name:  "header only",
			stdin: "a,b\n",
		},
		{
			name:       "values stay strings by default",
			stdin:      "a,b,c\n1,true,null\n",
			wantStdout: `{"a":"1","b":"true","c":"null"}` + "\n",
		},
		{
			name:       "parse values",
			args:       []string{"--parse-values"},
			stdin:      "int,neg,float,exp,true,false,null,lead0,trail,word,empty,upper\n42,-7,12.50,1E+3,true,false,null,007,1.,yes,,TRUE\n",
			wantStdout: `{"int":42,"neg":-7,"float":12.50,"exp":1E+3,"true":true,"false":false,"null":null,"lead0":"007","trail":"1.","word":"yes","empty":"","upper":"TRUE"}` + "\n",
		},
		{
			name:       "snake case",
			args:       []string{"-s"},
			stdin:      "Order ID,customerName,E-post\n1,Ada,a@example.com\n",
			wantStdout: `{"order_id":"1","customer_name":"Ada","e_post":"a@example.com"}` + "\n",
		},
		{
			name:       "semicolon delimiter",
			args:       []string{"-d", ";"},
			stdin:      "a;b\n1,5;2\n",
			wantStdout: `{"a":"1,5","b":"2"}` + "\n",
		},
		{
			name:       "tab delimiter",
			args:       []string{`--delimiter=\t`},
			stdin:      "a\tb\n1\t2\n",
			wantStdout: `{"a":"1","b":"2"}` + "\n",
		},
		{
			name:       "stdin by dash",
			args:       []string{"-"},
			stdin:      "a\n1\n",
			wantStdout: `{"a":"1"}` + "\n",
		},
		{
			name:       "record with too many fields",
			stdin:      "a,b\n1,2\n1,2,3\n",
			wantCode:   exitError,
			wantStdout: `{"a":"1","b":"2"}` + "\n",
			wantErr:    "record on line 3: wrong number of fields",
		},
		{
			name:     "record with too few fields",
			stdin:    "a,b\n1\n",
			wantCode: exitError,
			wantErr:  "wrong number of fields",
		},
		{
			name:     "duplicate column names",
			stdin:    "a,b,a\n1,2,3\n",
			wantCode: exitError,
			wantErr:  `columns 1 and 3 would both be written as "a"`,
		},
		{
			name:     "column names colliding in snake case",
			args:     []string{"--snake-case"},
			stdin:    "Order ID,orderId\n1,2\n",
			wantCode: exitError,
			wantErr:  `columns 1 and 2 would both be written as "order_id"`,
		},
		{
			name:     "invalid utf-8",
			stdin:    "a\nok\n\xe5\n",
			wantCode: exitError,
			// The record before is written; the failing one is named by its line.
			wantStdout: `{"a":"ok"}` + "\n",
			wantErr:    "write record (line 3)",
		},
		{
			name:       "invalid delimiter",
			args:       []string{"-d", "ab"},
			stdin:      "a\n1\n",
			wantCode:   exitUsage,
			wantStderr: "the delimiter must be one character",
		},
		{
			name:       "unknown option",
			args:       []string{"--nope"},
			wantCode:   exitUsage,
			wantStderr: "name not found: nope",
		},
		{
			name:       "abbreviated option refused",
			args:       []string{"--snake"},
			wantCode:   exitUsage,
			wantStderr: "name not found: snake",
		},
		{
			name:     "missing input file",
			args:     []string{filepath.Join(t.TempDir(), "missing.csv")},
			wantCode: exitError,
			wantErr:  "os open",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			code, err := run(testCase.args, strings.NewReader(testCase.stdin), &stdout, &stderr)

			if code != testCase.wantCode {
				t.Errorf("code = %d, want %d (stderr %q, error %v)", code, testCase.wantCode, stderr.String(), err)
			}
			if stdout.String() != testCase.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), testCase.wantStdout)
			}
			switch {
			case testCase.wantErr == "" && err != nil:
				t.Errorf("error = %v, want none", err)
			case testCase.wantErr != "" && (err == nil || !strings.Contains(err.Error(), testCase.wantErr)):
				t.Errorf("error = %v, want one containing %q", err, testCase.wantErr)
			}
			if !strings.Contains(stderr.String(), testCase.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), testCase.wantStderr)
			}
		})
	}
}

func TestRunHelp(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code, err := run([]string{"--help"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitClean || err != nil {
		t.Fatalf("run(--help) = %d, %v", code, err)
	}
	for _, want := range []string{"Usage: csvtojson", "--snake-case", "--parse-values", "--output", "FILE"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("help %q lacks %q", stdout.String(), want)
		}
	}
}

func TestRunInputFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "in.csv")
	if err := os.WriteFile(path, []byte("a,b\n1,2\n"), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code, err := run([]string{path}, strings.NewReader("ignored\nstdin\n"), &stdout, &stderr)
	if code != exitClean || err != nil {
		t.Fatalf("run = %d, %v (stderr %q)", code, err, stderr.String())
	}
	if want := `{"a":"1","b":"2"}` + "\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunOutputFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		// existing is what is at the output path before the run; nil for nothing.
		existing []byte
		stdin    string
		wantCode int
		// wantContent is what is at the output path after the run; nil for nothing.
		wantContent []byte
	}{
		{
			name:        "new file",
			stdin:       "a\n1\n",
			wantContent: []byte(`{"a":"1"}` + "\n"),
		},
		{
			name:        "replaced file",
			existing:    []byte("old\n"),
			stdin:       "a\n1\n",
			wantContent: []byte(`{"a":"1"}` + "\n"),
		},
		{
			name:        "failed run leaves the old file",
			existing:    []byte("old\n"),
			stdin:       "a\n1\n1,2\n",
			wantCode:    exitError,
			wantContent: []byte("old\n"),
		},
		{
			name:     "failed run creates nothing",
			stdin:    "a,a\n1,2\n",
			wantCode: exitError,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			directory := t.TempDir()
			path := filepath.Join(directory, "out.json")
			if testCase.existing != nil {
				if err := os.WriteFile(path, testCase.existing, 0o600); err != nil {
					t.Fatalf("write existing: %v", err)
				}
			}

			var stdout, stderr bytes.Buffer
			code, err := run([]string{"-o", path}, strings.NewReader(testCase.stdin), &stdout, &stderr)
			if code != testCase.wantCode {
				t.Errorf("code = %d, want %d (error %v)", code, testCase.wantCode, err)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", stdout.String())
			}

			content, readErr := os.ReadFile(path)
			switch {
			case testCase.wantContent == nil && !errors.Is(readErr, os.ErrNotExist):
				t.Errorf("output file read = %q, %v; want none", content, readErr)
			case testCase.wantContent != nil && !bytes.Equal(content, testCase.wantContent):
				t.Errorf("output file = %q, %v; want %q", content, readErr, testCase.wantContent)
			}

			// No temporary file is left behind, whatever the outcome.
			entries, err := os.ReadDir(directory)
			if err != nil {
				t.Fatalf("read dir: %v", err)
			}
			for _, entry := range entries {
				if entry.Name() != "out.json" {
					t.Errorf("left behind %q", entry.Name())
				}
			}
		})
	}
}
