package native_messaging

import (
	"bytes"
	"encoding/binary"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

type sample struct {
	Type string `json:"type"`
	Url  string `json:"url,omitzero"`
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		message *sample
	}{
		{name: "a plain message", message: &sample{Type: "correct", Url: "https://example.com/a"}},
		{name: "an empty field is omitted", message: &sample{Type: "done"}},
		{name: "non-ascii survives", message: &sample{Type: "progress", Url: "https://example.com/vår-höst"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			buffer := &bytes.Buffer{}
			if err := Write(buffer, testCase.message); err != nil {
				t.Fatalf("write: %v", err)
			}

			got := &sample{}
			if err := Read(buffer, got); err != nil {
				t.Fatalf("read: %v", err)
			}

			if *got != *testCase.message {
				t.Errorf("expected %+v, got %+v", testCase.message, got)
			}
		})
	}
}

// The browser frames every message with a little-endian uint32. Getting the
// order wrong produces a length in the gigabytes and a port that hangs, so it
// is asserted against the bytes rather than only against a round trip.
func TestWriteFramesLittleEndian(t *testing.T) {
	t.Parallel()

	buffer := &bytes.Buffer{}
	if err := Write(buffer, &sample{Type: "x"}); err != nil {
		t.Fatalf("write: %v", err)
	}

	data := buffer.Bytes()
	if len(data) < 4 {
		t.Fatalf("expected a length prefix, got %d bytes", len(data))
	}

	length := binary.LittleEndian.Uint32(data[:4])
	if int(length) != len(data)-4 {
		t.Errorf("expected a prefix of %d, got %d", len(data)-4, length)
	}
}

func TestReadSuccessiveMessages(t *testing.T) {
	t.Parallel()

	buffer := &bytes.Buffer{}
	for _, url := range []string{"https://example.com/a", "https://example.com/b"} {
		if err := Write(buffer, &sample{Type: "correct", Url: url}); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	for _, want := range []string{"https://example.com/a", "https://example.com/b"} {
		got := &sample{}
		if err := Read(buffer, got); err != nil {
			t.Fatalf("read: %v", err)
		}
		if got.Url != want {
			t.Errorf("expected %q, got %q", want, got.Url)
		}
	}
}

// A closed port is how the browser says to exit, so it has to be distinguishable
// from a stream that went wrong. Closing between messages and closing part-way
// through a length prefix are the same event to a native application, but they
// are different errors out of binary.Read, and reading only the first is how a
// clean browser shutdown comes to be reported as a failure.
func TestReadClosedPortIsEof(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input []byte
	}{
		{name: "closed between messages", input: nil},
		{name: "closed one byte into the prefix", input: []byte{0x01}},
		{name: "closed three bytes into the prefix", input: []byte{0x01, 0x00, 0x00}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if err := Read(bytes.NewReader(testCase.input), &sample{}); !errors.Is(err, io.EOF) {
				t.Errorf("expected io.EOF, got %v", err)
			}
		})
	}
}

func TestReadTruncatedBody(t *testing.T) {
	t.Parallel()

	framed := make([]byte, 4, 8)
	binary.LittleEndian.PutUint32(framed, 32)
	framed = append(framed, []byte(`{"type"`)...)

	err := Read(bytes.NewReader(framed), &sample{})
	if err == nil {
		t.Fatal("expected an error for a truncated body, got none")
	}
	if errors.Is(err, io.EOF) {
		t.Error("a truncated body is not a closed port and must not read as one")
	}
}

// The length prefix is four bytes of untrusted input. Refusing it before the
// allocation is the whole reason the check exists, so the test asserts that
// nothing was read for the body rather than only that an error came back.
func TestReadOversizedLengthIsRefused(t *testing.T) {
	t.Parallel()

	framed := make([]byte, 4)
	binary.LittleEndian.PutUint32(framed, MaximumMessageBytes+1)

	reader := bytes.NewReader(framed)

	err := Read(reader, &sample{})
	if !errors.Is(err, ErrMessageTooLarge) {
		t.Errorf("expected a too-large error, got %v", err)
	}
	if reader.Len() != 0 {
		t.Errorf("expected the prefix to have been consumed, %d bytes left", reader.Len())
	}
}

type oversized struct {
	Text string `json:"text"`
}

func TestWriteOversizedIsRefused(t *testing.T) {
	t.Parallel()

	message := &oversized{Text: string(make([]byte, MaximumMessageBytes+1))}

	buffer := &bytes.Buffer{}

	err := Write(buffer, message)
	if !errors.Is(err, ErrMessageTooLarge) {
		t.Errorf("expected a too-large error, got %v", err)
	}
	// A refused message must not have put a prefix on the stream: half a
	// message on the pipe desynchronises every message after it.
	if buffer.Len() != 0 {
		t.Errorf("expected nothing written, got %d bytes", buffer.Len())
	}
}

// A native application carries whatever the extension sends it, and an error
// value ends up in the journal. Message contents must therefore stay out of the
// errors this package returns.
//
// Three surfaces are checked. The text an error renders as is the obvious one,
// and the json packages put content in it in more ways than one: the input
// around a fault, and the keys on the way to it. altshiftErrors carries its
// values beside the message rather than in it — GetInput, not Error — so a
// payload attached as a value would be invisible to a test that only read the
// text. And a json error left in the chain carries the offending value in a
// field of its own, for anything that unwraps it.
func TestErrorsDoNotCarryMessageContents(t *testing.T) {
	t.Parallel()

	// Each body is malformed in its own way, so that the read fails, and holds
	// the secret where that failure would show it.
	const secret = "kontouppgifterna"

	type keyed struct {
		Counts map[string]int `json:"counts"`
	}

	testCases := []struct {
		name   string
		body   string
		target any
	}{
		{name: "text where a value goes", body: `{"type": ` + secret, target: &sample{}},
		{name: "a bad escape, quoted with what surrounds it", body: `{"type":"` + secret + `\ud83d","url":"` + secret + `"}`, target: &sample{}},
		{name: "a bad literal inside a secret key", body: `{"counts":{"` + secret + `": tru}}`, target: &keyed{}},
		{name: "a value of the wrong type under a secret key", body: `{"counts":{"` + secret + `":"` + secret + `"}}`, target: &keyed{}},
		{name: "a secret value of the wrong type", body: `{"type":["` + secret + `"]}`, target: &sample{}},
		{name: "text after the message", body: `{"type":"x"}` + secret, target: &sample{}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			framed := make([]byte, 4, 4+len(testCase.body))
			binary.LittleEndian.PutUint32(framed, uint32(len(testCase.body))) //nolint:gosec // A test literal, far under a uint32.
			framed = append(framed, testCase.body...)

			err := Read(bytes.NewReader(framed), testCase.target)
			if err == nil {
				t.Fatal("expected an error for a malformed body, got none")
			}

			checkCarriesNoContents(t, err, secret)

			if !errors.Is(err, altshiftErrors.ErrParseError) {
				t.Errorf("expected a parse error, so a caller can tell bad input from a broken stream: %v", err)
			}
			if !strings.Contains(err.Error(), "at byte") {
				t.Errorf("expected the error to say where the fault is: %v", err)
			}
		})
	}
}

func TestWriteErrorsDoNotCarryMessageContents(t *testing.T) {
	t.Parallel()

	const secret = "kontouppgifterna"

	testCases := []struct {
		name    string
		message any
	}{
		{name: "invalid UTF-8 in a value", message: &sample{Type: secret + "\xff"}},
		{name: "invalid UTF-8 in a key", message: map[string]string{secret + "\xff": secret}},
		{name: "a value JSON cannot hold, under a secret key", message: map[string]any{secret: func() {}}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			buffer := &bytes.Buffer{}
			err := Write(buffer, testCase.message)
			if err == nil {
				t.Fatal("expected an error, got none")
			}

			checkCarriesNoContents(t, err, secret)

			if !errors.Is(err, ErrUnencodable) {
				t.Errorf("expected an unencodable error: %v", err)
			}
			if buffer.Len() != 0 {
				t.Errorf("expected nothing written, got %d bytes", buffer.Len())
			}
		})
	}
}

func checkCarriesNoContents(t *testing.T, err error, secret string) {
	t.Helper()

	if strings.Contains(err.Error(), secret) {
		t.Errorf("the error's text repeated the message contents: %v", err)
	}

	if extendedErr, ok := errors.AsType[*altshiftErrors.ExtendedError](err); ok {
		if input := fmt.Sprint(extendedErr.GetInput()); strings.Contains(input, secret) {
			t.Errorf("the error's attached values repeated the message contents: %s", input)
		}
	}

	if _, ok := errors.AsType[*jsontext.SyntacticError](err); ok {
		t.Error("a syntactic error was left in the chain, with the input it quotes")
	}
	if _, ok := errors.AsType[*json.SemanticError](err); ok {
		t.Error("a semantic error was left in the chain, with the value it carries")
	}
}
