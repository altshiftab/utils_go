package object

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		want     Object
		wantUsed int
		wantErr  bool
	}{
		{name: "null", input: "null", want: nil, wantUsed: 4},
		{name: "true", input: "true", want: Boolean(true), wantUsed: 4},
		{name: "integer", input: "-42 ", want: Integer(-42), wantUsed: 3},
		{name: "real", input: "+.5", want: Real(0.5), wantUsed: 3},
		{name: "real with trailing point", input: "4.", want: Real(4), wantUsed: 2},
		{name: "name with escape", input: "/A#20B", want: Name("A B"), wantUsed: 6},
		{name: "empty name", input: "/ ", want: Name(""), wantUsed: 1},
		{name: "literal escapes", input: `(a\(b\)\\\n\101\0533)`, want: &String{Value: []byte("a(b)\\\nA+3")}, wantUsed: 21},
		{name: "nested parentheses", input: "(a(b)c)", want: &String{Value: []byte("a(b)c")}, wantUsed: 7},
		{name: "line continuation", input: "(a\\\r\nb)", want: &String{Value: []byte("ab")}, wantUsed: 7},
		{name: "bare CRLF reads as LF", input: "(a\r\nb)", want: &String{Value: []byte("a\nb")}, wantUsed: 6},
		{name: "hex string", input: "<48 65 6C6C 6f>", want: &String{Value: []byte("Hello"), Hex: true}, wantUsed: 15},
		{name: "odd hex string", input: "<4>", want: &String{Value: []byte{0x40}, Hex: true}, wantUsed: 3},
		{name: "reference", input: "12 0 R", want: &Reference{Number: 12}, wantUsed: 6},
		{name: "integers that are not a reference", input: "[1 2 3]", want: Array{Integer(1), Integer(2), Integer(3)}, wantUsed: 7},
		{name: "reference in an array", input: "[1 0 R 2]", want: Array{&Reference{Number: 1}, Integer(2)}, wantUsed: 9},
		{name: "R prefix is not a reference", input: "[1 0 RG]", wantErr: true},
		{
			name:     "dictionary with comment",
			input:    "<</Type/Page % a comment\n/Kids[3 0 R]/Count 1>>",
			want:     Dictionary{"Type": Name("Page"), "Kids": Array{&Reference{Number: 3}}, "Count": Integer(1)},
			wantUsed: 47,
		},
		{name: "unterminated string", input: "(abc", wantErr: true},
		{name: "unterminated dictionary", input: "<</A 1", wantErr: true},
		{name: "key that is not a name", input: "<<1 2>>", wantErr: true},
		{name: "unknown keyword", input: "endobj", wantErr: true},
		{name: "bad hex digit", input: "<4G>", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, used, err := Parse([]byte(testCase.input))
			if testCase.wantErr {
				if !errors.Is(err, ErrSyntax) {
					t.Errorf("%s: parse = %v, %v, want a syntax error", testCase.name, got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: parse: %v", testCase.name, err)
			}
			if !reflect.DeepEqual(got, testCase.want) || used != testCase.wantUsed {
				t.Errorf("%s: parse = %#v (%d bytes), want %#v (%d bytes)", testCase.name, got, used, testCase.want, testCase.wantUsed)
			}
		})
	}
}

func TestParseDepth(t *testing.T) {
	t.Parallel()

	deep := make([]byte, 0, 2*maxDepth+10)
	for range maxDepth + 5 {
		deep = append(deep, '[')
	}
	if _, _, err := Parse(deep); !errors.Is(err, ErrSyntax) {
		t.Errorf("parse of deep nesting = %v, want a syntax error", err)
	}
}

// TestWriteRoundTrip writes each value and reads it back, so the writer emits nothing the reader would take differently.
func TestWriteRoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		value Object
		want  string
	}{
		{name: "null", value: nil, want: "null"},
		{name: "integer", value: Integer(-7), want: "-7"},
		{name: "real without exponent", value: Real(0.000001), want: "0.000001"},
		{name: "whole real keeps its point", value: Real(4), want: "4.0"},
		{name: "name with delimiters", value: Name("A B/C#"), want: "/A#20B#2FC#23"},
		{name: "string with delimiters", value: &String{Value: []byte("a(b)\\\n\x01é")}, want: `(a\(b\)\\\n\001\303\251)`},
		{name: "hex string", value: &String{Value: []byte{0xde, 0xad}, Hex: true}, want: "<DEAD>"},
		{name: "reference", value: &Reference{Number: 4, Generation: 1}, want: "4 1 R"},
		{
			name:  "dictionary is written sorted",
			value: Dictionary{"Type": Name("Sig"), "A": Array{Integer(1), Boolean(false), nil}, "Ref": &Reference{Number: 9}},
			want:  "<< /A [1 false null] /Ref 9 0 R /Type /Sig >>",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			written, err := Append(nil, testCase.value)
			if err != nil {
				t.Fatalf("%s: append: %v", testCase.name, err)
			}
			if string(written) != testCase.want {
				t.Errorf("%s: wrote %s, want %s", testCase.name, written, testCase.want)
			}
			read, _, err := Parse(written)
			if err != nil {
				t.Fatalf("%s: parse: %v", testCase.name, err)
			}
			if !reflect.DeepEqual(read, testCase.value) {
				t.Errorf("%s: read back %#v, want %#v", testCase.name, read, testCase.value)
			}
		})
	}
}

func TestAppendRefuses(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		value Object
	}{
		{name: "stream as a direct object", value: Array{&Stream{}}},
		{name: "not a pdf object", value: 3},
		{name: "infinity", value: Real(math.Inf(1))},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Append(nil, testCase.value); !errors.Is(err, ErrSyntax) {
				t.Errorf("%s: append = %v, want a syntax error", testCase.name, err)
			}
		})
	}
}

func TestParseIndirect(t *testing.T) {
	t.Parallel()

	indirectLength := func(length Object) (int, error) {
		if reference, ok := length.(*Reference); ok && reference.Number == 9 {
			return 5, nil
		}
		if integer, ok := length.(Integer); ok {
			return int(integer), nil
		}
		return 0, ErrSyntax
	}

	testCases := []struct {
		name    string
		input   string
		offset  int
		want    *Indirect
		wantErr bool
	}{
		{
			name:   "dictionary",
			input:  "xx 3 0 obj\n<< /Type /Page >>\nendobj\n",
			offset: 3,
			want:   &Indirect{Number: 3, Value: Dictionary{"Type": Name("Page")}},
		},
		{
			name:  "stream with CRLF",
			input: "5 0 obj <</Length 5>>stream\r\nhello\r\nendstream endobj",
			want:  &Indirect{Number: 5, Value: &Stream{Dictionary: Dictionary{"Length": Integer(5)}, Data: []byte("hello")}},
		},
		{
			name:  "stream with an indirect length",
			input: "6 2 obj <</Length 9 0 R>>\nstream\nhello\nendstream\nendobj",
			want:  &Indirect{Number: 6, Generation: 2, Value: &Stream{Dictionary: Dictionary{"Length": &Reference{Number: 9}}, Data: []byte("hello")}},
		},
		{name: "length past the data", input: "5 0 obj <</Length 50>>stream\nhello\nendstream endobj", wantErr: true},
		{name: "a length that would overflow the bounds check", input: "5 0 obj <</Length 9223372036854775807>>stream\nhello\nendstream endobj", wantErr: true},
		{name: "wrong length", input: "5 0 obj <</Length 3>>stream\nhello\nendstream endobj", wantErr: true},
		{name: "no endobj", input: "3 0 obj 1", wantErr: true},
		{name: "no obj keyword", input: "3 0 1", wantErr: true},
		{name: "offset outside", input: "3 0 obj 1 endobj", offset: 99, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseIndirect([]byte(testCase.input), testCase.offset, indirectLength)
			if testCase.wantErr {
				if err == nil {
					t.Errorf("%s: parse indirect succeeded", testCase.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: parse indirect: %v", testCase.name, err)
			}
			if !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("%s: parse indirect = %#v, want %#v", testCase.name, got, testCase.want)
			}
		})
	}
}

func TestAppendBody(t *testing.T) {
	t.Parallel()

	stream := &Stream{Dictionary: Dictionary{"Length": Integer(99), "Filter": Name("FlateDecode")}, Data: []byte("abc")}
	body, err := AppendBody(nil, stream)
	if err != nil {
		t.Fatalf("append body: %v", err)
	}
	if want := "<< /Filter /FlateDecode /Length 3 >>\nstream\nabc\nendstream"; string(body) != want {
		t.Errorf("wrote %q, want %q", body, want)
	}
	if stream.Dictionary["Length"] != Integer(99) {
		t.Error("append body changed the stream's own dictionary")
	}

	indirect, err := ParseIndirect(append(append([]byte("1 0 obj\n"), body...), "\nendobj"...), 0, nil)
	if err != nil {
		t.Fatalf("parse indirect: %v", err)
	}
	if read, ok := indirect.Value.(*Stream); !ok || string(read.Data) != "abc" {
		t.Errorf("read back %#v", indirect.Value)
	}
}

func FuzzParse(f *testing.F) {
	for _, seed := range []string{"<</Type/Page/Kids[3 0 R]>>", "(a\\(b)", "<4a>", "[1 2 R 3 0 R]", "/A#20", "-.5", "<<"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		value, used, err := Parse(data)
		if err != nil {
			return
		}
		if used < 0 || used > len(data) {
			t.Fatalf("parse used %d of %d bytes", used, len(data))
		}
		// What parses has to write, and read back the same.
		written, err := Append(nil, value)
		if err != nil {
			return
		}
		again, _, err := Parse(written)
		if err != nil {
			t.Fatalf("parse of %q, written from %q: %v", written, data, err)
		}
		rewritten, err := Append(nil, again)
		if err != nil || string(rewritten) != string(written) {
			t.Fatalf("%q wrote %q, then %q", data, written, rewritten)
		}
	})
}

func FuzzParseIndirect(f *testing.F) {
	for _, seed := range []string{"1 0 obj <</Length 5>>stream\nhello\nendstream endobj", "2 0 obj 1 endobj", "3 0 obj <</Length 3 0 R>>stream\nx\nendstream endobj"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseIndirect(data, 0, nil)
	})
}
