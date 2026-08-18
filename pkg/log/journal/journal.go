// Package journal writes entries to the local systemd journal over its native
// protocol.
//
// The protocol is a datagram of "FIELD=value" lines sent to a Unix socket. A
// value containing a newline cannot be written that way, so it is sent as the
// field name on its own line, the value's length as a little-endian uint64, and
// then the value. A datagram too large for the socket buffer is instead written
// to an unlinked file whose descriptor is passed to journald out of band.
//
// See https://systemd.io/JOURNAL_NATIVE_PROTOCOL/ and
// https://www.freedesktop.org/software/systemd/man/systemd.journal-fields.html
package journal

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"syscall"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

// Priority is the syslog severity an entry is recorded with.
type Priority int

const (
	PriorityEmergency Priority = iota
	PriorityAlert
	PriorityCritical
	PriorityError
	PriorityWarning
	PriorityNotice
	PriorityInfo
	PriorityDebug
)

const (
	socketPath = "/run/systemd/journal/socket"
	// largeEntryDirectory holds the temporary file backing an entry too large to
	// fit in the socket buffer. It is a tmpfs, so the entry never reaches a disk.
	largeEntryDirectory = "/dev/shm"
)

var (
	connectionOnce sync.Once
	connection     *net.UnixConn
)

// getConnection returns the process-wide socket entries are sent from, opening
// it on first use. It is an unconnected socket: the destination is named on
// every write, so a journald restart does not strand it.
func getConnection() *net.UnixConn {
	connectionOnce.Do(func() {
		// An empty name asks the kernel to autobind an abstract address.
		address, err := net.ResolveUnixAddr("unixgram", "")
		if err != nil {
			return
		}

		socket, err := net.ListenUnixgram("unixgram", address)
		if err != nil {
			return
		}

		connection = socket
	})

	return connection
}

// Enabled reports whether the local journal is available to write to.
func Enabled() bool {
	if getConnection() == nil {
		return false
	}

	var dialer net.Dialer
	socket, err := dialer.DialContext(context.Background(), "unixgram", socketPath)
	if err != nil {
		return false
	}
	defer func() {
		_ = socket.Close()
	}()

	return true
}

// validateFieldName rejects a name journald would not accept: it must be
// non-empty, must not lead with an underscore (those are reserved for the
// fields journald sets itself), and may hold only capitals, digits and
// underscores.
func validateFieldName(name string) error {
	if name == "" {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: empty field name", altshiftErrors.ErrValidationError),
		)
	}

	if name[0] == '_' {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: field name begins with an underscore: %s", altshiftErrors.ErrValidationError, name),
			name,
		)
	}

	for _, character := range name {
		isUpper := character >= 'A' && character <= 'Z'
		isDigit := character >= '0' && character <= '9'
		if !isUpper && !isDigit && character != '_' {
			return altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: field name contains an invalid character: %s", altshiftErrors.ErrValidationError, name),
				name,
			)
		}
	}

	return nil
}

// appendField writes one field in whichever of the two framings the value needs.
func appendField(buffer *bytes.Buffer, name string, value string) error {
	if err := validateFieldName(name); err != nil {
		return fmt.Errorf("validate field name: %w", err)
	}

	if !bytes.ContainsRune([]byte(value), '\n') {
		fmt.Fprintf(buffer, "%s=%s\n", name, value)
		return nil
	}

	fmt.Fprintf(buffer, "%s\n", name)
	if err := binary.Write(buffer, binary.LittleEndian, uint64(len(value))); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("binary write: %w", err), name)
	}
	fmt.Fprintf(buffer, "%s\n", value)

	return nil
}

// isSocketSpaceError reports whether the write failed only because the entry
// was too large for the socket, which is recoverable by passing a descriptor.
func isSocketSpaceError(err error) bool {
	var opError *net.OpError
	if !errors.As(err, &opError) {
		return false
	}

	var syscallError *os.SyscallError
	if !errors.As(opError.Err, &syscallError) {
		return false
	}

	return errors.Is(syscallError.Err, syscall.EMSGSIZE) || errors.Is(syscallError.Err, syscall.ENOBUFS)
}

// newLargeEntryFile makes an unlinked file to hold an oversized entry. It is
// unlinked immediately, so it exists only for as long as a descriptor to it does.
func newLargeEntryFile() (*os.File, error) {
	file, err := os.CreateTemp(largeEntryDirectory, "journal-*")
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("os create temp: %w", err), largeEntryDirectory)
	}

	name := file.Name()
	if err := os.Remove(name); err != nil {
		_ = file.Close()
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("os remove: %w", err), name)
	}

	return file, nil
}

// Send writes one entry to the journal. Field names in variables must satisfy
// journald's rules; see validateFieldName. variables may be nil.
func Send(message string, priority Priority, variables map[string]string) error {
	socket := getConnection()
	if socket == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("journal socket"))
	}

	address := &net.UnixAddr{Name: socketPath, Net: "unixgram"}

	buffer := new(bytes.Buffer)
	if err := appendField(buffer, "PRIORITY", strconv.Itoa(int(priority))); err != nil {
		return fmt.Errorf("append field (priority): %w", err)
	}
	if err := appendField(buffer, "MESSAGE", message); err != nil {
		return fmt.Errorf("append field (message): %w", err)
	}
	for name, value := range variables {
		if err := appendField(buffer, name, value); err != nil {
			return fmt.Errorf("append field: %w", err)
		}
	}

	if _, _, err := socket.WriteMsgUnix(buffer.Bytes(), nil, address); err == nil {
		return nil
	} else if !isSocketSpaceError(err) {
		return altshiftErrors.NewWithTrace(fmt.Errorf("unix conn write msg unix: %w", err), socketPath)
	}

	// The entry did not fit in the socket buffer, so hand journald a descriptor
	// to read it from instead.
	file, err := newLargeEntryFile()
	if err != nil {
		return fmt.Errorf("new large entry file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	if _, err := io.Copy(file, buffer); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("io copy: %w", err))
	}

	rights := syscall.UnixRights(int(file.Fd()))
	if _, _, err := socket.WriteMsgUnix(nil, rights, address); err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("unix conn write msg unix (fd): %w", err), socketPath)
	}

	return nil
}
