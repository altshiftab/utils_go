package cloud_storage

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/reader_config"
)

func TestReader(t *testing.T) {
	t.Parallel()

	const size = 10_000
	data := randomBytes(size)

	testCases := []struct {
		name        string
		options     []reader_config.Option
		cutReads    int
		cutAfter    int
		ignoreRange bool
		want        []byte
		// wantRanges is the Range header of each media request.
		wantRanges []string
	}{
		{
			name:       "whole",
			want:       data,
			wantRanges: []string{""},
		},
		{
			name:       "a range",
			options:    []reader_config.Option{reader_config.WithRange(100, 1000)},
			want:       data[100:1100],
			wantRanges: []string{"bytes=100-1099"},
		},
		{
			name:       "a range past the end",
			options:    []reader_config.Option{reader_config.WithRange(size-10, 100)},
			want:       data[size-10:],
			wantRanges: []string{"bytes=9990-10089"},
		},
		{
			name:       "from the end",
			options:    []reader_config.Option{reader_config.WithRange(size, 0)},
			want:       []byte{},
			wantRanges: []string{"bytes=10000-"},
		},
		{
			name:     "broken off twice",
			cutReads: 2,
			cutAfter: 1000,
			want:     data,
			// Once the first answer has told the size, the rest are bounded by it.
			wantRanges: []string{"", "bytes=1000-9999", "bytes=2000-9999"},
		},
		{
			name:       "a range broken off",
			options:    []reader_config.Option{reader_config.WithRange(500, 3000)},
			cutReads:   1,
			cutAfter:   1000,
			want:       data[500:3500],
			wantRanges: []string{"bytes=500-3499", "bytes=1500-3499"},
		},
		{
			name:        "a range answered with the whole object",
			options:     []reader_config.Option{reader_config.WithRange(100, 50)},
			ignoreRange: true,
			want:        data[100:150],
			wantRanges:  []string{"bytes=100-149"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake, client := newFakeGcs(t)
			stored := fake.put("data/ab/abc", data)
			fake.cutReads = testCase.cutReads
			fake.cutAfter = testCase.cutAfter
			fake.ignoreRange = testCase.ignoreRange

			reader, err := client.NewReader(context.Background(), "bucket", "data/ab/abc", testCase.options...)
			if err != nil {
				t.Fatalf("new reader: %v", err)
			}
			defer func() { _ = reader.Close() }()

			got, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("io read all: %v", err)
			}
			if !bytes.Equal(got, testCase.want) {
				t.Errorf("read %d bytes that differ from the %d expected", len(got), len(testCase.want))
			}

			if reader.Generation() != stored.generation {
				t.Errorf("generation = %d, want %d", reader.Generation(), stored.generation)
			}
			if reader.Size() != size {
				t.Errorf("size = %d, want %d", reader.Size(), size)
			}
			if reader.Crc32c() != crc32cString(data) {
				t.Errorf("crc32c = %q, want %q", reader.Crc32c(), crc32cString(data))
			}

			if len(fake.mediaRequests) != len(testCase.wantRanges) {
				t.Fatalf("%d media requests, want %d", len(fake.mediaRequests), len(testCase.wantRanges))
			}
			for i, request := range fake.mediaRequests {
				if request.rangeHeader != testCase.wantRanges[i] {
					t.Errorf("request %d: range = %q, want %q", i, request.rangeHeader, testCase.wantRanges[i])
				}
				// Every request after the first is pinned to the generation the
				// first was served.
				if i > 0 && request.generation != strconv.FormatInt(stored.generation, 10) {
					t.Errorf("request %d: generation = %q, want %d", i, request.generation, stored.generation)
				}
			}
		})
	}
}

func TestReaderFailures(t *testing.T) {
	t.Parallel()

	const size = 10_000
	data := randomBytes(size)

	testCases := []struct {
		name string
		// setup prepares the fake after the object is stored.
		setup   func(fake *fakeGcs)
		options []reader_config.Option
		// failOpen is the failure expected from NewReader rather than from
		// reading.
		failOpen bool
	}{
		{
			name:     "missing",
			setup:    func(fake *fakeGcs) { delete(fake.objects, "name") },
			failOpen: true,
		},
		{
			name:     "an offset past the end",
			options:  []reader_config.Option{reader_config.WithRange(size+1, 0)},
			failOpen: true,
		},
		{
			name: "replaced while being read",
			setup: func(fake *fakeGcs) {
				fake.cutReads = 1
				fake.cutAfter = 1000
				fake.onMedia = func(n int) {
					if n == 2 {
						fake.put("name", data)
					}
				}
			},
		},
		{
			name: "broken off with nothing read, more often in a row than allowed",
			setup: func(fake *fakeGcs) {
				fake.cutReads = 3
				fake.cutAfter = 0
			},
			options: []reader_config.Option{reader_config.WithMaxReconnects(2)},
		},
		{
			name:    "refused for good on reopening",
			setup:   func(fake *fakeGcs) { fake.cutReads = 1; fake.cutAfter = 1000; fake.mediaStatus[2] = 403 },
			options: []reader_config.Option{reader_config.WithMaxReconnects(5)},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake, client := newFakeGcs(t)
			fake.put("name", data)
			if testCase.setup != nil {
				testCase.setup(fake)
			}

			options := append([]reader_config.Option{fastReads}, testCase.options...)
			reader, err := client.NewReader(context.Background(), "bucket", "name", options...)
			if testCase.failOpen {
				if err == nil {
					t.Errorf("new reader: err = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("new reader: %v", err)
			}
			defer func() { _ = reader.Close() }()

			if _, err := io.ReadAll(reader); err == nil {
				t.Errorf("io read all: err = nil")
			}
		})
	}
}

// fastReads keeps the waits between attempts out of the test's run time.
var fastReads = reader_config.WithRetryDelays(time.Millisecond, time.Millisecond)

// TestReaderRecovers covers what a long read meets: breaks that each made
// progress, and requests to resume that fail for a while before they work.
func TestReaderRecovers(t *testing.T) {
	t.Parallel()

	const size = 10_000
	data := randomBytes(size)

	testCases := []struct {
		name  string
		setup func(fake *fakeGcs)
		// wantRequests is how many media requests the read took.
		wantRequests int
	}{
		{
			name:         "more breaks than the budget, each after progress",
			setup:        func(fake *fakeGcs) { fake.cutReads = 20; fake.cutAfter = 400 },
			wantRequests: 21,
		},
		{
			name:         "a reopen refused with 503, then served",
			setup:        func(fake *fakeGcs) { fake.cutReads = 1; fake.cutAfter = 1000; fake.mediaStatus[2] = 503 },
			wantRequests: 3,
		},
		{
			name:         "the first request refused with 429 and 503",
			setup:        func(fake *fakeGcs) { fake.mediaStatus[1] = 429; fake.mediaStatus[2] = 503 },
			wantRequests: 3,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake, client := newFakeGcs(t)
			fake.put("name", data)
			testCase.setup(fake)

			reader, err := client.NewReader(
				context.Background(), "bucket", "name", fastReads, reader_config.WithMaxReconnects(2),
			)
			if err != nil {
				t.Fatalf("new reader: %v", err)
			}
			defer func() { _ = reader.Close() }()

			got, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("io read all: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Errorf("read %d bytes that differ from the %d stored", len(got), len(data))
			}
			if len(fake.mediaRequests) != testCase.wantRequests {
				t.Errorf("%d media requests, want %d", len(fake.mediaRequests), testCase.wantRequests)
			}
		})
	}
}

// TestReaderReadsStoredBytes reads an object stored gzip-compressed, which must
// come back as stored, and whole: neither decompressed by Cloud Storage nor by
// the transport and then cut at the stored length.
func TestReaderReadsStoredBytes(t *testing.T) {
	t.Parallel()

	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(bytes.Repeat([]byte("compressible "), 10_000)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	fake, client := newFakeGcs(t)
	fake.put("name", compressed.Bytes()).gzipped = true

	for _, options := range [][]reader_config.Option{nil, {reader_config.WithRange(10, 100)}} {
		reader, err := client.NewReader(context.Background(), "bucket", "name", options...)
		if err != nil {
			t.Fatalf("new reader: %v", err)
		}
		got, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatalf("io read all: %v", err)
		}

		want := compressed.Bytes()
		if len(options) != 0 {
			want = want[10:110]
		}
		if !bytes.Equal(got, want) {
			t.Errorf("read %d bytes, not the %d stored", len(got), len(want))
		}
	}
}

func TestParseContentRange(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		value     string
		wantStart int64
		wantTotal int64
		wantErr   bool
	}{
		{name: "span", value: "bytes 100-199/1000", wantStart: 100, wantTotal: 1000},
		{name: "unsatisfiable", value: "bytes */1000", wantStart: 0, wantTotal: 1000},
		{name: "no unit", value: "100-199/1000", wantErr: true},
		{name: "no total", value: "bytes 100-199", wantErr: true},
		{name: "bad start", value: "bytes x-199/1000", wantErr: true},
	}

	for _, testCase := range testCases {
		start, total, err := parseContentRange(testCase.value)
		if (err != nil) != testCase.wantErr {
			t.Errorf("%s: err = %v, want error %t", testCase.name, err, testCase.wantErr)
			continue
		}
		if start != testCase.wantStart || total != testCase.wantTotal {
			t.Errorf("%s: got (%d, %d), want (%d, %d)", testCase.name, start, total, testCase.wantStart, testCase.wantTotal)
		}
	}
}

func TestCrc32cFromHashHeaders(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "one header", values: []string{"crc32c=4waSgw==,md5=AAAA"}, want: "4waSgw=="},
		{name: "separate headers", values: []string{"md5=AAAA", "crc32c=4waSgw=="}, want: "4waSgw=="},
		{name: "none", values: []string{"md5=AAAA"}, want: ""},
	}

	for _, testCase := range testCases {
		if got := crc32cFromHashHeaders(testCase.values); got != testCase.want {
			t.Errorf("%s: got %q, want %q", testCase.name, got, testCase.want)
		}
	}
}
