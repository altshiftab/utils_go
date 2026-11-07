package cloud_storage

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math/rand/v2"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/types/object"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/upload_config"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
)

const testChunk = upload_config.ChunkGranularity

func randomBytes(n int) []byte {
	data := make([]byte, n)
	var seed [32]byte
	binary.BigEndian.PutUint64(seed[:], uint64(n)) //nolint:gosec // A test size is never negative.
	_, _ = rand.NewChaCha8(seed).Read(data)

	return data
}

// fastRetries keeps the waits between attempts out of the test's run time.
var fastRetries = upload_config.WithRetryDelays(time.Millisecond, time.Millisecond)

func TestUploadObject(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		size int
		// failChunk injects failures by the count of PUTs carrying bytes.
		failChunk map[int]*chunkFailure
		// wantOffsets is the start offset of each PUT carrying bytes; a client
		// that sends a chunk the session already holds shows up here.
		wantOffsets     []int64
		wantInitiations int
	}{
		{
			name:            "one chunk",
			size:            1000,
			wantOffsets:     []int64{0},
			wantInitiations: 1,
		},
		{
			name:            "several chunks, the last one short",
			size:            3*testChunk + 123,
			wantOffsets:     []int64{0, testChunk, 2 * testChunk, 3 * testChunk},
			wantInitiations: 1,
		},
		{
			name:            "a whole number of chunks",
			size:            2 * testChunk,
			wantOffsets:     []int64{0, testChunk},
			wantInitiations: 1,
		},
		{
			name:            "empty",
			size:            0,
			wantOffsets:     nil,
			wantInitiations: 1,
		},
		{
			name:            "a chunk refused with 503 is sent again",
			size:            3 * testChunk,
			failChunk:       map[int]*chunkFailure{2: {status: http.StatusServiceUnavailable}},
			wantOffsets:     []int64{0, testChunk, testChunk, 2 * testChunk},
			wantInitiations: 1,
		},
		{
			name: "a chunk kept but its answer lost is not sent again",
			size: 3 * testChunk,
			failChunk: map[int]*chunkFailure{
				2: {status: 0, keep: true},
			},
			wantOffsets:     []int64{0, testChunk, 2 * testChunk},
			wantInitiations: 1,
		},
		{
			name: "a dropped connection with nothing kept resends the chunk",
			size: 2 * testChunk,
			failChunk: map[int]*chunkFailure{
				1: {status: 0},
			},
			wantOffsets:     []int64{0, 0, testChunk},
			wantInitiations: 1,
		},
		{
			name:            "a session gone midway is opened again",
			size:            2 * testChunk,
			failChunk:       map[int]*chunkFailure{2: {status: http.StatusGone}},
			wantOffsets:     []int64{0, testChunk, 0, testChunk},
			wantInitiations: 2,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake, client := newFakeGcs(t)
			for n, failure := range testCase.failChunk {
				fake.failChunk[n] = failure
			}
			data := randomBytes(testCase.size)

			uploaded, created, err := client.UploadObject(
				context.Background(),
				"bucket",
				&object.Object{Name: "data/ab/abc", ContentType: "application/octet-stream"},
				bytes.NewReader(data),
				int64(len(data)),
				upload_config.WithChunkSize(testChunk),
				fastRetries,
			)
			if err != nil {
				t.Fatalf("upload object: %v", err)
			}
			if !created {
				t.Fatalf("created = false")
			}

			stored := fake.get("data/ab/abc")
			if stored == nil {
				t.Fatalf("no object was stored")
			}
			if !bytes.Equal(stored.data, data) {
				t.Errorf("stored %d bytes that differ from the %d uploaded", len(stored.data), len(data))
			}
			if uploaded.Crc32c != crc32cString(data) {
				t.Errorf("crc32c = %q, want %q", uploaded.Crc32c, crc32cString(data))
			}
			if !slices.Equal(fake.chunkOffsets, testCase.wantOffsets) {
				t.Errorf("chunk offsets = %v, want %v", fake.chunkOffsets, testCase.wantOffsets)
			}
			if fake.initiations != testCase.wantInitiations {
				t.Errorf("initiations = %d, want %d", fake.initiations, testCase.wantInitiations)
			}
		})
	}
}

func TestUploadObjectIfAbsent(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		// existsBefore puts an object under the name before the upload starts;
		// existsDuring puts one there while the first chunk is in flight.
		existsBefore bool
		existsDuring bool
		ifAbsent     bool
		wantCreated  bool
		wantStatus   int
	}{
		{name: "free name", ifAbsent: true, wantCreated: true},
		{name: "taken before", existsBefore: true, ifAbsent: true, wantCreated: false},
		{name: "taken during", existsDuring: true, ifAbsent: true, wantCreated: false},
		{name: "taken during, without if absent", existsDuring: true, ifAbsent: false, wantCreated: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake, client := newFakeGcs(t)
			other := []byte("someone else's")
			if testCase.existsBefore {
				fake.put("name", other)
			}
			if testCase.existsDuring {
				fake.onChunk = func(n int) {
					if n == 1 {
						fake.put("name", other)
					}
				}
			}

			data := randomBytes(2 * testChunk)
			_, created, err := client.UploadObject(
				context.Background(),
				"bucket",
				&object.Object{Name: "name"},
				bytes.NewReader(data),
				int64(len(data)),
				upload_config.WithChunkSize(testChunk),
				upload_config.WithIfAbsent(testCase.ifAbsent),
				fastRetries,
			)
			if err != nil {
				t.Fatalf("upload object: %v", err)
			}
			if created != testCase.wantCreated {
				t.Errorf("created = %t, want %t", created, testCase.wantCreated)
			}

			want := data
			if !testCase.wantCreated {
				want = other
			}
			if stored := fake.get("name"); stored == nil || !bytes.Equal(stored.data, want) {
				t.Errorf("the stored object is not the one expected")
			}
		})
	}
}

func TestUploadObjectFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		metadata  *object.Object
		failChunk map[int]*chunkFailure
		options   []upload_config.Option
		check     func(t *testing.T, err error)
	}{
		{
			name:     "a declared crc32c the bytes do not match",
			metadata: &object.Object{Name: "name", Crc32c: EncodeCrc32c(12345)},
			check: func(t *testing.T, err error) {
				t.Helper()
				statusError, ok := errors.AsType[*altshiftHttpErrors.Non2xxStatusCodeError](err)
				if !ok || statusError.StatusCode != http.StatusBadRequest {
					t.Errorf("err = %v, want a 400", err)
				}
			},
		},
		{
			name:     "too many failures in a row",
			metadata: &object.Object{Name: "name"},
			failChunk: map[int]*chunkFailure{
				2: {status: http.StatusServiceUnavailable},
				3: {status: http.StatusServiceUnavailable},
				4: {status: http.StatusServiceUnavailable},
			},
			options: []upload_config.Option{upload_config.WithMaxAttempts(3)},
			check: func(t *testing.T, err error) {
				t.Helper()
				if err == nil {
					t.Errorf("err = nil")
				}
			},
		},
		{
			name:     "a chunk size off the granularity",
			metadata: &object.Object{Name: "name"},
			options:  []upload_config.Option{upload_config.WithChunkSize(testChunk + 1)},
			check: func(t *testing.T, err error) {
				t.Helper()
				if !errors.Is(err, altshiftErrors.ErrValidationError) {
					t.Errorf("err = %v, want a validation error", err)
				}
			},
		},
		{
			name:     "a malformed declared crc32c",
			metadata: &object.Object{Name: "name", Crc32c: "not base64!"},
			check: func(t *testing.T, err error) {
				t.Helper()
				if !errors.Is(err, altshiftErrors.ErrParseError) {
					t.Errorf("err = %v, want a parse error", err)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake, client := newFakeGcs(t)
			for n, failure := range testCase.failChunk {
				fake.failChunk[n] = failure
			}
			data := randomBytes(3 * testChunk)

			options := append([]upload_config.Option{upload_config.WithChunkSize(testChunk), fastRetries}, testCase.options...)
			_, created, err := client.UploadObject(
				context.Background(), "bucket", testCase.metadata, bytes.NewReader(data), int64(len(data)), options...,
			)
			if created {
				t.Errorf("created = true")
			}
			testCase.check(t, err)
			if fake.get("name") != nil {
				t.Errorf("an object was stored")
			}
		})
	}
}

// TestUploadObjectResumesSession is the upload continued by a later process: the
// first gives up after its first chunk, and the second, handed the session the
// first was told about, sends only what the session lacks.
func TestUploadObjectResumesSession(t *testing.T) {
	t.Parallel()

	fake, client := newFakeGcs(t)
	fake.failChunk[2] = &chunkFailure{status: http.StatusServiceUnavailable}
	data := randomBytes(3 * testChunk)

	var sessionUri string
	_, _, err := client.UploadObject(
		context.Background(),
		"bucket",
		&object.Object{Name: "name"},
		bytes.NewReader(data),
		int64(len(data)),
		upload_config.WithChunkSize(testChunk),
		upload_config.WithMaxAttempts(1),
		upload_config.WithOnSession(func(uri string) { sessionUri = uri }),
		fastRetries,
	)
	if err == nil {
		t.Fatalf("the first upload did not fail")
	}
	if sessionUri == "" {
		t.Fatalf("no session was reported")
	}

	_, created, err := client.UploadObject(
		context.Background(),
		"bucket",
		&object.Object{Name: "name"},
		bytes.NewReader(data),
		int64(len(data)),
		upload_config.WithChunkSize(testChunk),
		upload_config.WithSessionUri(sessionUri),
		fastRetries,
	)
	if err != nil {
		t.Fatalf("upload object (resumed): %v", err)
	}
	if !created {
		t.Errorf("created = false")
	}

	if stored := fake.get("name"); stored == nil || !bytes.Equal(stored.data, data) {
		t.Errorf("the stored object is not the one uploaded")
	}
	if want := []int64{0, testChunk, testChunk, 2 * testChunk}; !slices.Equal(fake.chunkOffsets, want) {
		t.Errorf("chunk offsets = %v, want %v", fake.chunkOffsets, want)
	}
	if fake.initiations != 1 {
		t.Errorf("initiations = %d, want 1", fake.initiations)
	}
}

func TestPersistedFromRange(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		header  string
		want    int64
		wantErr bool
	}{
		{name: "none", header: "", want: 0},
		{name: "some", header: "bytes=0-262143", want: 262144},
		{name: "not from zero", header: "bytes=5-10", wantErr: true},
		{name: "not a number", header: "bytes=0-x", wantErr: true},
	}

	for _, testCase := range testCases {
		got, err := persistedFromRange(testCase.header)
		if (err != nil) != testCase.wantErr {
			t.Errorf("%s: err = %v, want error %t", testCase.name, err, testCase.wantErr)
		}
		if got != testCase.want {
			t.Errorf("%s: got %d, want %d", testCase.name, got, testCase.want)
		}
	}
}
