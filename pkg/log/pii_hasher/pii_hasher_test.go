package pii_hasher

import (
	"errors"
	"testing"
)

// testKey is the bytes 0 to 31, the key the reference values below were computed with.
func testKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

func TestNew(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		key     []byte
		wantErr bool
		// wantShort is the error being the key's length rather than its absence.
		wantShort bool
	}{
		{name: "a key of the minimum length", key: testKey()},
		{name: "a longer key", key: append(testKey(), testKey()...)},
		{name: "no key", key: nil, wantErr: true},
		{name: "an empty key", key: []byte{}, wantErr: true},
		{name: "a key one byte short", key: testKey()[:MinimumKeyLength-1], wantErr: true, wantShort: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			hasher, err := New(testCase.key)
			if testCase.wantErr {
				if err == nil || hasher != nil {
					t.Errorf("%s: got %v, %v, want an error and no hasher", testCase.name, hasher, err)
				}
				if isShort := errors.Is(err, ErrShortKey); isShort != testCase.wantShort {
					t.Errorf("%s: the error is ErrShortKey: %t, want %t", testCase.name, isShort, testCase.wantShort)
				}
				return
			}
			if err != nil || hasher == nil {
				t.Fatalf("%s: got %v, %v, want a hasher", testCase.name, hasher, err)
			}
		})
	}
}

/*
 * The values are fixed, since other services compute the same and are matched against them. The
 * reference values were computed apart from this code, as Python's
 * hmac.new(bytes(range(32)), address.lower().encode(), hashlib.sha256).hexdigest().
 */
func TestHasher_HashEmailAddress(t *testing.T) {
	t.Parallel()

	hasher, err := New(testKey())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if hasher == nil {
		t.Fatal("no hasher")
	}

	const reference = "e3b5ff4c7131a4a458cc2f74705b07c83fc78073c249b5bb72bd84a7cfb744ac"

	testCases := []struct {
		name         string
		emailAddress string
		want         string
	}{
		{name: "an address", emailAddress: "person@example.com", want: reference},
		{name: "the same address in another case", emailAddress: "Person@Example.com", want: reference},
		{name: "no address", emailAddress: "", want: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := hasher.HashEmailAddress(testCase.emailAddress); got != testCase.want {
				t.Errorf("%s: got %q, want %q", testCase.name, got, testCase.want)
			}
		})
	}

	if hasher.HashEmailAddress("person@example.com") == hasher.HashEmailAddress("other@example.com") {
		t.Error("two addresses hash alike")
	}

	other, err := New(append(testKey()[1:], 32))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if other == nil {
		t.Fatal("no hasher")
	}
	if other.HashEmailAddress("person@example.com") == reference {
		t.Error("another key hashes alike")
	}
}

func TestHasher_HashEmailAddress_NilHasher(t *testing.T) {
	t.Parallel()

	var hasher *Hasher
	if got := hasher.HashEmailAddress("person@example.com"); got != "" {
		t.Errorf("a nil hasher returned %q, want nothing", got)
	}
}

func TestNew_KeyIsCopied(t *testing.T) {
	t.Parallel()

	key := testKey()
	hasher, err := New(key)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if hasher == nil {
		t.Fatal("no hasher")
	}

	before := hasher.HashEmailAddress("person@example.com")
	key[0] ^= 0xff
	if after := hasher.HashEmailAddress("person@example.com"); after != before {
		t.Errorf("a change to the caller's key reached the hasher: %q, then %q", before, after)
	}
}
