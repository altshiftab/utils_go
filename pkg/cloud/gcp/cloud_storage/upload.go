package cloud_storage

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/types/object"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/upload_config"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

// ErrUploadStalled is an upload that stopped getting any further: sessions that
// kept ending, or chunks that were taken and not kept.
var ErrUploadStalled = errors.New("upload stalled")

// UploadObject uploads size bytes read from body as an object, through a
// resumable upload session, and returns the object's metadata and whether it
// was created.
//
// It is InsertObject for what should not be held in memory. The body is read a
// chunk at a time, straight from body, and never passes through the fetch
// logging, which records only a body it is handed as bytes. A chunk broken off
// is not sent again from its start: the session is asked how much it kept, and
// the upload goes on from there.
//
// The object's CRC32C is declared when the session is opened -- metadata's own
// if it has one, otherwise computed here with a first pass over body -- so Cloud
// Storage refuses to create an object whose bytes are not the ones meant, and
// the object that comes back is checked against the declared size and CRC32C
// all the same.
//
// With upload_config.WithIfAbsent, a name already taken, when the session is
// opened or when it completes, is reported as nothing created rather than as an
// error, as InsertObjectIfAbsent does.
func (c *Client) UploadObject(
	ctx context.Context,
	bucketName string,
	metadata *object.Object,
	body io.ReaderAt,
	size int64,
	options ...upload_config.Option,
) (*object.Object, bool, error) {
	if bucketName == "" {
		return nil, false, altshiftErrors.NewWithTrace(empty_error.New("bucket name"))
	}
	if metadata == nil {
		return nil, false, altshiftErrors.NewWithTrace(nil_error.New("metadata"))
	}
	if metadata.Name == "" {
		return nil, false, altshiftErrors.NewWithTrace(empty_error.New("object name"))
	}
	if body == nil {
		return nil, false, altshiftErrors.NewWithTrace(nil_error.New("body"))
	}
	if size < 0 {
		return nil, false, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: size %d is negative", altshiftErrors.ErrValidationError, size),
		)
	}

	config := upload_config.New(options...)
	if config.ChunkSize <= 0 || config.ChunkSize%upload_config.ChunkGranularity != 0 {
		return nil, false, altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: chunk size %d is not a positive multiple of %d",
				altshiftErrors.ErrValidationError, config.ChunkSize, upload_config.ChunkGranularity,
			),
		)
	}
	if config.MaxAttempts < 1 {
		return nil, false, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: max attempts %d is less than one", altshiftErrors.ErrValidationError, config.MaxAttempts),
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, false, fmt.Errorf("context err: %w", err)
	}

	declared := *metadata
	if declared.Crc32c == "" {
		checksum, err := Crc32cOf(io.NewSectionReader(body, 0, size))
		if err != nil {
			return nil, false, fmt.Errorf("crc32c of: %w", err)
		}
		declared.Crc32c = checksum
	} else if _, err := DecodeCrc32c(declared.Crc32c); err != nil {
		return nil, false, fmt.Errorf("decode crc32c: %w", err)
	}

	upload := &resumableUpload{
		client:     c,
		bucketName: bucketName,
		metadata:   &declared,
		body:       body,
		size:       size,
		config:     config,
		// Statuses are the protocol here, a 308 above all, so they are read
		// rather than turned into errors; and a chunk is never retried blind,
		// since what to send next depends on what the session kept.
		fetchOptions: slices.Concat(
			c.config.FetchOptions,
			config.FetchOptions,
			[]fetch_config.Option{
				fetch_config.WithSkipErrorOnStatus(true),
				fetch_config.WithRetryConfig(nil),
			},
		),
	}

	uploadedObject, err := upload.run(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("resumable upload run: %w", err)
	}
	if uploadedObject == nil {
		return nil, false, nil
	}

	if uploadedObject.Size != strconv.FormatInt(size, 10) {
		return nil, false, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: uploaded object has size %q, not %d", altshiftErrors.ErrVerificationError, uploadedObject.Size, size),
			declared.Name,
		)
	}
	if uploadedObject.Crc32c != declared.Crc32c {
		return nil, false, altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: uploaded object has crc32c %q, not %q",
				altshiftErrors.ErrVerificationError, uploadedObject.Crc32c, declared.Crc32c,
			),
			declared.Name,
		)
	}

	return uploadedObject, true, nil
}

type resumableUpload struct {
	client       *Client
	bucketName   string
	metadata     *object.Object
	body         io.ReaderAt
	size         int64
	config       *upload_config.Config
	fetchOptions []fetch_config.Option
}

// uploadOutcome is what one request of the protocol came to.
type uploadOutcome struct {
	// object is set once the upload is complete.
	object *object.Object
	// persisted is how many bytes the session holds, when it is not complete.
	persisted int64
	// taken is a precondition failed: the name is in use.
	taken bool
	// gone is the session no longer existing, expired or ended.
	gone bool
	// transient is a failure worth trying again after a wait.
	transient error
}

func (u *resumableUpload) run(ctx context.Context) (*object.Object, error) {
	sessionUri := u.config.SessionUri
	// A session handed over by the caller is asked where it stands before
	// anything is sent to it; one opened here starts at nothing.
	query := sessionUri != ""
	var offset int64
	attempts := 0
	sessions := 0

	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("context err: %w", err)
		}

		if sessionUri == "" {
			sessions++
			if sessions > u.config.MaxAttempts {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: upload sessions ended %d times", ErrUploadStalled, sessions-1),
					u.metadata.Name,
				)
			}

			openedUri, taken, err := u.open(ctx)
			if err != nil {
				return nil, fmt.Errorf("open: %w", err)
			}
			if taken {
				return u.taken()
			}

			sessionUri = openedUri
			offset = 0
			query = false
			if u.config.OnSession != nil {
				u.config.OnSession(sessionUri)
			}
		}

		var outcome uploadOutcome
		var err error
		if query {
			outcome, err = u.send(ctx, sessionUri, u.size, u.size)
		} else {
			outcome, err = u.send(ctx, sessionUri, offset, min(offset+u.config.ChunkSize, u.size))
		}
		if err != nil {
			return nil, fmt.Errorf("send: %w", err)
		}

		switch {
		case outcome.object != nil:
			return outcome.object, nil
		case outcome.taken:
			return u.taken()
		case outcome.gone:
			sessionUri = ""
		case outcome.transient != nil:
			attempts++
			if attempts >= u.config.MaxAttempts {
				return nil, altshiftErrors.New(
					fmt.Errorf("%d attempts in a row failed: %w", attempts, outcome.transient),
					u.metadata.Name,
				)
			}
			if err := u.wait(ctx, attempts); err != nil {
				return nil, fmt.Errorf("wait: %w", err)
			}
			query = true
		default:
			if outcome.persisted > u.size {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: session holds %d bytes of %d", altshiftErrors.ErrVerificationError, outcome.persisted, u.size),
					u.metadata.Name,
				)
			}

			if !query && outcome.persisted <= offset {
				// The chunk was accepted and nothing was kept: sending it again
				// unchanged would loop.
				attempts++
				if attempts >= u.config.MaxAttempts {
					return nil, altshiftErrors.NewWithTrace(
						fmt.Errorf("%w: %d chunks in a row were not kept", ErrUploadStalled, attempts),
						u.metadata.Name,
					)
				}
				if err := u.wait(ctx, attempts); err != nil {
					return nil, fmt.Errorf("wait: %w", err)
				}
			} else if outcome.persisted > offset {
				attempts = 0
			}

			offset = outcome.persisted
			query = false
		}
	}
}

func (u *resumableUpload) taken() (*object.Object, error) {
	if u.config.IfAbsent {
		return nil, nil
	}

	return nil, altshiftErrors.NewWithTrace(
		&altshiftHttpErrors.Non2xxStatusCodeError{StatusCode: http.StatusPreconditionFailed},
		u.metadata.Name,
	)
}

func (u *resumableUpload) wait(ctx context.Context, attempts int) error {
	delay := u.config.RetryBaseDelay
	for range attempts - 1 {
		delay *= 2
		if delay >= u.config.RetryMaxDelay {
			delay = u.config.RetryMaxDelay
			break
		}
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("context done: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

// open starts a session and returns its URI, or reports the name taken.
func (u *resumableUpload) open(ctx context.Context) (string, bool, error) {
	metadataBytes, err := json.Marshal(u.metadata)
	if err != nil {
		return "", false, altshiftErrors.NewWithTrace(fmt.Errorf("json marshal (metadata): %w", err))
	}

	target := *u.client.uploadBaseUrl
	target.RawPath = target.Path + "b/" + url.PathEscape(u.bucketName) + "/o"
	target.Path += "b/" + u.bucketName + "/o"
	query := url.Values{"uploadType": {"resumable"}}
	if u.config.IfAbsent {
		query.Set("ifGenerationMatch", "0")
	}
	target.RawQuery = query.Encode()
	urlString := target.String()

	for attempt := 1; ; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, urlString, bytes.NewReader(metadataBytes))
		if err != nil {
			return "", false, altshiftErrors.NewWithTrace(fmt.Errorf("http new request with context: %w", err), urlString)
		}
		request.Header.Set("Content-Type", "application/json; charset=UTF-8")
		request.Header.Set("X-Upload-Content-Length", strconv.FormatInt(u.size, 10))
		if u.metadata.ContentType != "" {
			request.Header.Set("X-Upload-Content-Type", u.metadata.ContentType)
		}

		response, responseBody, err := altshiftHttpUtils.FetchWithRequest(ctx, request, u.fetchOptions...)

		var transient error
		switch {
		case err != nil:
			transient = err
		case response == nil:
			return "", false, altshiftErrors.NewWithTrace(nil_error.New("http response"), urlString)
		case response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated:
			location := response.Header.Get("Location")
			if location == "" {
				return "", false, altshiftErrors.NewWithTrace(empty_error.New("session uri"), urlString)
			}
			return location, false, nil
		case response.StatusCode == http.StatusPreconditionFailed:
			return "", true, nil
		case retryableStatus(response.StatusCode):
			transient = &altshiftHttpErrors.Non2xxStatusCodeError{StatusCode: response.StatusCode}
		default:
			return "", false, altshiftErrors.NewWithTrace(
				&altshiftHttpErrors.Non2xxStatusCodeError{StatusCode: response.StatusCode},
				urlString,
				string(responseBody),
			)
		}

		if attempt >= u.config.MaxAttempts {
			return "", false, altshiftErrors.New(
				fmt.Errorf("%d attempts to open a session failed: %w", attempt, transient),
				urlString,
			)
		}
		if err := u.wait(ctx, attempt); err != nil {
			return "", false, fmt.Errorf("wait: %w", err)
		}
	}
}

// send puts the bytes [start, end) of the body to the session. With start and
// end both at the size it sends none, which asks the session where it stands
// and completes an upload that holds every byte.
func (u *resumableUpload) send(ctx context.Context, sessionUri string, start int64, end int64) (uploadOutcome, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, sessionUri, http.NoBody)
	if err != nil {
		return uploadOutcome{}, altshiftErrors.NewWithTrace(fmt.Errorf("http new request with context: %w", err))
	}

	length := end - start
	if length > 0 {
		request.Body = io.NopCloser(io.NewSectionReader(u.body, start, length))
		request.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(io.NewSectionReader(u.body, start, length)), nil
		}
		request.ContentLength = length
		request.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end-1, u.size))
	} else {
		request.Header.Set("Content-Range", fmt.Sprintf("bytes */%d", u.size))
	}

	response, responseBody, err := altshiftHttpUtils.FetchWithRequest(ctx, request, u.fetchOptions...)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return uploadOutcome{}, fmt.Errorf("context err: %w", ctxErr)
		}
		return uploadOutcome{transient: err}, nil
	}
	if response == nil {
		return uploadOutcome{}, altshiftErrors.NewWithTrace(nil_error.New("http response"))
	}

	switch statusCode := response.StatusCode; {
	case statusCode == http.StatusOK || statusCode == http.StatusCreated:
		uploadedObject := &object.Object{}
		if err := json.Unmarshal(responseBody, uploadedObject); err != nil {
			return uploadOutcome{}, altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal (object): %w", err))
		}
		return uploadOutcome{object: uploadedObject}, nil
	case statusCode == http.StatusPermanentRedirect:
		persisted, err := persistedFromRange(response.Header.Get("Range"))
		if err != nil {
			return uploadOutcome{}, fmt.Errorf("persisted from range: %w", err)
		}
		return uploadOutcome{persisted: persisted}, nil
	case statusCode == http.StatusPreconditionFailed:
		return uploadOutcome{taken: true}, nil
	case statusCode == http.StatusNotFound || statusCode == http.StatusGone:
		return uploadOutcome{gone: true}, nil
	case retryableStatus(statusCode):
		return uploadOutcome{transient: &altshiftHttpErrors.Non2xxStatusCodeError{StatusCode: statusCode}}, nil
	default:
		return uploadOutcome{}, altshiftErrors.NewWithTrace(
			&altshiftHttpErrors.Non2xxStatusCodeError{StatusCode: statusCode},
			u.metadata.Name,
			string(responseBody),
		)
	}
}

func retryableStatus(statusCode int) bool {
	return statusCode == http.StatusRequestTimeout ||
		statusCode == http.StatusTooManyRequests ||
		statusCode >= http.StatusInternalServerError
}

// persistedFromRange reads how many bytes a session holds from the Range header
// of its 308, "bytes=0-N"; a session holding none sends no Range at all.
func persistedFromRange(rangeHeader string) (int64, error) {
	if rangeHeader == "" {
		return 0, nil
	}

	last, found := strings.CutPrefix(rangeHeader, "bytes=0-")
	if !found {
		return 0, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: range %q does not start at zero", altshiftErrors.ErrParseError, rangeHeader),
		)
	}

	lastByte, err := strconv.ParseInt(last, 10, 64)
	if err != nil {
		return 0, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: range %q: strconv parse int: %w", altshiftErrors.ErrParseError, rangeHeader, err),
		)
	}
	if lastByte < 0 {
		return 0, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: range %q ends before it starts", altshiftErrors.ErrParseError, rangeHeader),
		)
	}

	return lastByte + 1, nil
}
