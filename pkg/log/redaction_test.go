package log

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"testing"
	"time"

	context2 "github.com/altshiftab/utils_go/pkg/context"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/json/jose/jws"
	"github.com/altshiftab/utils_go/pkg/json/jose/jws/types/jws_object"
)

var errTokenRefused = errors.New("refused")

/*
 * The net under the code that attaches values to errors: a token attached anywhere, by code written
 * before the rule or after it, reaches the log without its signature. The header and payload are
 * kept, as they are for a header carrying a token.
 */
func TestErrorContextExtractor_MakeErrorAttrs_MasksSignatures(t *testing.T) {
	t.Parallel()

	const (
		token     = "eyJhbGciOiJFZERTQSJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJlLWJ5dGVz" //nolint:gosec // A made-up token, not a credential.
		signature = "c2lnbmF0dXJlLWJ5dGVz"
		masked    = "eyJhbGciOiJFZERTQSJ9.eyJzdWIiOiIxIn0.(MASKED)"
	)

	testCases := []struct {
		name string
		err  error
		// What the entry writes in the token's place; the masked token unless said otherwise.
		want string
	}{
		{name: "a token as the input", err: altshiftErrors.New(errTokenRefused, token)},
		{name: "a token among the inputs", err: altshiftErrors.New(errTokenRefused, "context", token, 3)},
		{name: "a token inside a value", err: altshiftErrors.New(errTokenRefused, map[string]any{"token": token})},
		{
			// An object writes itself out as its fingerprint, which the net need not touch.
			name: "a token attached as an object",
			err:  altshiftErrors.New(errTokenRefused, mustObject(token)),
			want: jws.Fingerprint(token),
		},
		{
			name: "a token on a wrapped cause",
			err:  altshiftErrors.New(fmt.Errorf("outer: %w", altshiftErrors.New(errTokenRefused, token))),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			written := fmt.Sprint((&ErrorContextExtractor{}).MakeErrorAttrs(testCase.err))
			if strings.Contains(written, signature) {
				t.Errorf("%s: the signature is written out: %s", testCase.name, written)
			}
			want := testCase.want
			if want == "" {
				want = masked
			}
			if !strings.Contains(written, want) {
				t.Errorf("%s: %q is not written out: %s", testCase.name, want, written)
			}
		})
	}
}

func TestErrorContextExtractor_MakeErrorAttrs_MasksMessages(t *testing.T) {
	t.Parallel()

	const (
		token     = "eyJhbGciOiJFZERTQSJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJlLWJ5dGVz" //nolint:gosec // A made-up token.
		signature = "c2lnbmF0dXJlLWJ5dGVz"
	)

	testCases := []struct {
		name string
		err  error
	}{
		{name: "a token formatted into the message", err: fmt.Errorf("refused %s: %w", token, errTokenRefused)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if written := fmt.Sprint((&ErrorContextExtractor{}).MakeErrorAttrs(testCase.err)); strings.Contains(written, signature) {
				t.Errorf("%s: the signature is written out: %s", testCase.name, written)
			}
		})
	}
}

/*
 * The URL a request failed for is written out by the error context extractor itself, as the URL in the
 * error's context, and the caller often writes it into the message the error is logged with. A token
 * in it reaches the log without its signature in either, as it does in the error's own message --
 * whatever else is masked in the URL.
 */
func TestErrorContextExtractor_Handle_MasksSignatures(t *testing.T) {
	t.Parallel()

	const (
		token     = "eyJhbGciOiJFZERTQSJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJlLWJ5dGVz" //nolint:gosec // A made-up token, not a credential.
		signature = "c2lnbmF0dXJlLWJ5dGVz"
		masked    = "eyJhbGciOiJFZERTQSJ9.eyJzdWIiOiIxIn0.(MASKED)"
	)

	inQuery := &url.Error{Op: "Get", URL: "https://example.com/callback?jwt=" + token, Err: errTokenRefused}
	inPath := &url.Error{Op: "Get", URL: "https://example.com/tokens/" + token + "/status", Err: errTokenRefused}

	plain := &ErrorContextExtractor{}
	maskingUrls := &ErrorContextExtractor{ContextExtractors: []ContextExtractor{maskingExtractor{word: "callback"}}}

	testCases := []struct {
		name      string
		extractor *ErrorContextExtractor
		err       error
		message   string
	}{
		{name: "a token in the query of the url", extractor: plain, err: inQuery, message: "The request failed."},
		{name: "a token in the path of the url", extractor: plain, err: inPath, message: "The request failed."},
		{name: "a token in the url, wrapped", extractor: plain, err: fmt.Errorf("get: %w", inQuery), message: "The request failed."},
		{name: "a token in a url masked as well", extractor: maskingUrls, err: inQuery, message: "The request failed."},
		{name: "a token in the url, written into the message", extractor: plain, err: inQuery, message: fmt.Sprintf("The request failed: %v", inQuery)},
		{name: "a token written into the message", extractor: plain, err: errTokenRefused, message: "The token " + token + " was refused."},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			record := slog.NewRecord(time.Time{}, slog.LevelError, testCase.message, 0)
			if err := testCase.extractor.Handle(context2.WithError(context.Background(), testCase.err), &record); err != nil {
				t.Fatalf("%s: handle: %v", testCase.name, err)
			}

			written := record.Message
			record.Attrs(func(attr slog.Attr) bool {
				written += " " + attr.String()
				return true
			})
			if strings.Contains(written, signature) {
				t.Errorf("%s: the signature is written out: %s", testCase.name, written)
			}
			if !strings.Contains(written, masked) {
				t.Errorf("%s: %q is not written out: %s", testCase.name, masked, written)
			}
		})
	}
}

func mustObject(serialization string) *jws_object.Object {
	object, err := jws_object.New(serialization)
	if err != nil {
		panic(err)
	}
	return object
}
