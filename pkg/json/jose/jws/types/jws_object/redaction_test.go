package jws_object

import (
	"encoding/json/v2"
	"fmt"
	"strings"
	"testing"

	"github.com/altshiftab/utils_go/pkg/json/jose/jws"
	altshiftStrings "github.com/altshiftab/utils_go/pkg/strings"
)

func TestObject_Redaction(t *testing.T) {
	t.Parallel()

	const raw = "eyJhbGciOiJFZERTQSJ9.eyJzdWIiOiJwZXJzb25AZXhhbXBsZS5jb20ifQ.c2lnbmF0dXJlLWJ5dGVz" //nolint:gosec // A made-up token.

	object, err := New(raw)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// What must not be written out: the signature and the payload, in every form they could take.
	secretForms := []string{
		"c2lnbmF0dXJlLWJ5dGVz",
		"signature-bytes",
		fmt.Sprintf("%#v", object.Signature)[len("[]byte{"):][:20],
		"person@example.com",
		fmt.Sprintf("%#v", object.Payload)[len("[]byte{"):][:20],
	}

	textual, _ := altshiftStrings.MakeTextualRepresentation(object)
	jsonData, _ := json.Marshal(object)
	renderings := map[string]string{
		"%#v":                    fmt.Sprintf("%#v", object),
		"%v":                     fmt.Sprintf("%v", object),
		"%+v":                    fmt.Sprintf("%+v", object),
		"textual representation": textual,
		"json":                   string(jsonData),
		"nested, %+v":            fmt.Sprintf("%+v", struct{ Token any }{Token: object}),
	}

	testCases := []struct {
		name   string
		object *Object
		want   string
	}{
		{name: "an object", object: object, want: fmt.Sprintf("&jws_object.Object{Fingerprint:%q}", jws.Fingerprint(raw))},
		{name: "nil", want: "(*jws_object.Object)(nil)"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := fmt.Sprintf("%#v", testCase.object); got != testCase.want {
				t.Errorf("%s: %%#v = %q, want %q", testCase.name, got, testCase.want)
			}
		})
	}

	for rendering, written := range renderings {
		for _, secretForm := range secretForms {
			if strings.Contains(written, secretForm) {
				t.Errorf("%s writes %q out: %s", rendering, secretForm, written)
			}
		}
	}
}
