package cloud_storage

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
)

func TestCrc32c(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		// want is CRC32C's published check value for "123456789", and the
		// encoding of the empty input's zero.
		want string
	}{
		{name: "check value", input: "123456789", want: "4waSgw=="},
		{name: "empty", input: "", want: "AAAAAA=="},
	}

	for _, testCase := range testCases {
		got, err := Crc32cOf(strings.NewReader(testCase.input))
		if err != nil {
			t.Fatalf("%s: crc32c of: %v", testCase.name, err)
		}
		if got != testCase.want {
			t.Errorf("%s: got %q, want %q", testCase.name, got, testCase.want)
		}

		decoded, err := DecodeCrc32c(got)
		if err != nil {
			t.Fatalf("%s: decode crc32c: %v", testCase.name, err)
		}
		if EncodeCrc32c(decoded) != got {
			t.Errorf("%s: %q did not survive decoding and encoding", testCase.name, got)
		}
	}
}

func TestDecodeCrc32cRejects(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		value string
	}{
		{name: "not base64", value: "!!!!"},
		{name: "too long", value: "AAAAAAAA"},
		{name: "too short", value: "AAA="},
		{name: "empty", value: ""},
	}

	for _, testCase := range testCases {
		if _, err := DecodeCrc32c(testCase.value); !errors.Is(err, altshiftErrors.ErrParseError) {
			t.Errorf("%s: err = %v, want a parse error", testCase.name, err)
		}
	}
}

func TestDeleteObjectGeneration(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		// generationOffset is added to the stored generation to make the one
		// asked for.
		generationOffset int64
		missing          bool
		wantStatus       int
		wantDeleted      bool
	}{
		{name: "the live generation", wantDeleted: true},
		{name: "a stale generation", generationOffset: -1, wantStatus: http.StatusPreconditionFailed},
		{name: "a newer generation", generationOffset: 1, wantStatus: http.StatusPreconditionFailed},
		{name: "gone already", missing: true, wantStatus: http.StatusNotFound},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake, client := newFakeGcs(t)
			fake.put("other", []byte("x"))
			stored := fake.put("name", []byte("data"))
			generation := stored.generation + testCase.generationOffset
			if testCase.missing {
				delete(fake.objects, "name")
			}

			err := client.DeleteObjectGeneration(context.Background(), "bucket", "name", generation)
			if testCase.wantStatus == 0 {
				if err != nil {
					t.Fatalf("delete object generation: %v", err)
				}
			} else {
				statusError, ok := errors.AsType[*altshiftHttpErrors.Non2xxStatusCodeError](err)
				if !ok || statusError.StatusCode != testCase.wantStatus {
					t.Errorf("err = %v, want status %d", err, testCase.wantStatus)
				}
			}

			if deleted := fake.get("name") == nil; deleted != (testCase.wantDeleted || testCase.missing) {
				t.Errorf("deleted = %t", deleted)
			}
		})
	}
}

func TestDeleteObjectGenerationRejectsArguments(t *testing.T) {
	t.Parallel()

	_, client := newFakeGcs(t)

	testCases := []struct {
		name       string
		bucket     string
		object     string
		generation int64
	}{
		{name: "no bucket", object: "name", generation: 1},
		{name: "no object", bucket: "bucket", generation: 1},
		{name: "zero generation", bucket: "bucket", object: "name", generation: 0},
	}

	for _, testCase := range testCases {
		if err := client.DeleteObjectGeneration(context.Background(), testCase.bucket, testCase.object, testCase.generation); err == nil {
			t.Errorf("%s: err = nil", testCase.name)
		}
	}
}
