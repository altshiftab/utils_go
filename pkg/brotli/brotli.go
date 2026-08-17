// Package brotli wraps the Brotli implementation used for compression, keeping
// the rest of the module independent of the underlying library.
//
// The implementation is a vendored copy of github.com/andybalholm/brotli (the
// established pure-Go port of the reference implementation) under internal/,
// kept verbatim with its license so it stays diffable against upstream, which
// sees only a handful of changes per year. The upstream http.go (an HTTP
// compressor helper pulling a flate subpackage) is omitted. A compression codec is correctness
// critical, so it is wrapped rather than rewritten; only this facade is part
// of the module's API.
package brotli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/altshiftab/utils_go/pkg/brotli/internal/brotli"

	motmedelContext "github.com/altshiftab/utils_go/pkg/context"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func MakeBrotliData(ctx context.Context, data []byte) ([]byte, error) {
	var buffer bytes.Buffer
	quality := brotli.BestCompression
	brotliWriter := brotli.NewWriterLevel(&buffer, quality)

	// NOTE: Unlike a gzip writer, closing a brotli writer twice reports an
	// error, so the writer is only closed by the deferred function on early
	// returns.
	closed := false
	defer func() {
		if closed {
			return
		}
		if err := brotliWriter.Close(); err != nil {
			slog.WarnContext(
				motmedelContext.WithError(
					ctx,
					motmedelErrors.NewWithTrace(fmt.Errorf("brotli writer close: %w", err)),
				),
				"An error occurred when closing a brotli writer.",
			)
		}
	}()

	if _, err := brotliWriter.Write(data); err != nil {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf("brotli writer write: %w", err),
			quality,
		)
	}

	closed = true
	if err := brotliWriter.Close(); err != nil {
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("brotli writer close: %w", err))
	}

	return buffer.Bytes(), nil
}

// NewReader returns a reader that decompresses Brotli data from the provided
// reader.
func NewReader(reader io.Reader) io.Reader {
	return brotli.NewReader(reader)
}
