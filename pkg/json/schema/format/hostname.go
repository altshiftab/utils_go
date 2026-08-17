// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package format

import (
	"fmt"
	"net/netip"
	"strings"
	"unicode"

	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// hostnameFormat requires a valid hostname.
func hostnameFormat(instance any, state *schema.ValidationState) error {
	s, ok := instance.(string)
	if !ok {
		return nil
	}
	if !isValidHostname(s, false) {
		return &schema.ValidationError{Message: fmt.Sprintf("%q is not a valid hostname", s)}
	}
	return nil
}

// idnHostnameFormat requires a valid internationalized hostname.
func idnHostnameFormat(instance any, state *schema.ValidationState) error {
	s, ok := instance.(string)
	if !ok {
		return nil
	}
	if !isValidHostname(s, true) {
		return &schema.ValidationError{Message: fmt.Sprintf("%q is not a valid internationalized hostname", s)}
	}
	return nil
}

// acePrefix is the ASCII Compatible Encoding prefix (RFC 5890).
const acePrefix = "xn--"

// maxLabelLength and maxHostnameLength are the DNS length limits,
// in bytes of the ASCII form (RFC 1035).
const (
	maxLabelLength    = 63
	maxHostnameLength = 253
)

// isValidHostname reports whether this is a valid hostname.
// If idn is true, this permits internationalized hostnames.
func isValidHostname(s string, idn bool) bool {
	if _, err := netip.ParseAddr(s); err == nil {
		// Valid IP address.
		return true
	}

	// Underscores are not permitted by the testsuite.
	if strings.Contains(s, "_") {
		return false
	}

	if !idn {
		if !isASCIIString(s) {
			return false
		}
	} else {
		// Permit all stops (RFC3490 section 3.1).
		s = strings.ReplaceAll(s, "\u3002", ".")
		s = strings.ReplaceAll(s, "\uff0e", ".")
		s = strings.ReplaceAll(s, "\uff61", ".")
	}

	// An empty root label (trailing dot) is not permitted by the testsuite.
	if strings.HasSuffix(s, ".") {
		return false
	}

	var uLabels []string
	totalLength := 0
	for label := range strings.SplitSeq(s, ".") {
		uLabel, aceLength, ok := checkHostnameLabel(label)
		if !ok {
			return false
		}
		if totalLength > 0 {
			totalLength++ // the separating dot
		}
		totalLength += aceLength
		uLabels = append(uLabels, uLabel)
	}
	if totalLength > maxHostnameLength {
		return false
	}

	if idn && !isValidBidiDomain(uLabels) {
		return false
	}

	return true
}

// checkHostnameLabel checks a single hostname label, returning the
// Unicode form of the label and the length of its ASCII form.
// This replaces the checks previously delegated to an x/net idna
// registration profile: LDH characters, hyphen placement (RFC 5891
// section 4.2.3.1), Punycode validity and canonicity of A-labels,
// and the DNS label length limit.
func checkHostnameLabel(label string) (string, int, bool) {
	if label == "" {
		// An empty label: a leading dot or consecutive dots.
		return "", 0, false
	}

	if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
		return "", 0, false
	}

	uLabel := label
	aceLabel := label
	switch {
	case hasACEPrefix(label):
		// An A-label must be valid Punycode, must re-encode to
		// itself (RFC 5891 section 4.2.3.1), and must decode to a
		// non-ASCII U-label that satisfies the RFC 5892 rules.
		if !isLDHString(label) {
			return "", 0, false
		}
		decoded, ok := decodePunycode(label[len(acePrefix):])
		if !ok {
			return "", 0, false
		}
		reencoded, ok := encodePunycode(decoded)
		if !ok || !strings.EqualFold(reencoded, label[len(acePrefix):]) {
			return "", 0, false
		}
		if isASCIIString(decoded) || !isValidUnicodeLabel(decoded) {
			return "", 0, false
		}
		uLabel = decoded
	case isASCIIString(label):
		// Hyphens in the third and fourth position are reserved
		// for the ACE prefix (RFC 5891 section 4.2.3.1).
		if len(label) >= 4 && label[2] == '-' && label[3] == '-' {
			return "", 0, false
		}
		if !isLDHString(label) {
			return "", 0, false
		}
	default:
		// A U-label; its ASCII form determines the label length.
		if !isValidUnicodeLabel(label) {
			return "", 0, false
		}
		encoded, ok := encodePunycode(label)
		if !ok {
			return "", 0, false
		}
		aceLabel = acePrefix + encoded
	}

	if len(aceLabel) > maxLabelLength {
		return "", 0, false
	}
	return uLabel, len(aceLabel), true
}

// isLDHString reports whether s consists solely of ASCII letters,
// digits, and hyphens.
func isLDHString(s string) bool {
	for i := range len(s) {
		b := s[i]
		switch {
		case 'a' <= b && b <= 'z':
		case 'A' <= b && b <= 'Z':
		case '0' <= b && b <= '9':
		case b == '-':
		default:
			return false
		}
	}
	return true
}

// isASCIIString reports whether s consists solely of ASCII bytes.
func isASCIIString(s string) bool {
	for i := range len(s) {
		if s[i]&0x80 != 0 {
			return false
		}
	}
	return true
}

// hasACEPrefix reports whether the label starts with the ASCII
// Compatible Encoding prefix "xn--", case-insensitively.
func hasACEPrefix(label string) bool {
	return len(label) >= len(acePrefix) &&
		(label[0] == 'x' || label[0] == 'X') &&
		(label[1] == 'n' || label[1] == 'N') &&
		label[2] == '-' && label[3] == '-'
}

// isValidUnicodeLabel checks a Unicode (U-label) hostname label for
// the RFC 5891 and RFC 5892 rules.
func isValidUnicodeLabel(label string) bool {
	runes := []rune(label)
	if len(runes) == 0 {
		return false
	}

	// A label must not begin with a combining mark
	// (RFC 5891 section 4.2.3.2).
	if unicode.In(runes[0], unicode.M) {
		return false
	}

	// Hyphens in the third and fourth position are reserved for the
	// ACE prefix (RFC 5891 section 4.2.3.1).
	if len(runes) >= 4 && runes[2] == '-' && runes[3] == '-' {
		return false
	}

	// Arabic-Indic and Extended Arabic-Indic digits must not be
	// mixed (RFC 5892 appendixes A.8 and A.9).
	sawArabicIndicDigit := false
	sawExtendedArabicIndicDigit := false

	for i, c := range runes {
		switch c {
		case '\u0640', '\u07fa', '\u302e', '\u302f',
			'\u3031', '\u3032', '\u3033', '\u3034',
			'\u3035', '\u303b':
			// Disallowed rune (RFC 5892 section 2.6).
			return false

		case '\u00b7':
			// MIDDLE DOT must be surrounded by 'l'
			// (RFC 5892 appendix A.3).
			if i == 0 || runes[i-1] != 'l' || i+1 >= len(runes) || runes[i+1] != 'l' {
				return false
			}

		case '\u0375':
			// GREEK LOWER NUMERAL SIGN must be followed by Greek
			// (RFC 5892 appendix A.4).
			if i+1 >= len(runes) || !unicode.Is(unicode.Greek, runes[i+1]) {
				return false
			}

		case '\u05f3', '\u05f4':
			// HEBREW GERESH and GERSHAYIM must be preceded by Hebrew
			// (RFC 5892 appendixes A.5 and A.6).
			if i == 0 || !unicode.Is(unicode.Hebrew, runes[i-1]) {
				return false
			}

		case '\u30fb':
			// KATAKANA MIDDLE DOT requires Hiragana, Katakana, or Han
			// in the label (RFC 5892 appendix A.7).
			found := false
			for _, r := range runes {
				if unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Han, r) {
					found = true
					break
				}
			}
			if !found {
				return false
			}

		case '\u200d':
			// ZERO WIDTH JOINER must be preceded by a virama
			// (RFC 5892 appendix A.2).
			if i == 0 || !viramaRunes[runes[i-1]] {
				return false
			}

		case '\u200c':
			// ZERO WIDTH NON-JOINER must be preceded by a virama,
			// or must appear between joining letters
			// (RFC 5892 appendix A.1).
			if i == 0 {
				return false
			}
			if !viramaRunes[runes[i-1]] && !isJoiningContext(runes, i) {
				return false
			}

		case '-', '\u06fd', '\u06fe', '\u0f0b', '\u3007':
			// Permitted: hyphen and the PVALID exceptions with
			// non-letter general categories (RFC 5892 section 2.6).

		default:
			if '\u0660' <= c && c <= '\u0669' {
				sawArabicIndicDigit = true
			}
			if '\u06f0' <= c && c <= '\u06f9' {
				sawExtendedArabicIndicDigit = true
			}
			// Everything else must be a letter, mark, decimal digit,
			// or letterlike number to be PVALID (RFC 5892 section 2);
			// symbols and punctuation are disallowed.
			if !unicode.In(c, unicode.L, unicode.M, unicode.Nd, unicode.Nl) {
				return false
			}
		}
	}

	return !sawArabicIndicDigit || !sawExtendedArabicIndicDigit
}

// viramaRunes is the set of virama code points: combining marks with
// canonical combining class 9, which permit a following zero width
// joiner or non-joiner (RFC 5892 appendixes A.1 and A.2).
var viramaRunes = map[rune]bool{
	'\u094d': true, '\u09cd': true, '\u0a4d': true, '\u0acd': true,
	'\u0b4d': true, '\u0bcd': true, '\u0c4d': true, '\u0ccd': true,
	'\u0d3b': true, '\u0d3c': true, '\u0d4d': true, '\u0dca': true,
	'\u0e3a': true, '\u0eba': true, '\u0f84': true, '\u1039': true,
	'\u103a': true, '\u1714': true, '\u1715': true, '\u1734': true,
	'\u17d2': true, '\u1a60': true, '\u1b44': true, '\u1baa': true,
	'\u1bab': true, '\u1bf2': true, '\u1bf3': true, '\u2d7f': true,
	'\ua806': true, '\ua82c': true, '\ua8c4': true, '\ua953': true,
	'\ua9c0': true, '\uaaf6': true, '\uabed': true,
	'\U00010a3f': true, '\U00011046': true, '\U00011070': true,
	'\U0001107f': true, '\U000110b9': true, '\U00011133': true,
	'\U00011134': true, '\U000111c0': true, '\U00011235': true,
	'\U000112ea': true, '\U0001134d': true, '\U00011442': true,
	'\U000114c2': true, '\U000115bf': true, '\U0001163f': true,
	'\U000116b6': true, '\U0001172b': true, '\U00011839': true,
	'\U000119e0': true, '\U00011a34': true, '\U00011a47': true,
	'\U00011a99': true, '\U00011c3f': true, '\U00011d44': true,
	'\U00011d45': true, '\U00011d97': true,
}

// joiningScripts are the scripts whose letters join cursively, for
// the zero width non-joiner rule (RFC 5892 appendix A.1). The rule
// proper uses Unicode joining types; requiring the surrounding
// letters to be of a joining script is a close approximation.
var joiningScripts = []*unicode.RangeTable{
	unicode.Arabic,
	unicode.Syriac,
	unicode.Nko,
	unicode.Mandaic,
	unicode.Mongolian,
	unicode.Phags_Pa,
}

// isJoiningContext reports whether the zero width non-joiner at
// index i appears between letters of a joining script, skipping
// nonspacing marks.
func isJoiningContext(runes []rune, i int) bool {
	j := i - 1
	for j >= 0 && unicode.In(runes[j], unicode.Mn) {
		j--
	}
	if j < 0 || !unicode.In(runes[j], joiningScripts...) {
		return false
	}

	k := i + 1
	for k < len(runes) && unicode.In(runes[k], unicode.Mn) {
		k++
	}
	return k < len(runes) && unicode.In(runes[k], joiningScripts...)
}

// isArabicIndicDigit reports whether c is an Arabic-Indic digit,
// which has bidi class AN.
func isArabicIndicDigit(c rune) bool {
	return '\u0660' <= c && c <= '\u0669'
}

// isEuropeanDigit reports whether c has bidi class EN: the ASCII
// digits and the Extended Arabic-Indic digits.
func isEuropeanDigit(c rune) bool {
	return ('0' <= c && c <= '9') || ('\u06f0' <= c && c <= '\u06f9')
}

// isRTLRune reports whether c is a right-to-left character
// (bidi classes R and AL, approximated by script).
func isRTLRune(c rune) bool {
	if isArabicIndicDigit(c) || isEuropeanDigit(c) || unicode.In(c, unicode.M) {
		return false
	}
	return unicode.In(c,
		unicode.Hebrew, unicode.Arabic, unicode.Syriac, unicode.Thaana,
		unicode.Nko, unicode.Mandaic, unicode.Samaritan, unicode.Adlam)
}

// isValidBidiDomain checks the RFC 5893 bidi rule over the Unicode
// forms of a hostname's labels, approximating bidi classes by script.
// The rule only applies to bidi domain names: those with at least one
// right-to-left character or Arabic-Indic digit.
func isValidBidiDomain(uLabels []string) bool {
	bidi := false
	for _, label := range uLabels {
		for _, c := range label {
			if isRTLRune(c) || isArabicIndicDigit(c) {
				bidi = true
				break
			}
		}
		if bidi {
			break
		}
	}
	if !bidi {
		return true
	}

	for _, label := range uLabels {
		if !isValidBidiLabel(label) {
			return false
		}
	}
	return true
}

// isValidBidiLabel checks one label of a bidi domain name
// (RFC 5893 section 2).
func isValidBidiLabel(label string) bool {
	runes := []rune(label)
	if len(runes) == 0 {
		return false
	}

	// The first character determines the direction and must be a
	// letter (rule 1).
	rtl := isRTLRune(runes[0])
	if !rtl && !unicode.In(runes[0], unicode.L) {
		return false
	}

	sawEuropeanDigit := false
	sawArabicIndic := false
	for _, c := range runes {
		switch {
		case isArabicIndicDigit(c):
			sawArabicIndic = true
		case isEuropeanDigit(c):
			sawEuropeanDigit = true
		case !rtl && isRTLRune(c):
			// A left-to-right label must not contain
			// right-to-left characters (rule 5).
			return false
		}
	}
	// A right-to-left label must not mix the digit types (rule 4).
	if rtl && sawEuropeanDigit && sawArabicIndic {
		return false
	}

	// The last character, skipping combining marks, must match the
	// label direction (rules 3 and 6).
	j := len(runes) - 1
	for j >= 0 && unicode.In(runes[j], unicode.M) {
		j--
	}
	if j < 0 {
		return false
	}
	last := runes[j]
	if rtl {
		return isRTLRune(last) || isEuropeanDigit(last) || isArabicIndicDigit(last)
	}
	return unicode.In(last, unicode.L) || isEuropeanDigit(last)
}
