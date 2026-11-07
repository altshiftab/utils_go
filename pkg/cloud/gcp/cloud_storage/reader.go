package cloud_storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/reader_config"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

// Reader reads an object's bytes as they arrive, rather than all at once as
// DownloadObject does.
//
// A read broken off midway is resumed where it stopped, from the same object
// generation: what it yields is one generation's bytes or an error, never the
// start of one object followed by the rest of another written in its place.
type Reader struct {
	//nolint:containedctx // io.Reader takes no context; the reader's requests use the one it was opened with.
	ctx          context.Context
	client       *Client
	bucketName   string
	objectName   string
	fetchOptions []fetch_config.Option

	generation int64
	// offset is the next byte to read, counted from the object's start.
	offset int64
	// end is one past the last byte to read, or -1 until the object's size is
	// known for a read to its end.
	end    int64
	size   int64
	crc32c string
	body   io.ReadCloser

	reconnects    int
	maxReconnects int
}

// NewReader opens an object for reading, from the live generation unless
// reader_config.WithGeneration pins one, and over the whole object unless
// reader_config.WithRange narrows it.
func (c *Client) NewReader(
	ctx context.Context,
	bucketName string,
	objectName string,
	options ...reader_config.Option,
) (*Reader, error) {
	if bucketName == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("bucket name"))
	}
	if objectName == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("object name"))
	}

	config := reader_config.New(options...)
	if config.Offset < 0 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: offset %d is negative", altshiftErrors.ErrValidationError, config.Offset),
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	reader := &Reader{
		ctx:        ctx,
		client:     c,
		bucketName: bucketName,
		objectName: objectName,
		fetchOptions: slices.Concat(
			c.config.FetchOptions,
			config.FetchOptions,
			[]fetch_config.Option{
				fetch_config.WithSkipReadResponseBody(true),
				fetch_config.WithSkipErrorOnStatus(true),
				fetch_config.WithRetryConfig(nil),
			},
		),
		generation:    config.Generation,
		offset:        config.Offset,
		end:           -1,
		size:          -1,
		maxReconnects: config.MaxReconnects,
	}
	if config.Length > 0 {
		reader.end = config.Offset + config.Length
	}

	if err := reader.open(); err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	return reader, nil
}

// Generation is the generation being read.
func (r *Reader) Generation() int64 {
	return r.generation
}

// Size is the size of the whole object, whatever range is being read.
func (r *Reader) Size() int64 {
	return r.size
}

// Crc32c is the whole object's CRC32C, as Cloud Storage reported it.
func (r *Reader) Crc32c() string {
	return r.crc32c
}

func (r *Reader) Read(p []byte) (int, error) {
	for {
		if r.end >= 0 && r.offset >= r.end {
			return 0, io.EOF
		}

		if r.body == nil {
			if err := r.open(); err != nil {
				return 0, fmt.Errorf("open: %w", err)
			}
			continue
		}

		n, err := r.body.Read(p)
		r.offset += int64(n)
		if n > 0 || err == nil {
			return n, nil
		}

		r.closeBody()
		if r.offset >= r.end {
			return 0, io.EOF
		}

		// The response ended before the range did, cleanly or not.
		if r.reconnects >= r.maxReconnects {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return 0, altshiftErrors.NewWithTrace(
				fmt.Errorf("read broken off at byte %d after %d reconnects: %w", r.offset, r.reconnects, err),
				r.objectName,
			)
		}
		r.reconnects++

		if err := r.ctx.Err(); err != nil {
			return 0, fmt.Errorf("context err: %w", err)
		}
	}
}

func (r *Reader) Close() error {
	r.closeBody()
	return nil
}

func (r *Reader) closeBody() {
	if r.body != nil {
		_ = r.body.Close()
		r.body = nil
	}
}

// open requests the object from the current offset to the end of the range.
func (r *Reader) open() error {
	if r.end >= 0 && r.offset >= r.end {
		r.body = io.NopCloser(strings.NewReader(""))
		return nil
	}

	target := *r.client.baseUrl
	target.RawPath = target.Path + "b/" + url.PathEscape(r.bucketName) + "/o/" + url.PathEscape(r.objectName)
	target.Path += "b/" + r.bucketName + "/o/" + r.objectName
	query := url.Values{"alt": {"media"}}
	if r.generation != 0 {
		query.Set("generation", strconv.FormatInt(r.generation, 10))
	}
	target.RawQuery = query.Encode()
	urlString := target.String()

	request, err := http.NewRequestWithContext(r.ctx, http.MethodGet, urlString, http.NoBody)
	if err != nil {
		return altshiftErrors.NewWithTrace(fmt.Errorf("http new request with context: %w", err), urlString)
	}
	ranged := r.offset > 0 || r.end >= 0
	if ranged {
		if r.end >= 0 {
			request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", r.offset, r.end-1))
		} else {
			request.Header.Set("Range", fmt.Sprintf("bytes=%d-", r.offset))
		}
	}

	response, _, err := altshiftHttpUtils.FetchWithRequest(r.ctx, request, r.fetchOptions...)
	if err != nil {
		return fmt.Errorf("fetch with request: %w", err)
	}
	if response == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("http response"), urlString)
	}

	body := response.Body
	switch response.StatusCode {
	case http.StatusOK:
		// The whole object, whether or not a range was asked for.
		size, err := storedSize(response, -1)
		if err != nil {
			_ = body.Close()
			return fmt.Errorf("stored size: %w", err)
		}
		if size < 0 {
			_ = body.Close()
			return altshiftErrors.NewWithTrace(empty_error.New("object size"), urlString)
		}
		if err := r.adopt(response, size); err != nil {
			_ = body.Close()
			return fmt.Errorf("adopt: %w", err)
		}
		if r.offset > 0 {
			if _, err := io.CopyN(io.Discard, body, r.offset); err != nil {
				_ = body.Close()
				return altshiftErrors.NewWithTrace(fmt.Errorf("io copy n (skip to offset): %w", err), urlString)
			}
		}
	case http.StatusPartialContent:
		start, total, err := parseContentRange(response.Header.Get("Content-Range"))
		if err != nil {
			_ = body.Close()
			return fmt.Errorf("parse content range: %w", err)
		}
		if start != r.offset {
			_ = body.Close()
			return altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: range starts at %d, not at %d", altshiftErrors.ErrVerificationError, start, r.offset),
				urlString,
			)
		}
		if err := r.adopt(response, total); err != nil {
			_ = body.Close()
			return fmt.Errorf("adopt: %w", err)
		}
	case http.StatusRequestedRangeNotSatisfiable:
		_ = body.Close()
		_, total, err := parseContentRange(response.Header.Get("Content-Range"))
		if err != nil {
			return fmt.Errorf("parse content range: %w", err)
		}
		if r.offset != total {
			return altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: offset %d is past the end of an object of %d bytes", altshiftErrors.ErrValidationError, r.offset, total),
				urlString,
			)
		}
		if err := r.adopt(response, total); err != nil {
			return fmt.Errorf("adopt: %w", err)
		}
		r.body = io.NopCloser(strings.NewReader(""))
		return nil
	default:
		_ = body.Close()
		return altshiftErrors.NewWithTrace(
			&altshiftHttpErrors.Non2xxStatusCodeError{StatusCode: response.StatusCode},
			urlString,
		)
	}

	r.body = struct {
		io.Reader
		io.Closer
	}{io.LimitReader(body, r.end-r.offset), body}

	return nil
}

// adopt takes the object's attributes from a response, pinning the generation
// on the first and refusing any later one that is not the same object.
func (r *Reader) adopt(response *http.Response, size int64) error {
	generationHeader := response.Header.Get("X-Goog-Generation")
	generation, err := strconv.ParseInt(generationHeader, 10, 64)
	if err != nil {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: generation %q: strconv parse int: %w", altshiftErrors.ErrParseError, generationHeader, err),
		)
	}

	switch {
	case r.generation == 0:
		r.generation = generation
	case generation != r.generation:
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: generation %d was served for generation %d", altshiftErrors.ErrVerificationError, generation, r.generation),
			r.objectName,
		)
	}

	if r.size >= 0 && size != r.size {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: size %d was served for an object of %d bytes", altshiftErrors.ErrVerificationError, size, r.size),
			r.objectName,
		)
	}
	r.size = size
	if r.end < 0 || r.end > size {
		r.end = size
	}

	if checksum := crc32cFromHashHeaders(response.Header.Values("X-Goog-Hash")); checksum != "" {
		r.crc32c = checksum
	}

	return nil
}

// storedSize is the size of the object as stored, which is what a whole-object
// response carries unless Cloud Storage decompressed it on the way.
func storedSize(response *http.Response, fallback int64) (int64, error) {
	if stored := response.Header.Get("X-Goog-Stored-Content-Length"); stored != "" {
		size, err := strconv.ParseInt(stored, 10, 64)
		if err != nil {
			return 0, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: stored content length %q: strconv parse int: %w", altshiftErrors.ErrParseError, stored, err),
			)
		}
		return size, nil
	}
	if response.ContentLength >= 0 {
		return response.ContentLength, nil
	}

	return fallback, nil
}

// parseContentRange reads "bytes a-b/total", or "bytes */total" as a 416 sends
// it, into the first byte and the total size.
func parseContentRange(contentRange string) (int64, int64, error) {
	rest, found := strings.CutPrefix(contentRange, "bytes ")
	if !found {
		return 0, 0, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: content range %q", altshiftErrors.ErrParseError, contentRange),
		)
	}

	span, totalString, found := strings.Cut(rest, "/")
	if !found {
		return 0, 0, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: content range %q has no total", altshiftErrors.ErrParseError, contentRange),
		)
	}

	total, err := strconv.ParseInt(totalString, 10, 64)
	if err != nil {
		return 0, 0, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: content range %q: strconv parse int: %w", altshiftErrors.ErrParseError, contentRange, err),
		)
	}

	if span == "*" {
		return 0, total, nil
	}

	startString, _, found := strings.Cut(span, "-")
	if !found {
		return 0, 0, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: content range %q", altshiftErrors.ErrParseError, contentRange),
		)
	}
	start, err := strconv.ParseInt(startString, 10, 64)
	if err != nil {
		return 0, 0, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: content range %q: strconv parse int: %w", altshiftErrors.ErrParseError, contentRange, err),
		)
	}

	return start, total, nil
}

// crc32cFromHashHeaders picks the CRC32C out of X-Goog-Hash, which carries it
// beside the MD5 as "crc32c=...,md5=..." in one header or several.
func crc32cFromHashHeaders(values []string) string {
	for _, value := range values {
		for part := range strings.SplitSeq(value, ",") {
			if checksum, found := strings.CutPrefix(strings.TrimSpace(part), "crc32c="); found {
				return checksum
			}
		}
	}

	return ""
}
