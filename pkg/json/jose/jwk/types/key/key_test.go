package key

import (
	"encoding/json/v2"
	"testing"

	ecKey "github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key/ec"
	rsaKey "github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key/rsa"
)

func TestKey_MarshalJSON_EC(t *testing.T) {
	t.Parallel()

	k := &Key{
		Alg: "ES256",
		Kty: "EC",
		Kid: "kid-ec-1",
		Use: "sig",
		Material: &ecKey.Key{
			Crv: "P-256",
			X:   "AQIDBAUGBwgJCgsMDQ4PEA",
			Y:   "AgMEBQYHCAkKCwwNDg8QEQ",
		},
	}

	b, err := json.Marshal(k)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal back: %v", err)
	}

	want := map[string]any{
		"alg": "ES256",
		"kty": "EC",
		"kid": "kid-ec-1",
		"use": "sig",
		"crv": "P-256",
		"x":   "AQIDBAUGBwgJCgsMDQ4PEA",
		"y":   "AgMEBQYHCAkKCwwNDg8QEQ",
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected field count: got %d want %d (%v)", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("field %q: got %v want %v (full: %v)", k, got[k], v, got)
		}
	}
}

func TestKey_MarshalJSON_RSA(t *testing.T) {
	t.Parallel()

	k := &Key{
		Alg: "RS256",
		Kty: "RSA",
		Kid: "kid-rsa-1",
		Use: "sig",
		Material: &rsaKey.Key{
			N: "sometestmodulus",
			E: "AQAB",
		},
	}

	b, err := json.Marshal(k)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal back: %v", err)
	}

	want := map[string]any{
		"alg": "RS256",
		"kty": "RSA",
		"kid": "kid-rsa-1",
		"use": "sig",
		"n":   "sometestmodulus",
		"e":   "AQAB",
	}

	if len(got) != len(want) {
		t.Fatalf("unexpected field count: got %d want %d (%v)", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("field %q: got %v want %v (full: %v)", k, got[k], v, got)
		}
	}
}
