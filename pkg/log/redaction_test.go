package log

import (
	"errors"
	"fmt"
	"strings"
	"testing"

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

func mustObject(serialization string) *jws_object.Object {
	object, err := jws_object.New(serialization)
	if err != nil {
		panic(err)
	}
	return object
}
