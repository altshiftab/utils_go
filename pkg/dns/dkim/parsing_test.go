package dkim

import (
	"errors"
	"testing"

	"github.com/altshiftab/utils_go/pkg/abnf"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/testing/cmp"
)

func TestParseRecord(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		input          []byte
		expected       *Record
		expectedErrors []error
	}{
		{
			name:           "empty data",
			input:          nil,
			expected:       nil,
			expectedErrors: []error{motmedelErrors.ErrSyntaxError},
		},
		{
			name:           "syntax error",
			input:          []byte("garbage"),
			expected:       nil,
			expectedErrors: []error{motmedelErrors.ErrSyntaxError},
		},
		{
			name:  "basic record",
			input: []byte("v=DKIM1; p=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDDmzRmJRQxLEuyYiyMg4suA2SyMwR5MGHpP9diNT1hRiwUd/mZp1ro7kIDTKS8ttkI6z6eTRW9e9dDOxzSxNuXmume60Cjbu08gOyhPG3GfWdg7QkdN6kR4V75MFlw624VY35DaXBvnlTJTgRg/EW72O1DiYVThkyCgpSYS8nmEQIDAQAB"),
			expected: &Record{
				Version:       1,
				PublicKeyData: "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDDmzRmJRQxLEuyYiyMg4suA2SyMwR5MGHpP9diNT1hRiwUd/mZp1ro7kIDTKS8ttkI6z6eTRW9e9dDOxzSxNuXmume60Cjbu08gOyhPG3GfWdg7QkdN6kR4V75MFlw624VY35DaXBvnlTJTgRg/EW72O1DiYVThkyCgpSYS8nmEQIDAQAB",
			},
		},
		{
			name:  "advanced record",
			input: []byte("k=rsa; t=s; p=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDDmzRmJRQxLEuyYiyMg4suA2SyMwR5MGHpP9diNT1hRiwUd/mZp1ro7kIDTKS8ttkI6z6eTRW9e9dDOxzSxNuXmume60Cjbu08gOyhPG3GfWdg7QkdN6kR4V75MFlw624VY35DaXBvnlTJTgRg/EW72O1DiYVThkyCgpSYS8nmEQIDAQAB"),
			expected: &Record{
				KeyType:       "rsa",
				Flags:         []string{"s"},
				PublicKeyData: "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDDmzRmJRQxLEuyYiyMg4suA2SyMwR5MGHpP9diNT1hRiwUd/mZp1ro7kIDTKS8ttkI6z6eTRW9e9dDOxzSxNuXmume60Cjbu08gOyhPG3GfWdg7QkdN6kR4V75MFlw624VY35DaXBvnlTJTgRg/EW72O1DiYVThkyCgpSYS8nmEQIDAQAB",
			},
		},
		{
			name:  "advanced record #2",
			input: []byte("v=DKIM1; k=other-value; h=a:b:c; s=*; t=d:e:f; n=hello; p=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDKNbD2WnGf/DPSHXhpc4LnZ9s6P8xWoc0PDQUOdNPy78RR1W8DNS4v7xwP8e0BySw0WUafDrOUmUiCePAh23c1ApfzzDTO8bBwc9Jlb0HYmEOQ3JgHeQx4zGNdE8VzzV8ArXTbc6k/2oaVrVvu9xNSU9bU3ZAMFwav1n9Gmua4cwIDAQAB"),
			expected: &Record{
				Version:                  1,
				AcceptableHashAlgorithms: []string{"a", "b", "c"},
				KeyType:                  "other-value",
				ServiceType:              "*",
				Flags:                    []string{"d", "e", "f"},
				Notes:                    "hello",
				PublicKeyData:            "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDKNbD2WnGf/DPSHXhpc4LnZ9s6P8xWoc0PDQUOdNPy78RR1W8DNS4v7xwP8e0BySw0WUafDrOUmUiCePAh23c1ApfzzDTO8bBwc9Jlb0HYmEOQ3JgHeQx4zGNdE8VzzV8ArXTbc6k/2oaVrVvu9xNSU9bU3ZAMFwav1n9Gmua4cwIDAQAB",
			},
		},
		{
			name:  "extensions",
			input: []byte("p= ; extra=123; super=456"),
			expected: &Record{
				Extensions: [][2]string{
					{"extra", "123"},
					{"super", "456"},
				},
			},
		},
		{
			name:     "empty p",
			input:    []byte("p="),
			expected: &Record{},
		},
		{
			name:           "misplaced v",
			input:          []byte("p= ; v=DKIM1"),
			expectedErrors: []error{motmedelErrors.ErrSemanticError, ErrVNotFirstTag},
		},
		{
			name:           "missing p",
			input:          []byte("v=DKIM1"),
			expectedErrors: []error{motmedelErrors.ErrSemanticError, ErrMissingPublicKeyData},
		},
		{
			name:           "duplicate tags",
			input:          []byte("p= ; p="),
			expectedErrors: []error{motmedelErrors.ErrSemanticError, ErrDuplicateTags},
		},
		{
			name:           "malformed tag",
			input:          []byte("v=garbage"),
			expectedErrors: []error{motmedelErrors.ErrSyntaxError, ErrMalformedTag},
		},
		{
			name:           "malformed p data",
			input:          []byte("p=11qYAYKxCrfVS/7TyWQHOg7hcvPapiMlrwIaaPcHURo="),
			expectedErrors: []error{motmedelErrors.ErrSemanticError, ErrMalformedPublicKeyData},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dkimRecord, err := ParseRecord(testCase.input)
			expectedErrors := testCase.expectedErrors

			if len(expectedErrors) == 0 && err != nil {
				t.Fatalf("expected no errors, got: %v", err)
			}

			if !motmedelErrors.IsAll(err, expectedErrors...) {
				t.Fatalf("expected errors: %v, got: %v", expectedErrors, err)
			}

			expected := testCase.expected
			if expected != nil {
				expected.Raw = string(testCase.input)
			}

			if diff := cmp.Diff(expected, dkimRecord); diff != "" {
				t.Fatalf("struct mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestParseHeader(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		input          []byte
		expected       *Header
		expectedErrors []error
	}{
		{
			name:           "empty data",
			input:          nil,
			expected:       nil,
			expectedErrors: []error{motmedelErrors.ErrSyntaxError},
		},
		{
			name:           "syntax error",
			input:          []byte("garbage"),
			expected:       nil,
			expectedErrors: []error{motmedelErrors.ErrSyntaxError},
		},
		{
			name:  "extensions and empty tags",
			input: []byte("v=1; a=rsa-sha256; b=AAAA; bh=AAAA; d=example.net; h=from; s=brisbane; l=42; t=1; x=2; q=dns/txt; i=@example.net; c=relaxed/simple; extra=foo; empty="),
			expected: &Header{
				Version:                 1,
				Algorithm:               "rsa-sha256",
				Signature:               "AAAA",
				Hash:                    "AAAA",
				SigningDomainIdentifier: "example.net",
				SignedHeaderFields:      []string{"from"},
				Selector:                "brisbane",
				BodyLengthCount:         "42",
				SignatureTimestamp:      "1",
				SignatureExpiration:     "2",
				QueryMethods:            []string{"dns/txt"},
				AgentOrUserIdentifier:   "@example.net",
				MessageCanonicalization: "relaxed/simple",
				Extensions: [][2]string{
					{"extra", "foo"},
					{"empty", ""},
				},
			},
		},
		{
			name:           "malformed signature tag",
			input:          []byte("v=1; a=1; b=AAAA; bh=AAAA; d=example.net; h=from; s=brisbane"),
			expectedErrors: []error{ErrMalformedTag},
		},
		{
			name:           "missing required tag h",
			input:          []byte("v=1; a=rsa-sha256; b=AAAA; bh=AAAA; d=example.net; s=brisbane"),
			expectedErrors: []error{motmedelErrors.ErrSemanticError, ErrMissingRequiredTag},
		},
		{
			name:           "bad base64 in b",
			input:          []byte("v=1; a=rsa-sha256; b=A; bh=AAAA; d=example.net; h=from; s=brisbane"),
			expectedErrors: []error{motmedelErrors.ErrSemanticError},
		},
		{
			name:           "bad base64 in bh",
			input:          []byte("v=1; a=rsa-sha256; b=AAAA; bh=A; d=example.net; h=from; s=brisbane"),
			expectedErrors: []error{motmedelErrors.ErrSemanticError},
		},
		{
			name:  "rfc example",
			input: []byte("v=1; a=rsa-sha256; d=example.net; s=brisbane;\n      c=simple; q=dns/txt; i=@eng.example.net;\n      t=1117574938; x=1118006938;\n      h=from:to:subject:date;\n      z=From:foo@eng.example.net|To:joe@example.com|\n       Subject:demo=20run|Date:July=205,=202005=203:44:08=20PM=20-0700;\n      bh=MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=;\n      b=dzdVyOfAKCdLXdJOc9G2q8LoXSlEniSbav+yuU4zGeeruD00lszZVoG4ZHRNiYzR"),
			expected: &Header{
				Version:                 1,
				Algorithm:               "rsa-sha256",
				SigningDomainIdentifier: "example.net",
				Selector:                "brisbane",
				MessageCanonicalization: "simple",
				QueryMethods:            []string{"dns/txt"},
				AgentOrUserIdentifier:   "@eng.example.net",
				SignatureTimestamp:      "1117574938",
				SignatureExpiration:     "1118006938",
				SignedHeaderFields:      []string{"from", "to", "subject", "date"},
				CopiedHeaderFields: [][2]string{
					{"From", "foo@eng.example.net"},
					{"To", "joe@example.com"},
					{"Subject", "demo=20run"},
					{"Date", "July=205,=202005=203:44:08=20PM=20-0700"},
				},
				Hash:      "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=",
				Signature: "dzdVyOfAKCdLXdJOc9G2q8LoXSlEniSbav+yuU4zGeeruD00lszZVoG4ZHRNiYzR",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			header, err := ParseHeader(testCase.input)
			expectedErrors := testCase.expectedErrors

			if len(expectedErrors) == 0 && err != nil {
				t.Fatalf("expected no errors, got: %v", err)
			}

			if !motmedelErrors.IsAll(err, expectedErrors...) {
				t.Fatalf("expected errors: %v, got: %v", expectedErrors, err)
			}

			expected := testCase.expected
			if expected != nil {
				expected.Raw = string(testCase.input)
			}

			if diff := cmp.Diff(expected, header); diff != "" {
				t.Fatalf("struct mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestExtractTagPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		tagNameInput   string
		tagValueInput  []byte
		tagTypeInput   string
		expected       *abnf.Path
		expectedErrors []error
		checkErr       func(*testing.T, error)
	}{
		{
			name: "empty tag name",
		},
		{
			name: "empty tag value",
		},
		{
			name:          "empty tag type",
			tagNameInput:  "x",
			tagValueInput: []byte("x"),
			checkErr: func(t *testing.T, err error) {
				ee, ok := errors.AsType[*empty_error.Error](err)
				if !ok {
					t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
				}
				if ee.Field != "tag type" {
					t.Errorf("Field = %q, want %q", ee.Field, "tag type")
				}
			},
		},
		{
			name:           "unexpected tag type",
			tagNameInput:   "x",
			tagValueInput:  []byte("x"),
			tagTypeInput:   "other",
			expectedErrors: []error{ErrUnexpectedTagType},
		},
		{
			name:          "unknown key tag returns nil",
			tagNameInput:  "unknown",
			tagValueInput: []byte("foo"),
			tagTypeInput:  "key",
		},
		{
			name:          "unknown signature tag returns nil",
			tagNameInput:  "unknown",
			tagValueInput: []byte("foo"),
			tagTypeInput:  "signature",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path, err := extractTagPath(testCase.tagNameInput, testCase.tagValueInput, testCase.tagTypeInput)
			expectedErrors := testCase.expectedErrors

			if len(expectedErrors) == 0 && testCase.checkErr == nil && err != nil {
				t.Fatalf("expected no errors, got: %v", err)
			}

			if !motmedelErrors.IsAll(err, expectedErrors...) {
				t.Fatalf("expected errors: %v, got: %v", expectedErrors, err)
			}

			if testCase.checkErr != nil {
				testCase.checkErr(t, err)
			}

			expected := testCase.expected
			if diff := cmp.Diff(expected, path); diff != "" {
				t.Fatalf("struct mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}

func TestGetTagSpecItems(t *testing.T) {
	t.Parallel()

	t.Run("nil tag map yields error and stops", func(t *testing.T) {
		t.Parallel()
		var seen []error
		for item, err := range getTagSpecItems(nil, nil, []byte("p=A")) {
			seen = append(seen, err)
			if item != nil {
				t.Errorf("expected nil item, got %v", item)
			}
		}
		if len(seen) != 1 {
			t.Fatalf("expected exactly one yield, got %d", len(seen))
		}
		ne, ok := errors.AsType[*nil_error.Error](seen[0])
		if !ok {
			t.Fatalf("err type = %T (%v), want *nil_error.Error", seen[0], seen[0])
		}
		if ne.Field != "tag map" {
			t.Errorf("Field = %q, want %q", ne.Field, "tag map")
		}
	})

	t.Run("empty data yields error and stops", func(t *testing.T) {
		t.Parallel()
		var seen []error
		for item, err := range getTagSpecItems(nil, map[string]struct{}{}, nil) {
			seen = append(seen, err)
			if item != nil {
				t.Errorf("expected nil item, got %v", item)
			}
		}
		if len(seen) != 1 {
			t.Fatalf("expected exactly one yield, got %d", len(seen))
		}
		ee, ok := errors.AsType[*empty_error.Error](seen[0])
		if !ok {
			t.Fatalf("err type = %T (%v), want *empty_error.Error", seen[0], seen[0])
		}
		if ee.Field != "path input" {
			t.Errorf("Field = %q, want %q", ee.Field, "path input")
		}
	})
}

func TestExtractBase64String(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		pathInput      *abnf.Path
		valueInput     []byte
		expected       string
		expectedErrors []error
		checkErr       func(*testing.T, error)
	}{
		{
			name: "empty path",
		},
		{
			name:      "empty value",
			pathInput: &abnf.Path{},
			checkErr: func(t *testing.T, err error) {
				ee, ok := errors.AsType[*empty_error.Error](err)
				if !ok {
					t.Fatalf("err type = %T (%v), want *empty_error.Error", err, err)
				}
				if ee.Field != "path input" {
					t.Errorf("Field = %q, want %q", ee.Field, "path input")
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path, err := extractBase64String(testCase.pathInput, testCase.valueInput)
			expectedErrors := testCase.expectedErrors

			if len(expectedErrors) == 0 && testCase.checkErr == nil && err != nil {
				t.Fatalf("expected no errors, got: %v", err)
			}

			if !motmedelErrors.IsAll(err, expectedErrors...) {
				t.Fatalf("expected errors: %v, got: %v", expectedErrors, err)
			}

			if testCase.checkErr != nil {
				testCase.checkErr(t, err)
			}

			expected := testCase.expected
			if diff := cmp.Diff(expected, path); diff != "" {
				t.Fatalf("struct mismatch (-expected +got):\n%s", diff)
			}
		})
	}
}
