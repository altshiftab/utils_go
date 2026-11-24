package image

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
)

// ErrPodmanFailed reports that podman exited with an error; the error carries podman's own message.
var ErrPodmanFailed = errors.New("podman failed")

// maxStderrSize bounds how much of podman's diagnostics is kept.
const maxStderrSize = 64 << 10

// Store reads images from the local podman store.
type Store struct {
	// Podman is the podman executable to run; "podman" (found in PATH) when empty.
	Podman string
}

// Save streams the image as a docker archive (`podman save --format docker-archive`). Closing the reader waits for
// podman and reports its failure, so a Close error tells why a truncated stream ended.
func (store *Store) Save(ctx context.Context, reference string) (io.ReadCloser, error) {
	if reference == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("reference"))
	}

	podman := store.executable()
	command := exec.CommandContext(ctx, podman, "save", "--format", "docker-archive", "--", reference)
	stderr := &boundedBuffer{limit: maxStderrSize}
	command.Stderr = stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("stdout pipe: %w", err))
	}
	if err := command.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("podman not found (%q): %w", podman, err), podman)
		}
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("command start: %w", err), podman, reference)
	}

	return &saveReader{ReadCloser: stdout, command: command, stderr: stderr}, nil
}

// Exists reports whether the image is in the store (`podman image exists`); nothing is pulled. Podman answers an
// image's absence with exit status 1 and any failure of its own with another, so only the former reads as false.
func (store *Store) Exists(ctx context.Context, reference string) (bool, error) {
	if reference == "" {
		return false, altshiftErrors.NewWithTrace(empty_error.New("reference"))
	}

	podman := store.executable()
	command := exec.CommandContext(ctx, podman, "image", "exists", "--", reference)
	stderr := &boundedBuffer{limit: maxStderrSize}
	command.Stderr = stderr
	err := command.Run()
	if err == nil {
		return true, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, fmt.Errorf("context: %w", ctxErr)
	}
	if errors.Is(err, exec.ErrNotFound) {
		return false, altshiftErrors.NewWithTrace(fmt.Errorf("podman not found (%q): %w", podman, err), podman)
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		if exitErr.ExitCode() == 1 {
			return false, nil
		}
		message := cmp.Or(strings.TrimSpace(stderr.String()), err.Error())
		return false, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s", ErrPodmanFailed, message), command.Args)
	}
	return false, altshiftErrors.NewWithTrace(fmt.Errorf("command run: %w", err), podman, reference)
}

// Analyze saves and analyzes an image from the store.
func (store *Store) Analyze(ctx context.Context, reference string) (*Analysis, error) {
	reader, err := store.Save(ctx, reference)
	if err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}

	analysis, analyzeErr := AnalyzeArchive(reader, reference)
	closeErr := reader.Close()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("context: %w", ctxErr)
	}
	// Podman's own failure (an unknown image, say) explains a broken stream better than the parse error the stream
	// caused; podman dying of the pipe we closed on a parse error explains nothing.
	if closeErr != nil && !errors.Is(closeErr, errPodmanSignaled) {
		if analyzeErr != nil {
			return nil, altshiftErrors.New(fmt.Errorf("%w (analyze archive: %w)", closeErr, analyzeErr), reference)
		}
		return nil, fmt.Errorf("close: %w", closeErr)
	}
	if analyzeErr != nil {
		return nil, fmt.Errorf("analyze archive: %w", analyzeErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close: %w", closeErr)
	}

	return analysis, nil
}

// executable is the podman to run.
func (store *Store) executable() string {
	if store == nil {
		return "podman"
	}
	return cmp.Or(store.Podman, "podman")
}

// errPodmanSignaled marks a podman that ended by a signal rather than by its own decision, e.g. from the pipe being
// closed under it.
var errPodmanSignaled = errors.New("podman ended by signal")

// saveReader is podman's stdout; closing it ends podman and reports how it exited.
type saveReader struct {
	io.ReadCloser
	command *exec.Cmd
	stderr  *boundedBuffer
	once    sync.Once
	err     error
}

func (reader *saveReader) Close() error {
	reader.once.Do(func() {
		// Closing the pipe first gives a still-running podman EPIPE, so Wait cannot hang.
		_ = reader.ReadCloser.Close()
		if err := reader.command.Wait(); err != nil {
			message := strings.TrimSpace(reader.stderr.String())
			if message == "" {
				message = err.Error()
			}
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && exitErr.ExitCode() == -1 {
				reader.err = fmt.Errorf("%w: %w: %s", ErrPodmanFailed, errPodmanSignaled, message)
				return
			}
			reader.err = altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s", ErrPodmanFailed, message), reader.command.Args)
		}
	})
	return reader.err
}

// boundedBuffer keeps the first limit bytes written to it.
type boundedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if remaining := b.limit - b.buffer.Len(); remaining > 0 {
		if len(p) > remaining {
			b.buffer.Write(p[:remaining])
		} else {
			b.buffer.Write(p)
		}
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	return b.buffer.String()
}
