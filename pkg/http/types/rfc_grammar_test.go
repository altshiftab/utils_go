package types_test

// The tests here feed each HTTP grammar the values its specification says a
// recipient has to accept, and the values it says are not well formed. They
// work on the grammars themselves rather than on the parsers built over
// them, so that a fault in a grammar is reported as a fault in that grammar.
//
// Every case cites where the requirement comes from. A case with no citation
// is a regression guard rather than a conformance one.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altshiftab/utils_go/pkg/abnf"
)

// grammarCase is one value a grammar is required to accept or to reject.
type grammarCase struct {
	name string
	// input is the field value, as it reaches a parser: RFC 9110
	// Section 5.5 has already stripped leading and trailing whitespace.
	input string
	// accepts is what the specification requires of a recipient.
	accepts bool
	// reference cites the requirement.
	reference string
}

// grammarSuite is every case for one grammar.
type grammarSuite struct {
	// directory holds the grammar, relative to pkg/http/types.
	directory string
	// root is the rule a field value is parsed from.
	root  string
	cases []*grammarCase
}

// quotedObsText is a quoted-string holding an obs-text octet, which
// RFC 9110 Appendix A admits through both qdtext and quoted-pair.
const quotedObsText = "\"\xc3\xa9\""

var grammarSuites = []*grammarSuite{
	{
		directory: "accept",
		root:      "Accept",
		cases: []*grammarCase{
			{name: "media range", input: "text/plain", accepts: true},
			{name: "wildcard", input: "*/*", accepts: true},
			{name: "subtype wildcard", input: "text/*", accepts: true},
			{name: "weight", input: "text/html;q=0.8", accepts: true},
			{
				name:      "browser value",
				input:     "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
				accepts:   true,
				reference: "RFC 9110 Section 12.5.1",
			},
			{
				name:      "empty field value",
				input:     "",
				accepts:   true,
				reference: "RFC 9110 Section 12.5.1: Accept = #( media-range [ weight ] )",
			},
			{
				name:      "empty list element",
				input:     "text/plain, ,text/html",
				accepts:   true,
				reference: "RFC 9110 Section 5.6.1: a recipient MUST parse and ignore empty list elements",
			},
			{
				name:      "trailing comma",
				input:     "text/plain,",
				accepts:   true,
				reference: "RFC 9110 Section 5.6.1",
			},
			{
				name:      "empty parameter",
				input:     "text/plain;;charset=utf-8",
				accepts:   true,
				reference: "RFC 9110 Appendix A: parameters = *( OWS \";\" OWS [ parameter ] )",
			},
			{
				name:      "obs-text in a parameter value",
				input:     "text/plain;charset=" + quotedObsText,
				accepts:   true,
				reference: "RFC 9110 Appendix A: qdtext includes obs-text",
			},
			{name: "no subtype", input: "text/", accepts: false},
			{name: "no type", input: "/plain", accepts: false},
		},
	},
	{
		directory: "accept_encoding",
		root:      "Accept-Encoding",
		cases: []*grammarCase{
			{name: "coding", input: "gzip", accepts: true},
			{name: "identity", input: "identity", accepts: true},
			{name: "wildcard", input: "*", accepts: true},
			{
				name:      "weights",
				input:     "compress;q=0.5, gzip;q=1.0",
				accepts:   true,
				reference: "RFC 9110 Section 12.5.3",
			},
			{
				name:      "empty field value",
				input:     "",
				accepts:   true,
				reference: "RFC 9110 Section 12.5.3: Accept-Encoding = #( codings [ weight ] )",
			},
			{
				name:      "empty list element",
				input:     "gzip, ,br",
				accepts:   true,
				reference: "RFC 9110 Section 5.6.1",
			},
			{name: "quoted coding", input: "\"gzip\"", accepts: false},
		},
	},
	{
		directory: "accept_language",
		root:      "Accept-Language",
		cases: []*grammarCase{
			{name: "primary subtag", input: "en", accepts: true},
			{name: "subtag", input: "en-US", accepts: true},
			{
				name:      "wildcard",
				input:     "*",
				accepts:   true,
				reference: "RFC 4647 Section 2.1: language-range = (1*8ALPHA *(\"-\" 1*8alphanum)) / \"*\"",
			},
			{
				name:      "wildcard among ranges",
				input:     "da, en-gb;q=0.8, en;q=0.7, *;q=0.5",
				accepts:   true,
				reference: "RFC 9110 Section 12.5.4 and RFC 4647 Section 2.1",
			},
			{
				name:      "empty field value",
				input:     "",
				accepts:   true,
				reference: "RFC 9110 Section 12.5.4: Accept-Language = #( language-range [ weight ] )",
			},
			{
				name:      "primary subtag over eight characters",
				input:     "englishlanguage",
				accepts:   false,
				reference: "RFC 4647 Section 2.1: 1*8ALPHA",
			},
			{name: "digits in the primary subtag", input: "e1", accepts: false},
		},
	},
	{
		directory: "authorization",
		root:      "Authorization",
		cases: []*grammarCase{
			{
				name:      "token68",
				input:     "Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ==",
				accepts:   true,
				reference: "RFC 9110 Section 11.4 and Section 11.6.2",
			},
			{
				name:      "auth-params",
				input:     "Newauth realm=\"apps\", type=1, title=\"Login to apps\"",
				accepts:   true,
				reference: "RFC 9110 Section 11.6.1",
			},
			{name: "scheme alone", input: "Negotiate", accepts: true},
			{
				name:      "obs-text in a parameter value",
				input:     "Basic realm=" + quotedObsText,
				accepts:   true,
				reference: "RFC 9110 Appendix A: qdtext includes obs-text",
			},
			{name: "no scheme", input: "=abc", accepts: false},
		},
	},
	{
		directory: "cache_control",
		root:      "Cache-Control",
		cases: []*grammarCase{
			{name: "directive", input: "no-cache", accepts: true},
			{name: "directive with a value", input: "max-age=604800", accepts: true},
			{
				name:      "quoted directive value",
				input:     "private=\"Set-Cookie\"",
				accepts:   true,
				reference: "RFC 9111 Section 5.2: cache-directive = token [ \"=\" ( token / quoted-string ) ]",
			},
			{
				name:      "several directives",
				input:     "public, max-age=604800, immutable",
				accepts:   true,
				reference: "RFC 9111 Section 5.2",
			},
			{
				name:      "empty field value",
				input:     "",
				accepts:   true,
				reference: "RFC 9111 Section 5.2: Cache-Control = #cache-directive",
			},
			{
				name:      "obs-text in a directive value",
				input:     "private=" + quotedObsText,
				accepts:   true,
				reference: "RFC 9110 Appendix A: qdtext includes obs-text",
			},
		},
	},
	{
		directory: "content_type",
		root:      "Content-Type",
		cases: []*grammarCase{
			{name: "media type", input: "text/html", accepts: true},
			{
				name:      "parameter",
				input:     "text/html;charset=utf-8",
				accepts:   true,
				reference: "RFC 9110 Section 8.3",
			},
			{
				name:      "whitespace around the parameter",
				input:     "text/html ; charset=utf-8",
				accepts:   true,
				reference: "RFC 9110 Appendix A: parameters = *( OWS \";\" OWS [ parameter ] )",
			},
			{
				name:      "empty parameter",
				input:     "text/html;;charset=utf-8",
				accepts:   true,
				reference: "RFC 9110 Appendix A: the parameter of parameters is optional",
			},
			{
				name:      "quoted boundary",
				input:     "multipart/form-data; boundary=\"---abc---\"",
				accepts:   true,
				reference: "RFC 9110 Section 8.3.1",
			},
			{
				name:      "obs-text in a parameter value",
				input:     "text/plain;charset=" + quotedObsText,
				accepts:   true,
				reference: "RFC 9110 Appendix A: qdtext includes obs-text",
			},
			{name: "no subtype", input: "text", accepts: false},
			{name: "trailing slash", input: "text/", accepts: false},
		},
	},
	{
		directory: "etag",
		root:      "ETag",
		cases: []*grammarCase{
			{
				name:      "strong",
				input:     "\"xyzzy\"",
				accepts:   true,
				reference: "RFC 9110 Section 8.8.3",
			},
			{
				name:      "weak",
				input:     "W/\"xyzzy\"",
				accepts:   true,
				reference: "RFC 9110 Section 8.8.3",
			},
			{
				name:      "empty",
				input:     "\"\"",
				accepts:   true,
				reference: "RFC 9110 Section 8.8.3: opaque-tag = DQUOTE *etagc DQUOTE",
			},
			{
				name:      "lowercase weakness marker",
				input:     "w/\"xyzzy\"",
				accepts:   false,
				reference: "RFC 9110 Appendix A: weak = %x57.2F, which is case-sensitive",
			},
			{name: "unquoted", input: "xyzzy", accepts: false},
		},
	},
	{
		directory: "forwarded",
		root:      "Forwarded",
		cases: []*grammarCase{
			{
				name:      "one pair",
				input:     "for=\"_gazonk\"",
				accepts:   true,
				reference: "RFC 7239 Section 4",
			},
			{
				name:      "several pairs",
				input:     "for=192.0.2.43, for=198.51.100.17",
				accepts:   true,
				reference: "RFC 7239 Section 4",
			},
			{
				name:      "several parameters",
				input:     "for=192.0.2.60;proto=http;by=203.0.113.43",
				accepts:   true,
				reference: "RFC 7239 Section 4",
			},
			{
				name:      "obs-text in a value",
				input:     "for=" + quotedObsText,
				accepts:   true,
				reference: "RFC 7239 Section 4, over the quoted-string of RFC 9110",
			},
			{name: "no value", input: "for=", accepts: false},
		},
	},
	{
		directory: "retry_after",
		root:      "Retry-After",
		cases: []*grammarCase{
			{
				name:      "delay-seconds",
				input:     "120",
				accepts:   true,
				reference: "RFC 9110 Section 10.2.3",
			},
			{
				name:      "IMF-fixdate",
				input:     "Fri, 31 Dec 1999 23:59:59 GMT",
				accepts:   true,
				reference: "RFC 9110 Section 5.6.7",
			},
			{
				name:      "rfc850-date",
				input:     "Sunday, 06-Nov-94 08:49:37 GMT",
				accepts:   true,
				reference: "RFC 9110 Section 5.6.7: a recipient MUST accept all three HTTP-date formats",
			},
			{
				name:      "asctime-date",
				input:     "Sun Nov  6 08:49:37 1994",
				accepts:   true,
				reference: "RFC 9110 Section 5.6.7: a recipient MUST accept all three HTTP-date formats",
			},
			{
				name:      "asctime-date with a two-digit day",
				input:     "Sun Nov 16 08:49:37 1994",
				accepts:   true,
				reference: "RFC 9110 Appendix A: date3 = month SP ( 2DIGIT / ( SP DIGIT ) )",
			},
			{
				name:      "lowercase day name",
				input:     "fri, 31 Dec 1999 23:59:59 GMT",
				accepts:   false,
				reference: "RFC 9110 Appendix A: day-name is case-sensitive",
			},
			{name: "negative delay", input: "-1", accepts: false},
		},
	},
	{
		directory: "strict_transport_security",
		root:      "Strict-Transport-Security",
		cases: []*grammarCase{
			{
				name:      "max-age",
				input:     "max-age=31536000",
				accepts:   true,
				reference: "RFC 6797 Section 6.1",
			},
			{
				name:      "subdomains",
				input:     "max-age=15768000 ; includeSubDomains",
				accepts:   true,
				reference: "RFC 6797 Section 6.1.2",
			},
			{
				name:      "quoted value",
				input:     "max-age=\"31536000\"",
				accepts:   true,
				reference: "RFC 6797 Section 6.1: directive-value = token | quoted-string",
			},
			{
				name:      "obs-text in a directive value",
				input:     "max-age=" + quotedObsText,
				accepts:   true,
				reference: "RFC 6797 Section 6.1, over the quoted-string of RFC 2616",
			},
		},
	},
	{
		directory: "content_disposition",
		root:      "content-disposition",
		cases: []*grammarCase{
			{
				name:      "inline",
				input:     "inline",
				accepts:   true,
				reference: "RFC 6266 Section 4.1",
			},
			{
				name:      "filename",
				input:     "attachment; filename=\"filename.jpg\"",
				accepts:   true,
				reference: "RFC 6266 Section 4.3",
			},
			{
				name:      "ext-value",
				input:     "attachment; filename*=UTF-8''%e2%82%ac%20rates",
				accepts:   true,
				reference: "RFC 6266 Section 5",
			},
			{
				name:      "both filename forms",
				input:     "attachment; filename=\"EURO rates\"; filename*=utf-8''%e2%82%ac%20rates",
				accepts:   true,
				reference: "RFC 6266 Section 5",
			},
			{
				name:      "obs-text in a parameter value",
				input:     "attachment; filename=" + quotedObsText,
				accepts:   true,
				reference: "RFC 6266 Section 4.1, over the quoted-string of RFC 2616",
			},
			{name: "no disposition type", input: "; filename=\"a\"", accepts: false},
		},
	},
}

func TestGrammarsAgainstSpecifications(t *testing.T) {
	t.Parallel()

	for _, suite := range grammarSuites {
		t.Run(suite.directory, func(t *testing.T) {
			t.Parallel()

			definition, err := os.ReadFile(filepath.Join(suite.directory, "grammar.abnf"))
			if err != nil {
				t.Fatalf("os read file: %v", err)
			}

			grammar, err := abnf.ParseABNF(definition)
			if err != nil {
				t.Fatalf("abnf parse abnf: %v", err)
			}

			for _, grammarCase := range suite.cases {
				t.Run(grammarCase.name, func(t *testing.T) {
					t.Parallel()

					paths, err := abnf.Parse([]byte(grammarCase.input), grammar, suite.root)
					if err != nil {
						t.Fatalf("abnf parse: %v", err)
					}

					if accepts := len(paths) != 0; accepts != grammarCase.accepts {
						reference := grammarCase.reference
						if reference == "" {
							reference = "no citation, a regression guard"
						}
						t.Fatalf(
							"%q: expected accepts=%t, got accepts=%t (%s)",
							grammarCase.input,
							grammarCase.accepts,
							accepts,
							reference,
						)
					}
				})
			}
		})
	}
}
