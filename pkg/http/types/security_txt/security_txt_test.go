package security_txt

import (
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		// expectedContacts, expectedExpires and expectedLanguages are what the file
		// means.
		expectedContacts  []string
		expectedExpires   string
		expectedLanguages []string
		// expectedFields is how many field lines it holds, comments and blank lines
		// excluded.
		expectedFields int
		// expectedMalformed are the field names whose value does not match the
		// grammar the field requires.
		expectedMalformed []string
		expectedSigned    bool
		expectError       bool
	}{
		{
			name:             "the fields of an ordinary file",
			input:            "Contact: mailto:security@example.com\nExpires: 2030-01-01T00:00:00Z\n",
			expectedContacts: []string{"mailto:security@example.com"},
			expectedExpires:  "2030-01-01T00:00:00Z",
			expectedFields:   2,
		},
		{
			// The published grammar puts contact-field before expires-field, and the
			// prose says the order does not matter. This follows the prose.
			name:             "expires before contact",
			input:            "Expires: 2030-01-01T00:00:00Z\nContact: mailto:security@example.com\n",
			expectedContacts: []string{"mailto:security@example.com"},
			expectedExpires:  "2030-01-01T00:00:00Z",
			expectedFields:   2,
		},
		{
			// Errata 7743: the grammar says CRLF, section 2.2 says CRLF or LF.
			name:             "crlf line endings",
			input:            "Contact: mailto:a@example.com\r\nExpires: 2030-01-01T00:00:00Z\r\n",
			expectedContacts: []string{"mailto:a@example.com"},
			expectedExpires:  "2030-01-01T00:00:00Z",
			expectedFields:   2,
		},
		{
			name:             "no newline on the last line",
			input:            "Contact: mailto:a@example.com",
			expectedContacts: []string{"mailto:a@example.com"},
			expectedFields:   1,
		},
		{
			name: "comments, blank lines and trailing whitespace",
			input: "# who to tell\nContact: mailto:a@example.com   \n\n" +
				"   \n# and when this stops being true\nExpires: 2030-01-01T00:00:00Z\n",
			expectedContacts: []string{"mailto:a@example.com"},
			expectedExpires:  "2030-01-01T00:00:00Z",
			expectedFields:   2,
		},
		{
			// RFC 5234 makes an ABNF quoted string case-insensitive, so the field
			// names in the section 4 grammar are too.
			name:             "field names in any case",
			input:            "CONTACT: mailto:a@example.com\nexpires: 2030-01-01T00:00:00Z\n",
			expectedContacts: []string{"mailto:a@example.com"},
			expectedExpires:  "2030-01-01T00:00:00Z",
			expectedFields:   2,
		},
		{
			// RFC 3339 writes "T" and "Z" as case-insensitive quoted strings and
			// RFC 9116's own example uses a lowercase "z" (errata 7264). Go's
			// RFC3339 layout takes only the uppercase forms, so this is the case
			// that silently loses an Expires if it is not normalised. Two of the
			// first four large sites tried write it this way.
			name:             "a lowercase zulu marker",
			input:            "Contact: mailto:a@example.com\nExpires: 2030-04-01T00:00:00z\n",
			expectedContacts: []string{"mailto:a@example.com"},
			expectedExpires:  "2030-04-01T00:00:00Z",
			expectedFields:   2,
		},
		{
			name:             "a lowercase date-time separator",
			input:            "Contact: mailto:a@example.com\nExpires: 2030-04-01t00:00:00Z\n",
			expectedContacts: []string{"mailto:a@example.com"},
			expectedExpires:  "2030-04-01T00:00:00Z",
			expectedFields:   2,
		},
		{
			name:             "an offset rather than zulu",
			input:            "Contact: mailto:a@example.com\nExpires: 2030-04-01T00:00:00+02:00\n",
			expectedContacts: []string{"mailto:a@example.com"},
			expectedExpires:  "2030-04-01T00:00:00+02:00",
			expectedFields:   2,
		},
		{
			name: "contacts keep the order they were written in",
			input: "Contact: https://example.com/first\nContact: mailto:second@example.com\n" +
				"Contact: tel:+46000000000\n",
			expectedContacts: []string{"https://example.com/first", "mailto:second@example.com", "tel:+46000000000"},
			expectedFields:   3,
		},
		{
			name:              "preferred languages",
			input:             "Contact: mailto:a@example.com\nPreferred-Languages: en, sv , de-DE\n",
			expectedContacts:  []string{"mailto:a@example.com"},
			expectedLanguages: []string{"en", "sv", "de-DE"},
			expectedFields:    2,
		},
		{
			name:              "a contact that is not a uri",
			input:             "Contact: security@example.com\n",
			expectedFields:    1,
			expectedMalformed: []string{"Contact"},
		},
		{
			name:              "an expires that is not a timestamp",
			input:             "Contact: mailto:a@example.com\nExpires: next tuesday\n",
			expectedContacts:  []string{"mailto:a@example.com"},
			expectedFields:    2,
			expectedMalformed: []string{"Expires"},
		},
		{
			// A field name nothing has registered carries free text, and cannot be
			// malformed. Mozilla serves one of these in place of a Contact.
			name:             "an unregistered field",
			input:            "Email: security@example.com\nContact: mailto:a@example.com\n",
			expectedContacts: []string{"mailto:a@example.com"},
			expectedFields:   2,
		},
		{
			name: "a signed file",
			input: "-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA256\n\n" +
				"Contact: mailto:a@example.com\nExpires: 2030-01-01T00:00:00Z\n" +
				"-----BEGIN PGP SIGNATURE-----\n\nabcd\n-----END PGP SIGNATURE-----\n",
			expectedContacts: []string{"mailto:a@example.com"},
			expectedExpires:  "2030-01-01T00:00:00Z",
			expectedFields:   2,
			expectedSigned:   true,
		},
		{
			// A cleartext signature escapes a leading dash, and those two
			// characters are not part of what was signed.
			name: "a signed file with an escaped dash",
			input: "-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA256\n\n" +
				"- # -----a comment beginning with dashes\nContact: mailto:a@example.com\n" +
				"-----BEGIN PGP SIGNATURE-----\n\nabcd\n-----END PGP SIGNATURE-----\n",
			expectedContacts: []string{"mailto:a@example.com"},
			expectedFields:   1,
			expectedSigned:   true,
		},
		{name: "empty", input: "", expectedFields: 0},
		{name: "a line that is neither field nor comment", input: "this is not a field\n", expectError: true},
		{name: "a field with no space after the colon", input: "Contact:mailto:a@example.com\n", expectError: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := Parse([]byte(testCase.input))

			if testCase.expectError {
				if err == nil {
					t.Fatalf("Parse(%q) was accepted, and should not have been", testCase.input)
				}

				return
			}

			if err != nil {
				t.Fatalf("Parse(%q) error = %v", testCase.input, err)
			}

			if len(parsed.Fields) != testCase.expectedFields {
				t.Errorf("fields = %d, want %d", len(parsed.Fields), testCase.expectedFields)
			}

			if parsed.Signed != testCase.expectedSigned {
				t.Errorf("signed = %v, want %v", parsed.Signed, testCase.expectedSigned)
			}

			if got := strings.Join(parsed.SecurityTxt.Contacts, "|"); got != strings.Join(testCase.expectedContacts, "|") {
				t.Errorf("contacts = %v, want %v", parsed.SecurityTxt.Contacts, testCase.expectedContacts)
			}

			if got := strings.Join(parsed.SecurityTxt.PreferredLanguages, "|"); got != strings.Join(testCase.expectedLanguages, "|") {
				t.Errorf("languages = %v, want %v", parsed.SecurityTxt.PreferredLanguages, testCase.expectedLanguages)
			}

			switch testCase.expectedExpires {
			case "":
				if !parsed.SecurityTxt.Expires.IsZero() {
					t.Errorf("expires = %v, want none", parsed.SecurityTxt.Expires)
				}
			default:
				expected, err := time.Parse(time.RFC3339, testCase.expectedExpires)
				if err != nil {
					t.Fatalf("the case's own expected timestamp does not parse: %v", err)
				}

				if !parsed.SecurityTxt.Expires.Equal(expected) {
					t.Errorf("expires = %v, want %v", parsed.SecurityTxt.Expires, expected)
				}
			}

			malformed := map[string]bool{}
			for _, field := range parsed.Fields {
				if field.Malformed {
					malformed[field.Name] = true
				}
			}

			for _, name := range testCase.expectedMalformed {
				if !malformed[name] {
					t.Errorf("%s was accepted, and its value does not match the grammar", name)
				}
			}

			if len(malformed) != len(testCase.expectedMalformed) {
				t.Errorf("%d fields were malformed, want %d", len(malformed), len(testCase.expectedMalformed))
			}
		})
	}
}

// A caller reports a field that appears more often than RFC 9116 allows, so the
// parse has to keep the repeats rather than collapsing them.
func TestParseKeepsRepeats(t *testing.T) {
	t.Parallel()

	parsed, err := Parse([]byte(
		"Contact: mailto:a@example.com\nExpires: 2030-01-01T00:00:00Z\nExpires: 2031-01-01T00:00:00Z\n",
	))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	expires := parsed.Get(FieldExpires)
	if len(expires) != 2 {
		t.Fatalf("Get(%q) = %d fields, want 2", FieldExpires, len(expires))
	}

	// The first wins, as the one a reader would act on.
	expected, err := time.Parse(time.RFC3339, "2030-01-01T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}

	if !parsed.SecurityTxt.Expires.Equal(expected) {
		t.Errorf("expires = %v, want the first of the two, %v", parsed.SecurityTxt.Expires, expected)
	}
}

func TestParseRecordsLineNumbers(t *testing.T) {
	t.Parallel()

	parsed, err := Parse([]byte("# a comment\n\nContact: mailto:a@example.com\n\nExpires: 2030-01-01T00:00:00Z\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	testCases := []struct {
		name         string
		field        string
		expectedLine int
	}{
		{name: "contact", field: FieldContact, expectedLine: 3},
		{name: "expires", field: FieldExpires, expectedLine: 5},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			found := parsed.Get(testCase.field)
			if len(found) != 1 {
				t.Fatalf("Get(%q) = %d fields, want 1", testCase.field, len(found))
			}

			if found[0].Line != testCase.expectedLine {
				t.Errorf("line = %d, want %d", found[0].Line, testCase.expectedLine)
			}
		})
	}
}

func TestGet(t *testing.T) {
	t.Parallel()

	parsed, err := Parse([]byte("Contact: mailto:a@example.com\nCONTACT: https://example.com/b\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	testCases := []struct {
		name     string
		parsed   *Parsed
		field    string
		expected int
	}{
		{name: "both casings of a name", parsed: parsed, field: "contact", expected: 2},
		{name: "a field that is not there", parsed: parsed, field: FieldPolicy, expected: 0},
		{name: "nothing parsed at all", parsed: nil, field: FieldContact, expected: 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := len(testCase.parsed.Get(testCase.field)); got != testCase.expected {
				t.Errorf("Get(%q) = %d fields, want %d", testCase.field, got, testCase.expected)
			}
		})
	}
}
