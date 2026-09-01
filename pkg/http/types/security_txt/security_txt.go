// Package security_txt reads the file a host serves at /.well-known/security.txt to
// say how a vulnerability in it should be reported. RFC 9116.
//
// The grammar here is RFC 9116 section 4 with three deviations, each deliberate:
//
//   - Fields are not ordered. The published `unsigned` production requires a Contact
//     line before the Expires line, because `line` expands to `field`, which holds
//     contact-field but neither expires-field nor lang-field. The prose disagrees --
//     "order of fields within the file is not important except that if contact-field
//     appears more than once, the order of those indicates priority" -- so a file
//     that states Expires first is legal prose and illegal grammar. This accepts it.
//
//   - A line may end with a bare LF. The published grammar requires CRLF, while
//     section 2.2 permits "either a carriage return and line feed characters (CRLF)
//     or just a line feed character (LF)". That is errata 7743, reported and not yet
//     verified; this follows the prose, since files with LF endings are ordinary.
//
//   - A field is parsed as a name and a value, and the value is then held against the
//     grammar the field requires -- a URI for Contact, a timestamp for Expires. The
//     published grammar gives each field its own production, which makes every field
//     also match ext-field and leaves the parse ambiguous. Splitting it in two says
//     the same thing without the ambiguity, and lets a malformed value be reported as
//     a malformed value rather than as a file that does not parse.
//
// What the grammar does not do is decide whether the file is any good. A file with no
// Contact, or an Expires in the past, parses: RFC 9116 requires both, but a caller
// that wants to report their absence needs the parse to succeed first. Validity is
// the caller's to judge.
package security_txt

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/altshiftab/utils_go/pkg/abnf"
	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

//go:embed grammar.abnf
var grammar []byte

var Grammar *abnf.Grammar

// Field names, as RFC 9116 registers them. ABNF quoted strings are case-insensitive,
// so the grammar accepts any casing and the comparison here has to match.
const (
	FieldAcknowledgments    = "Acknowledgments"
	FieldCanonical          = "Canonical"
	FieldContact            = "Contact"
	FieldCsaf               = "CSAF"
	FieldEncryption         = "Encryption"
	FieldExpires            = "Expires"
	FieldHiring             = "Hiring"
	FieldPolicy             = "Policy"
	FieldPreferredLanguages = "Preferred-Languages"
)

// uriFields are the fields whose value RFC 9116 requires to be a URI. CSAF is not in
// the section 4 grammar -- it was registered afterwards, so the published grammar
// matches it as an ext-field -- but it carries a URI like the rest.
var uriFields = map[string]bool{
	FieldAcknowledgments: true,
	FieldCanonical:       true,
	FieldContact:         true,
	FieldCsaf:            true,
	FieldEncryption:      true,
	FieldHiring:          true,
	FieldPolicy:          true,
}

// Field is one line of the file, kept as it was written.
//
// It exists beside the parsed SecurityTxt because the checks a caller makes need what
// the parsed form throws away: which fields appeared and how often, so that a second
// Expires can be reported, and whether each value was well formed, so that a
// malformed one can be named rather than silently dropped.
type Field struct {
	// Name is as written, so a report can quote it. Compare it with strings.EqualFold.
	Name string
	// Value is the field value, trimmed of the whitespace RFC 9116 allows around it.
	Value string
	// Line is the one-based line the field was found on.
	Line int
	// Malformed is set when the value does not match the grammar the field requires.
	// The field is still returned: what is wrong with it is worth reporting.
	Malformed bool
}

// Parsed is everything a caller could want out of the file: the values, and the
// shape of what they were written in.
type Parsed struct {
	// SecurityTxt is the file's meaning, with each field's values in the order they
	// appeared.
	SecurityTxt *altshiftHttpTypes.SecurityTxt
	// Fields is every field line, in order, including repeats and unregistered
	// names.
	Fields []*Field
	// Signed reports whether the fields were read out of a PGP cleartext signature.
	// Nothing here checks the signature; that it is present is what is reported.
	Signed bool
}

// Get returns the fields with a name, which is what a caller counts to find a field
// that appears more times than RFC 9116 allows.
func (parsed *Parsed) Get(name string) []*Field {
	if parsed == nil {
		return nil
	}

	var found []*Field

	for _, field := range parsed.Fields {
		if strings.EqualFold(field.Name, name) {
			found = append(found, field)
		}
	}

	return found
}

// Parse reads a security.txt.
//
// It fails only when the file is not a security.txt at all -- when a line is neither
// a field, a comment, nor empty. A file that parses may still be missing everything
// RFC 9116 requires of it, which is a judgement for the caller and not a syntax
// error.
func Parse(data []byte) (*Parsed, error) {
	body := data
	signed := false

	if cleartext, found := unwrapCleartextSignature(body); found {
		body = cleartext
		signed = true
	}

	// An empty file is a file with no fields, which the grammar says as well: body
	// is *line, and none of them is a line. It is not a syntax error, and reporting
	// it as one would hide what is actually wrong with it -- that it names no
	// contact and never expires.
	if len(bytes.TrimSpace(body)) == 0 {
		return &Parsed{SecurityTxt: &altshiftHttpTypes.SecurityTxt{}, Signed: signed}, nil
	}

	// The grammar ends every line with a newline, as section 2.2 requires. A file
	// whose last line lacks one is malformed by that rule and commonplace in fact,
	// and refusing to read it would serve nobody.
	if body[len(body)-1] != '\n' {
		body = append(append([]byte{}, body...), '\n')
	}

	paths, err := abnfUtils.GetParsedDataPaths(Grammar, body, "body")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError, data)
	}

	parsed := &Parsed{SecurityTxt: &altshiftHttpTypes.SecurityTxt{}, Signed: signed}

	for _, fieldPath := range abnfUtils.SearchPath(paths[0], []string{"field"}, 3, false) {
		namePath := abnfUtils.SearchPathSingleName(fieldPath, "field-name", 1, false)
		if namePath == nil {
			continue
		}

		name := string(abnfUtils.ExtractPathValue(body, namePath))

		var value string
		if valuePath := abnfUtils.SearchPathSingleName(fieldPath, "field-value", 1, false); valuePath != nil {
			value = strings.TrimSpace(string(abnfUtils.ExtractPathValue(body, valuePath)))
		}

		field := &Field{
			Name:      name,
			Value:     value,
			Line:      lineOf(body, namePath),
			Malformed: !valueIsWellFormed(name, value),
		}

		parsed.Fields = append(parsed.Fields, field)

		if !field.Malformed {
			assign(parsed.SecurityTxt, field)
		}
	}

	return parsed, nil
}

// valueIsWellFormed holds a field's value against the grammar its name requires. A
// name with no required grammar carries free text and cannot be malformed.
func valueIsWellFormed(name string, value string) bool {
	switch {
	case isUriField(name):
		return matches(value, "URI")
	case strings.EqualFold(name, FieldExpires):
		return matches(value, "date-time")
	case strings.EqualFold(name, FieldPreferredLanguages):
		return matches(value, "lang-values")
	default:
		return true
	}
}

// isUriField reports whether a field's value has to be a URI. Field names are
// case-insensitive, so the comparison is too.
func isUriField(name string) bool {
	for candidate := range uriFields {
		if strings.EqualFold(candidate, name) {
			return true
		}
	}

	return false
}

func matches(value string, rule string) bool {
	if value == "" {
		return false
	}

	paths, err := abnfUtils.GetParsedDataPaths(Grammar, []byte(value), rule)

	return err == nil && len(paths) != 0
}

// assign files a well-formed field under the meaning its name gives it.
func assign(securityTxt *altshiftHttpTypes.SecurityTxt, field *Field) {
	switch {
	case strings.EqualFold(field.Name, FieldContact):
		securityTxt.Contacts = append(securityTxt.Contacts, field.Value)
	case strings.EqualFold(field.Name, FieldExpires):
		// The first Expires wins. RFC 9116 forbids a second, and a caller that
		// wants to report one finds it in Fields.
		if securityTxt.Expires.IsZero() {
			if expires, err := parseTimestamp(field.Value); err == nil {
				securityTxt.Expires = expires
			}
		}
	case strings.EqualFold(field.Name, FieldEncryption):
		securityTxt.Encryption = append(securityTxt.Encryption, field.Value)
	case strings.EqualFold(field.Name, FieldAcknowledgments):
		securityTxt.Acknowledgments = append(securityTxt.Acknowledgments, field.Value)
	case strings.EqualFold(field.Name, FieldCanonical):
		securityTxt.Canonical = append(securityTxt.Canonical, field.Value)
	case strings.EqualFold(field.Name, FieldPolicy):
		securityTxt.Policy = append(securityTxt.Policy, field.Value)
	case strings.EqualFold(field.Name, FieldHiring):
		securityTxt.Hiring = append(securityTxt.Hiring, field.Value)
	case strings.EqualFold(field.Name, FieldCsaf):
		securityTxt.Csaf = append(securityTxt.Csaf, field.Value)
	case strings.EqualFold(field.Name, FieldPreferredLanguages):
		if len(securityTxt.PreferredLanguages) == 0 {
			for _, language := range strings.Split(field.Value, ",") {
				if trimmed := strings.TrimSpace(language); trimmed != "" {
					securityTxt.PreferredLanguages = append(securityTxt.PreferredLanguages, trimmed)
				}
			}
		}
	}
}

// parseTimestamp reads an Expires value.
//
// RFC 3339 writes the date-time separator and the zulu marker as ABNF quoted
// strings, which RFC 5234 makes case-insensitive, so "2030-04-01T00:00:00z" is
// conformant -- and RFC 9116's own example writes the marker in lowercase, which is
// errata 7264. Go's time.RFC3339 layout accepts only the uppercase forms, so a
// timestamp that is perfectly valid would otherwise be dropped on the floor. Of the
// first four large sites this was tried against, two wrote it in lowercase.
func parseTimestamp(value string) (time.Time, error) {
	normalized := value

	// The separator sits after the ten characters of the full-date.
	const separatorIndex = 10

	if len(normalized) > separatorIndex && normalized[separatorIndex] == 't' {
		normalized = normalized[:separatorIndex] + "T" + normalized[separatorIndex+1:]
	}

	if strings.HasSuffix(normalized, "z") {
		normalized = strings.TrimSuffix(normalized, "z") + "Z"
	}

	timestamp, err := time.Parse(time.RFC3339, normalized)
	if err != nil {
		return time.Time{}, fmt.Errorf("time parse: %w", err)
	}

	return timestamp, nil
}

// Cleartext signature framing, from RFC 4880 section 7.
var (
	cleartextHeader = []byte("-----BEGIN PGP SIGNED MESSAGE-----")
	signatureHeader = []byte("-----BEGIN PGP SIGNATURE-----")
)

// unwrapCleartextSignature takes the fields out of a signed file.
//
// RFC 9116 has a signed body and an unsigned one, and the signed one wraps the fields
// in an OpenPGP cleartext signature. The armor is stripped here rather than described
// in the grammar: nothing verifies the signature, so what the armor holds is of no
// interest beyond finding where the fields begin and end.
func unwrapCleartextSignature(data []byte) ([]byte, bool) {
	if !bytes.HasPrefix(bytes.TrimLeft(data, " \t\r\n"), cleartextHeader) {
		return nil, false
	}

	// The hash headers run to the first blank line, and the cleartext from there to
	// the signature.
	_, rest, found := bytes.Cut(data, cleartextHeader)
	if !found {
		return nil, false
	}

	blankLine := bytes.Index(rest, []byte("\n\n"))
	if carriageReturn := bytes.Index(rest, []byte("\r\n\r\n")); carriageReturn != -1 &&
		(blankLine == -1 || carriageReturn < blankLine) {
		blankLine = carriageReturn + 2
	}

	if blankLine == -1 {
		return nil, false
	}

	cleartext := rest[blankLine+2:]

	if end := bytes.Index(cleartext, signatureHeader); end != -1 {
		cleartext = cleartext[:end]
	}

	return undashEscape(cleartext), true
}

// undashEscape undoes the dash escaping a cleartext signature applies: a line
// beginning "- " had those two characters added to it, and they are not part of what
// was signed. RFC 4880 section 7.1.
func undashEscape(cleartext []byte) []byte {
	lines := bytes.Split(cleartext, []byte("\n"))

	for index, line := range lines {
		lines[index] = bytes.TrimPrefix(line, []byte("- "))
	}

	return bytes.Join(lines, []byte("\n"))
}

// lineOf is the one-based line a path starts on, so that a finding can say where.
func lineOf(data []byte, path *abnf.Path) int {
	if path == nil || path.Start > len(data) {
		return 0
	}

	return bytes.Count(data[:path.Start], []byte("\n")) + 1
}

func init() {
	var err error

	Grammar, err = abnf.ParseABNF(grammar)
	if err != nil {
		panic(fmt.Sprintf("abnf parse abnf (security.txt grammar): %v", err))
	}
}
