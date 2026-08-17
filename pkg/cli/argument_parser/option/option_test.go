package option

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/testing/cmp"
)

var diffOpts = []cmp.Option{cmp.EquateEmpty()}

func TestConstructorMetadata(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		option       Option
		wantShort    string
		wantLong     string
		wantUsage    string
		wantRequired bool
		wantNargs    Nargs
		wantMetavar  string
	}{
		{
			name:         "int",
			option:       NewIntOption('i', "int", "An int option", true, nil),
			wantShort:    "i",
			wantLong:     "int",
			wantUsage:    "An int option",
			wantRequired: true,
			wantMetavar:  "INT",
		},
		{
			name:        "ints",
			option:      NewIntsOption('n', "numbers", "Ints", false, nil),
			wantShort:   "n",
			wantLong:    "numbers",
			wantUsage:   "Ints",
			wantNargs:   NargsAtLeastOne,
			wantMetavar: "INT",
		},
		{
			name:        "string",
			option:      NewStringOption('s', "str", "A string option", false, nil),
			wantShort:   "s",
			wantLong:    "str",
			wantUsage:   "A string option",
			wantMetavar: "STRING",
		},
		{
			name:        "strings",
			option:      NewStringsOption('a', "array", "Strings", false, nil),
			wantShort:   "a",
			wantLong:    "array",
			wantUsage:   "Strings",
			wantNargs:   NargsAtLeastOne,
			wantMetavar: "STRING",
		},
		{
			name:      "bool",
			option:    NewBoolOption('b', "bool", "A bool option", false, nil),
			wantShort: "b",
			wantLong:  "bool",
			wantUsage: "A bool option",
			wantNargs: NargsNone,
		},
		{
			name:      "counted",
			option:    NewCountedOption('v', "verbose", "Verbosity", false, nil),
			wantShort: "v",
			wantLong:  "verbose",
			wantUsage: "Verbosity",
			wantNargs: NargsNone,
		},
		{
			name:        "file",
			option:      NewFileOption('f', "file", "A file option", false, nil),
			wantShort:   "f",
			wantLong:    "file",
			wantUsage:   "A file option",
			wantMetavar: "FILE",
		},
		{
			// A zero short name must render as "", not "\x00"; otherwise every option without a
			// short name shares that name and they collide with one another.
			name:        "no short name",
			option:      NewStringOption(0, "only-long", "A long-only option", false, nil),
			wantShort:   "",
			wantLong:    "only-long",
			wantUsage:   "A long-only option",
			wantMetavar: "STRING",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.option.GetShortName(); got != testCase.wantShort {
				t.Errorf("GetShortName() = %q, want %q", got, testCase.wantShort)
			}
			if got := testCase.option.GetLongName(); got != testCase.wantLong {
				t.Errorf("GetLongName() = %q, want %q", got, testCase.wantLong)
			}
			if got := testCase.option.GetUsage(); got != testCase.wantUsage {
				t.Errorf("GetUsage() = %q, want %q", got, testCase.wantUsage)
			}
			if got := testCase.option.GetRequired(); got != testCase.wantRequired {
				t.Errorf("GetRequired() = %v, want %v", got, testCase.wantRequired)
			}
			if got := testCase.option.GetNargs(); got != testCase.wantNargs {
				t.Errorf("GetNargs() = %q, want %q", got, testCase.wantNargs)
			}
			if got := testCase.option.GetMetavar(); got != testCase.wantMetavar {
				t.Errorf("GetMetavar() = %q, want %q", got, testCase.wantMetavar)
			}
		})
	}
}

func TestNewFileOptionDefaultsToReadOnly(t *testing.T) {
	t.Parallel()

	fileOption := NewFileOption('f', "file", "A file option", false, nil)

	if fileOption.Flag != os.O_RDONLY {
		t.Errorf("Flag = %d, want %d", fileOption.Flag, os.O_RDONLY)
	}
	if fileOption.Mode != 0 {
		t.Errorf("Mode = %v, want 0", fileOption.Mode)
	}
}

func TestIntOptionSet(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		ins   []string
		want  int
		nilIn bool
		// wantErr is a non-nil sentinel when Set is expected to fail.
		wantErr bool
	}{
		{name: "single", ins: []string{"42"}, want: 42},
		{name: "negative", ins: []string{"-3"}, want: -3},
		{name: "last wins", ins: []string{"1", "2"}, want: 2},
		{name: "not a number", ins: []string{"abc"}, wantErr: true},
		{name: "nil value", ins: []string{"1"}, nilIn: true, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var value int
			intOption := NewIntOption('i', "int", "usage", false, &value)
			if testCase.nilIn {
				intOption.Value = nil
			}

			var err error
			for _, in := range testCase.ins {
				if err = intOption.Set(in); err != nil {
					break
				}
			}

			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				if testCase.nilIn {
					assertNilFieldError(t, err, "value")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if value != testCase.want {
				t.Errorf("value = %d, want %d", value, testCase.want)
			}
		})
	}
}

func TestIntsOptionSet(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		ins     []string
		want    []int
		nilIn   bool
		wantErr bool
	}{
		{name: "none", ins: nil, want: nil},
		{name: "several", ins: []string{"1", "2", "3"}, want: []int{1, 2, 3}},
		{name: "not a number", ins: []string{"1", "x"}, wantErr: true},
		{name: "nil value", ins: []string{"1"}, nilIn: true, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			value := make([]int, 0)
			intsOption := NewIntsOption('n', "numbers", "usage", false, &value)
			if testCase.nilIn {
				intsOption.Value = nil
			}

			var err error
			for _, in := range testCase.ins {
				if err = intsOption.Set(in); err != nil {
					break
				}
			}

			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				if testCase.nilIn {
					assertNilFieldError(t, err, "value")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(testCase.want, value, diffOpts...); diff != "" {
				t.Errorf("value mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestStringOptionSet(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		ins     []string
		want    string
		nilIn   bool
		wantErr bool
	}{
		{name: "single", ins: []string{"abc"}, want: "abc"},
		{name: "empty", ins: []string{""}, want: ""},
		{name: "last wins", ins: []string{"a", "b"}, want: "b"},
		{name: "nil value", ins: []string{"a"}, nilIn: true, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var value string
			stringOption := NewStringOption('s', "str", "usage", false, &value)
			if testCase.nilIn {
				stringOption.Value = nil
			}

			var err error
			for _, in := range testCase.ins {
				if err = stringOption.Set(in); err != nil {
					break
				}
			}

			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				assertNilFieldError(t, err, "value")
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if value != testCase.want {
				t.Errorf("value = %q, want %q", value, testCase.want)
			}
		})
	}
}

func TestStringsOptionSet(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		ins     []string
		want    []string
		nilIn   bool
		wantErr bool
	}{
		{name: "none", ins: nil, want: nil},
		{name: "several", ins: []string{"a", "b"}, want: []string{"a", "b"}},
		{name: "nil value", ins: []string{"a"}, nilIn: true, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			value := make([]string, 0)
			stringsOption := NewStringsOption('a', "array", "usage", false, &value)
			if testCase.nilIn {
				stringsOption.Value = nil
			}

			var err error
			for _, in := range testCase.ins {
				if err = stringsOption.Set(in); err != nil {
					break
				}
			}

			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				assertNilFieldError(t, err, "value")
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(testCase.want, value, diffOpts...); diff != "" {
				t.Errorf("value mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestBoolOptionSet(t *testing.T) {
	t.Parallel()

	t.Run("ignores its argument", func(t *testing.T) {
		t.Parallel()

		var value bool
		boolOption := NewBoolOption('b', "bool", "usage", false, &value)

		if err := boolOption.Set(""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !value {
			t.Error("value = false, want true")
		}

		if err := boolOption.Set("anything"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !value {
			t.Error("value = false, want true")
		}
	})

	t.Run("nil value", func(t *testing.T) {
		t.Parallel()

		boolOption := NewBoolOption('b', "bool", "usage", false, nil)

		err := boolOption.Set("")
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		assertNilFieldError(t, err, "value")
	})
}

func TestCountedOptionSet(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		times   int
		want    int
		nilIn   bool
		wantErr bool
	}{
		{name: "once", times: 1, want: 1},
		{name: "three times", times: 3, want: 3},
		{name: "nil count", times: 1, nilIn: true, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var count int
			countedOption := NewCountedOption('v', "verbose", "usage", false, &count)
			if testCase.nilIn {
				countedOption.Count = nil
			}

			var err error
			for range testCase.times {
				if err = countedOption.Set(""); err != nil {
					break
				}
			}

			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				assertNilFieldError(t, err, "count")
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if count != testCase.want {
				t.Errorf("count = %d, want %d", count, testCase.want)
			}
		})
	}
}

func TestFileOptionSet(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	existing := filepath.Join(directory, "existing.txt")
	if err := os.WriteFile(existing, []byte("contents"), 0o600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	t.Run("opens an existing file for reading", func(t *testing.T) {
		t.Parallel()

		var file os.File
		fileOption := NewFileOption('f', "file", "usage", false, &file)

		if err := fileOption.Set(existing); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Cleanup(func() { _ = file.Close() })

		contents, err := os.ReadFile(file.Name())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(contents) != "contents" {
			t.Errorf("contents = %q, want %q", contents, "contents")
		}
	})

	t.Run("reports a missing file", func(t *testing.T) {
		t.Parallel()

		var file os.File
		fileOption := NewFileOption('f', "file", "usage", false, &file)

		err := fileOption.Set(filepath.Join(directory, "missing.txt"))
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("error = %v, want os.ErrNotExist", err)
		}
	})

	t.Run("creates a file when the flag says so", func(t *testing.T) {
		t.Parallel()

		created := filepath.Join(directory, "created.txt")

		var file os.File
		fileOption := NewFileOptionExtra(
			'f',
			"file",
			"usage",
			false,
			os.O_CREATE|os.O_WRONLY,
			0o600,
			&file,
		)

		if err := fileOption.Set(created); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Cleanup(func() { _ = file.Close() })

		if _, err := os.Stat(created); err != nil {
			t.Errorf("expected the file to exist: %v", err)
		}
	})

	t.Run("nil file", func(t *testing.T) {
		t.Parallel()

		fileOption := NewFileOption('f', "file", "usage", false, nil)

		err := fileOption.Set(existing)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		assertNilFieldError(t, err, "file")
	})
}

func TestSetErrorReportsTheOffendingInput(t *testing.T) {
	t.Parallel()

	var value int
	intOption := NewIntOption('i', "int", "usage", false, &value)

	err := intOption.Set("not-a-number")

	var numError *strconv.NumError
	if !errors.As(err, &numError) {
		t.Fatalf("expected a *strconv.NumError, got %T: %v", err, err)
	}
	if numError.Num != "not-a-number" {
		t.Errorf("Num = %q, want %q", numError.Num, "not-a-number")
	}
}

// assertNilFieldError asserts that err carries a nil_error.Error naming field.
func assertNilFieldError(t *testing.T, err error, field string) {
	t.Helper()

	nilError, ok := errors.AsType[*nil_error.Error](err)
	if !ok {
		t.Fatalf("expected a *nil_error.Error, got %T: %v", err, err)
	}
	if nilError.Field != field {
		t.Errorf("Field = %q, want %q", nilError.Field, field)
	}
}

func TestWithChoices(t *testing.T) {
	t.Parallel()

	stringOption := WithChoices(NewStringOption('m', "mode", "Mode", false, nil), "fast", "slow")

	if diff := cmp.Diff([]string{"fast", "slow"}, stringOption.GetChoices(), diffOpts...); diff != "" {
		t.Errorf("choices mismatch (-expected +got):\n%s", diff)
	}

	// GetChoices is not part of Option, so this compiling at all shows the helper gives back the
	// concrete type rather than an interface.
	stringOption.Value = new(string)

	if got := NewIntOption('i', "int", "Int", false, nil).GetChoices(); len(got) != 0 {
		t.Errorf("an option declares no choices by default, got %v", got)
	}
}

func TestWithDefault(t *testing.T) {
	t.Parallel()

	intOption := WithDefault(NewIntOption('p', "port", "Port", false, nil), "8080")

	if got := intOption.GetDefault(); got == nil || *got != "8080" {
		t.Errorf("GetDefault() = %v, want a pointer to %q", got, "8080")
	}
	if got := NewIntOption('i', "int", "Int", false, nil).GetDefault(); got != nil {
		t.Errorf("an option declares no default by default, got %q", *got)
	}

	// A nil default is what distinguishes declaring none from defaulting to the empty string.
	empty := WithDefault(NewStringOption('e', "empty", "Empty", false, nil), "")
	if got := empty.GetDefault(); got == nil || *got != "" {
		t.Errorf("GetDefault() = %v, want a pointer to the empty string", got)
	}
}

func TestNormalizeChoice(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "plain", in: "7", want: "7"},
		{name: "leading zeroes", in: "007", want: "7"},
		{name: "explicit sign", in: "+7", want: "7"},
		{name: "negative", in: "-7", want: "-7"},
		{name: "not a number", in: "abc", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewIntOption('i', "int", "Int", false, nil).NormalizeChoice(testCase.in)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != testCase.want {
				t.Errorf("NormalizeChoice(%q) = %q, want %q", testCase.in, got, testCase.want)
			}
		})
	}
}

func TestWithOptionalArgument(t *testing.T) {
	t.Parallel()

	stringOption := WithOptionalArgument(NewStringOption('c', "color", "Colour", false, nil), "always")

	if got := stringOption.GetNargs(); got != NargsOptional {
		t.Errorf("GetNargs() = %q, want %q", got, NargsOptional)
	}
	if got := stringOption.GetConst(); got != "always" {
		t.Errorf("GetConst() = %q, want %q", got, "always")
	}

	// The metavar survives, so help can still say what the argument would be.
	if got := stringOption.GetMetavar(); got != "STRING" {
		t.Errorf("GetMetavar() = %q, want %q", got, "STRING")
	}
}

func TestSettersOnEveryOptionKind(t *testing.T) {
	t.Parallel()

	options := []Option{
		NewIntOption('i', "int", "usage", false, nil),
		NewIntsOption('n', "ints", "usage", false, nil),
		NewStringOption('s', "str", "usage", false, nil),
		NewStringsOption('a', "strs", "usage", false, nil),
		NewBoolOption('b', "bool", "usage", false, nil),
		NewCountedOption('v', "count", "usage", false, nil),
		NewFileOption('f', "file", "usage", false, nil),
	}

	for _, opt := range options {
		t.Run(opt.GetLongName(), func(t *testing.T) {
			t.Parallel()

			choicesProvider, ok := opt.(ChoicesProvider)
			if !ok {
				t.Fatalf("%T does not provide choices", opt)
			}
			if _, ok := opt.(DefaultProvider); !ok {
				t.Fatalf("%T does not provide a default", opt)
			}
			if _, ok := opt.(ConstProvider); !ok {
				t.Fatalf("%T does not provide a constant", opt)
			}

			if got := choicesProvider.GetChoices(); len(got) != 0 {
				t.Errorf("expected no choices by default, got %v", got)
			}
		})
	}
}
