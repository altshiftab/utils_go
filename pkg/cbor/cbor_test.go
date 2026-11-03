package cbor

import (
	"bytes"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
)

func TestEncodeKnownAnswers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		value    any
		expected string
	}{
		{
			// The COSE_KDF_Context from cose-wg vector p256-hkdf-256-01.
			name: "kdf context",
			value: []any{
				int64(1),
				[]any{nil, nil, nil},
				[]any{nil, nil, nil},
				[]any{uint64(128), []byte{0xa1, 0x01, 0x38, 0x18}},
			},
			expected: "840183f6f6f683f6f6f682188044a1013818",
		},
		{
			// The Enc_structure from cose-wg vector p256-hkdf-256-01.
			name:     "enc structure",
			value:    []any{"Encrypt", []byte{0xa1, 0x01, 0x01}, []byte{}},
			expected: "8367456e637279707443a1010140",
		},
		{
			// Bytewise key order: 1 (0x01), 4 (0x04), -1 (0x20).
			name:     "deterministic map key order",
			value:    map[int64]any{-1: int64(0), 4: int64(0), 1: int64(0)},
			expected: "a3010004002000",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			data, err := Encode(testCase.value)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}

			if encodedHex := hex.EncodeToString(data); encodedHex != testCase.expected {
				t.Errorf("encode: got %s, want %s", encodedHex, testCase.expected)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	value := Tag{
		Number: 96,
		Content: []any{
			[]byte{0xa1, 0x01, 0x03},
			map[any]any{
				int64(5):  make([]byte, 12),
				int64(-1): "negative label",
				"label":   Undefined{},
			},
			nil,
			[]any{int64(0), int64(23), int64(24), int64(255), int64(65536), int64(-1), int64(-25), true, false, "text"},
		},
	}

	data, err := Encode(value)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !reflect.DeepEqual(decoded, value) {
		t.Errorf("round trip: got %#v, want %#v", decoded, value)
	}
}

func TestDecodeRejects(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		data string
	}{
		{name: "empty", data: ""},
		{name: "indefinite array", data: "9f01ff"},
		{name: "indefinite byte string", data: "5f41004100ff"},
		{name: "half float", data: "f94100"},
		{name: "double float", data: "fb4028ae147ae147ae"},
		{name: "extended simple value", data: "f820"},
		{name: "reserved additional information", data: "1c"},
		{name: "duplicate map key", data: "a201000100"},
		{name: "byte string map key", data: "a1410000"},
		{name: "array map key", data: "a1800100"},
		{name: "trailing data", data: "0101"},
		{name: "truncated byte string", data: "58ff00"},
		{name: "truncated argument", data: "1b00"},
		{name: "array length exceeding data", data: "9b7fffffffffffffff"},
		{name: "map length exceeding data", data: "bb7fffffffffffffff"},
		{name: "unsupported positive integer", data: "1bffffffffffffffff"},
		{name: "unsupported negative integer", data: "3bffffffffffffffff"},
		{name: "invalid utf-8 text string", data: "62fffe"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			data, err := hex.DecodeString(testCase.data)
			if err != nil {
				t.Fatalf("decode hex: %v", err)
			}

			if _, err := Decode(data); !errors.Is(err, ErrMalformed) {
				t.Errorf("expected malformed error, got %v", err)
			}
		})
	}
}

func TestDecodeRejectsExcessiveNesting(t *testing.T) {
	t.Parallel()

	data := append(bytes.Repeat([]byte{0x81}, 40), 0x01)
	if _, err := Decode(data); !errors.Is(err, ErrMalformed) {
		t.Errorf("expected malformed error, got %v", err)
	}
}

func decodedByteString(t *testing.T, value any) []byte {
	t.Helper()

	array, ok := value.([]any)
	if !ok || len(array) != 2 {
		t.Fatalf("unexpected decoded value: %#v", value)
	}

	byteString, ok := array[0].([]byte)
	if !ok {
		t.Fatalf("unexpected first element: %#v", array[0])
	}

	return byteString
}

func TestDecodeCopies(t *testing.T) {
	t.Parallel()

	// ["abc" as bstr, "xy"]: 82, 43 616263, 62 7879
	data, err := hex.DecodeString("8243616263627879")
	if err != nil {
		t.Fatalf("decode hex: %v", err)
	}

	value, err := Decode(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	byteString := decodedByteString(t, value)
	data[2] = 'z'
	if byteString[0] != 'a' {
		t.Error("expected the decoded byte string to be a copy")
	}
}

func TestDecodeNoCopyAliases(t *testing.T) {
	t.Parallel()

	data, err := hex.DecodeString("8243616263627879")
	if err != nil {
		t.Fatalf("decode hex: %v", err)
	}

	value, err := DecodeNoCopy(data)
	if err != nil {
		t.Fatalf("decode no copy: %v", err)
	}

	byteString := decodedByteString(t, value)
	data[2] = 'z'
	if byteString[0] != 'z' {
		t.Error("expected the decoded byte string to alias the input")
	}

	// Appending must not overwrite the input bytes following the byte string.
	_ = append(byteString, 'Q')
	if data[5] != 0x62 {
		t.Error("expected append to a decoded byte string to not modify the input")
	}
}

func TestDecodeNoCopyMatchesDecode(t *testing.T) {
	t.Parallel()

	data, err := Encode(
		map[int64]any{
			1: []byte{1, 2, 3},
			2: "text",
			3: []any{int64(-25), Tag{Number: 96, Content: []byte{4, 5}}},
		},
	)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	value, err := Decode(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	noCopyValue, err := DecodeNoCopy(data)
	if err != nil {
		t.Fatalf("decode no copy: %v", err)
	}

	if !reflect.DeepEqual(value, noCopyValue) {
		t.Errorf("decode mismatch: %#v != %#v", value, noCopyValue)
	}
}

func TestEncodeRejectsUnsupportedType(t *testing.T) {
	t.Parallel()

	if _, err := Encode(1.5); !errors.Is(err, ErrUnsupportedValue) {
		t.Errorf("expected unsupported value error, got %v", err)
	}
}

func FuzzDecode(f *testing.F) {
	seeds := []string{
		// The COSE_Encrypt message from cose-wg vector p256-hkdf-256-01.
		"d8608443a10101a1054cc9cf4df2fe6c632bf788641358247adbe2709ca818fb415f1e5df66f4e1a51053ba6d65a1a0c52a357da7a644b8070a151b0818344a1013818a220a4010220012158219" +
			"8f50a4ff6c05861c8860d13a638ea56c3f5ad7590bbfbf054e1c7b4d91d6280225820f01400b089867804b8e9fc96c3932161f1934f4223069170d924b7e03bf822bb0458246d65726961646f632e6272616e64796275636b406275636b6c616e642e6578616d706c6540",
		"840183f6f6f683f6f6f682188044a1013818",
		"8367456e637279707443a1010140",
		"a3010004002000",
	}
	for _, seed := range seeds {
		seedData, err := hex.DecodeString(seed)
		if err != nil {
			f.Fatalf("decode seed hex: %v", err)
		}
		f.Add(seedData)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		value, err := Decode(data)
		if err != nil {
			if _, noCopyErr := DecodeNoCopy(data); noCopyErr == nil {
				t.Fatal("decode no copy succeeded where decode failed")
			}
			return
		}

		noCopyValue, err := DecodeNoCopy(data)
		if err != nil {
			t.Fatalf("decode no copy failed where decode succeeded: %v", err)
		}
		if !reflect.DeepEqual(noCopyValue, value) {
			t.Fatalf("decode no copy mismatch: %#v != %#v", noCopyValue, value)
		}

		reencoded, err := Encode(value)
		if err != nil {
			t.Fatalf("encode of decoded value: %v", err)
		}

		redecoded, err := Decode(reencoded)
		if err != nil {
			t.Fatalf("decode of reencoded data: %v", err)
		}

		if !reflect.DeepEqual(redecoded, value) {
			t.Fatalf("round trip mismatch: %#v != %#v", redecoded, value)
		}
	})
}

func TestDecodeFirst(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		data          string
		expectedValue any
		expectedRest  string
	}{
		{name: "integer without rest", data: "01", expectedValue: int64(1), expectedRest: ""},
		{name: "integer with rest", data: "01ffee", expectedValue: int64(1), expectedRest: "ffee"},
		{
			name:          "map with rest",
			data:          "a1010243aabbcc",
			expectedValue: map[any]any{int64(1): int64(2)},
			expectedRest:  "43aabbcc",
		},
		{
			name:          "byte string with rest",
			data:          "43010203ff",
			expectedValue: []byte{1, 2, 3},
			expectedRest:  "ff",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			data, err := hex.DecodeString(testCase.data)
			if err != nil {
				t.Fatalf("decode hex: %v", err)
			}

			value, rest, err := DecodeFirst(data)
			if err != nil {
				t.Fatalf("decode first: %v", err)
			}

			if !reflect.DeepEqual(value, testCase.expectedValue) {
				t.Errorf("value mismatch: %#v != %#v", value, testCase.expectedValue)
			}

			if hex.EncodeToString(rest) != testCase.expectedRest {
				t.Errorf("rest mismatch: %x != %s", rest, testCase.expectedRest)
			}
		})
	}
}

func TestDecodeFirstRejectsMalformed(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		data string
	}{
		{name: "empty", data: ""},
		{name: "truncated byte string", data: "58ff00"},
		{name: "indefinite array", data: "9f01ff"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			data, err := hex.DecodeString(testCase.data)
			if err != nil {
				t.Fatalf("decode hex: %v", err)
			}

			if _, _, err := DecodeFirst(data); !errors.Is(err, ErrMalformed) {
				t.Errorf("expected malformed error, got %v", err)
			}
		})
	}
}
