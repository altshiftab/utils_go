package jws

import (
	"strings"
	"testing"
)

const testToken = "eyJhbGciOiJFZERTQSJ9.eyJzdWIiOiJ1c2VyQGV4YW1wbGUuY29tIn0.c2lnbmF0dXJlLWJ5dGVz" //nolint:gosec // A made-up token, not a credential.

func TestFingerprint(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		serialization string
		want          string
	}{
		// SHA-256 of the empty string begins e3b0c44298fc1c14.
		{name: "the empty string", serialization: "", want: "sha256:e3b0c44298fc1c14"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := Fingerprint(testCase.serialization); got != testCase.want {
				t.Errorf("%s: got %q, want %q", testCase.name, got, testCase.want)
			}
		})
	}

	if Fingerprint(testToken) == Fingerprint(testToken+"x") {
		t.Error("two serializations share a fingerprint")
	}
	if strings.Contains(Fingerprint(testToken), "c2lnbmF0dXJl") {
		t.Error("the fingerprint carries the token")
	}
}

func TestMaskSignatures(t *testing.T) {
	t.Parallel()

	const masked = "eyJhbGciOiJFZERTQSJ9.eyJzdWIiOiJ1c2VyQGV4YW1wbGUuY29tIn0.(MASKED)"

	testCases := []struct {
		name string
		text string
		want string
	}{
		{name: "a token alone", text: testToken, want: masked},
		{name: "a token in a sentence", text: "refused " + testToken + " at refresh", want: "refused " + masked + " at refresh"},
		{name: "a token as %#v quotes it", text: `"` + testToken + `"`, want: `"` + masked + `"`},
		{name: "two tokens", text: testToken + " " + testToken, want: masked + " " + masked},
		{name: "a token with no signature", text: "eyJhbGciOiJub25lIn0.eyJzdWIiOiIxIn0.", want: "eyJhbGciOiJub25lIn0.eyJzdWIiOiIxIn0."},
		{name: "a dotted name that is not a token", text: "github.com.example", want: "github.com.example"},
		{name: "no token", text: "an error occurred", want: "an error occurred"},
		{name: "eyJ inside a word", text: "keyJar.foo.bar", want: "keyJar.foo.bar"},
		{name: "eyJ inside a path", text: "https://x.example/aeyJabc.def.ghi", want: "https://x.example/aeyJabc.def.ghi"},
		{name: "a token after a slash", text: "/t/" + testToken, want: "/t/" + masked},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := MaskSignatures(testCase.text); got != testCase.want {
				t.Errorf("%s: got %q, want %q", testCase.name, got, testCase.want)
			}
		})
	}
}
