package eddsa

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	jsonV1 "encoding/json"
	"encoding/json/v2"
	"fmt"
	"strings"
	"testing"

	altshiftStrings "github.com/altshiftab/utils_go/pkg/strings"
)

// renderings writes a value out every way a log entry, an error or a JSON encoder might.
func renderings(value any) map[string]string {
	textual, _ := altshiftStrings.MakeTextualRepresentation(value)
	jsonData, _ := json.Marshal(value)
	jsonV1Data, _ := jsonV1.Marshal(value)
	nested := struct{ Method any }{Method: value}

	return map[string]string{
		"%#v":                    fmt.Sprintf("%#v", value),
		"%v":                     fmt.Sprintf("%v", value),
		"%+v":                    fmt.Sprintf("%+v", value),
		"%s":                     fmt.Sprintf("%s", value),
		"json/v2":                string(jsonData),
		"encoding/json":          string(jsonV1Data),
		"textual representation": textual,
		"nested, %+v":            fmt.Sprintf("%+v", nested),
		"nested, %#v":            fmt.Sprintf("%#v", nested),
	}
}

// byteListForm is how %#v writes bytes out: "0xe7, 0xbd, ...".
func byteListForm(data []byte) string {
	parts := make([]string, 0, len(data))
	for _, b := range data {
		parts = append(parts, fmt.Sprintf("%#x", b))
	}
	return strings.Join(parts, ", ")
}

func secretFormsOf(secret []byte) []string {
	return []string{
		hex.EncodeToString(secret),
		base64.StdEncoding.EncodeToString(secret),
		base64.RawURLEncoding.EncodeToString(secret),
		byteListForm(secret[:8]),
	}
}

func checkRedacted(t *testing.T, name string, method any, wantGoString string, secretForms []string) {
	t.Helper()

	if got := fmt.Sprintf("%#v", method); got != wantGoString {
		t.Errorf("%s: %%#v = %q, want %q", name, got, wantGoString)
	}

	for rendering, written := range renderings(method) {
		for _, secretForm := range secretForms {
			if strings.Contains(written, secretForm) {
				t.Errorf("%s: %s writes the secret out: %s", name, rendering, written)
			}
		}
	}
}

func TestMethod_Redaction(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("ed25519 generate key: %v", err)
	}
	encodedPublicKey := base64.RawURLEncoding.EncodeToString(publicKey)
	secretForms := append(secretFormsOf(privateKey.Seed()), secretFormsOf(privateKey)...)

	testCases := []struct {
		name         string
		method       *Method
		wantGoString string
	}{
		{
			name:         "a signer",
			method:       &Method{PrivateKey: privateKey, PublicKey: publicKey},
			wantGoString: fmt.Sprintf("&eddsa.Method{PublicKey:%q, PrivateKey:<redacted>}", encodedPublicKey),
		},
		{
			name:         "a verifier",
			method:       &Method{PublicKey: publicKey},
			wantGoString: fmt.Sprintf("&eddsa.Method{PublicKey:%q, PrivateKey:<nil>}", encodedPublicKey),
		},
		{name: "nil", wantGoString: "(*eddsa.Method)(nil)"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			checkRedacted(t, testCase.name, testCase.method, testCase.wantGoString, secretForms)
		})
	}
}
