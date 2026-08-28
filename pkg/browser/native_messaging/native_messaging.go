// Package native_messaging speaks the browsers' native messaging protocol: each
// message is a little-endian uint32 length followed by that many bytes of JSON,
// over the process's own standard input and output.
//
// Nothing else may be written to standard output. A stray print becomes a length
// prefix, and the browser then waits for a message that will never be the right
// size — which is why this package owns the stream, and why a native application's
// diagnostics belong in the journal instead.
package native_messaging

import (
	"encoding/binary"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// MaximumMessageBytes is the largest message the browsers accept in either
// direction. Firefox and Chrome both stop at a megabyte, so anything longer has
// to be sent in pieces.
const MaximumMessageBytes = 1 << 20

// ErrMessageTooLarge is a message over the browsers' limit.
//
// It is also what a length prefix that cannot be a real message reads as, and
// that is the point of checking it before allocating: the prefix is four bytes
// of untrusted input, a uint32 reaches four gigabytes, and a stream that has
// lost its framing produces exactly such a number.
var ErrMessageTooLarge = errors.New("message is too large")

// Read reads one message into message. It returns io.EOF once the browser has
// closed the port, which is how a native application is told to exit.
//
// Message contents are never attached to the errors returned here. A native
// application carries whatever the extension sends it — a page's text, a form's
// contents — and an error value is written to the journal, so putting the
// payload in one turns a parse failure into a disclosure.
func Read(reader io.Reader, message any) error {
	var length uint32
	if err := binary.Read(reader, binary.LittleEndian, &length); err != nil {
		// A port closed between messages and one closed part-way through a
		// prefix are both the browser having gone away, and both mean stop
		// rather than fail.
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return io.EOF
		}

		return altshiftErrors.NewWithTrace(fmt.Errorf("binary read (length): %w", err))
	}

	if length > MaximumMessageBytes {
		return altshiftErrors.NewWithTrace(fmt.Errorf("%w: %d bytes", ErrMessageTooLarge, length), length)
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("io read full: %w", err), length)
	}

	if err := json.Unmarshal(data, message); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal: %w", err), length)
	}

	return nil
}

// Write writes one message.
func Write(writer io.Writer, message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("json marshal: %w", err))
	}

	// Checked before the conversion below, which is what makes it safe: the
	// limit is far under what a uint32 holds.
	if len(data) > MaximumMessageBytes {
		return altshiftErrors.NewWithTrace(fmt.Errorf("%w: %d bytes", ErrMessageTooLarge, len(data)), len(data))
	}

	if err := binary.Write(writer, binary.LittleEndian, uint32(len(data))); err != nil { //nolint:gosec // Bounded above.
		return altshiftErrors.NewWithTrace(fmt.Errorf("binary write (length): %w", err), len(data))
	}

	if _, err := writer.Write(data); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("write: %w", err), len(data))
	}

	return nil
}
