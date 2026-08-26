// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package format

import (
	"fmt"
	"net/mail"
	"strings"

	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// emailFormat requires a valid email address.
func emailFormat(instance any, state *schema.ValidationState) error {
	s, ok := instance.(string)
	if !ok {
		return nil
	}
	if !isValidEmail(s, false) {
		return &schema.ValidationError{Message: fmt.Sprintf("%q is not a valid email address", s)}
	}
	return nil
}

// idnEmailFormat requires a valid internationalized email address.
func idnEmailFormat(instance any, state *schema.ValidationState) error {
	s, ok := instance.(string)
	if !ok {
		return nil
	}
	if !isValidEmail(s, true) {
		return &schema.ValidationError{Message: fmt.Sprintf("%q is not a valid extended email address", s)}
	}
	return nil
}

// isValidEmail reports whether s is a valid RFC5321 email address.
// If idn is true, this permits RFC6531 internationalized email addresses.
//
//nolint:dupword // RFC 5321 grammar quoted verbatim.
func isValidEmail(s string, idn bool) bool {
	// This is the syntax we are supposed to parse.
	// But in fact we don't bother, and just defer to
	// the net/mail package. That is more likely to implement
	// what the user expects anyhow.
	//
	// Mailbox          = Local-part "@" ( Domain / address-literal )
	// Local-part       = Dot-string / Quoted-string
	// Dot-string       = Atom *("."  Atom)
	// Atom             = 1*atext
	// Quoted-string    = DQUOTE *QcontentSMTP DQUOTE
	// QcontentSMTP     = qtextSMTP / quoted-pairSMTP
	// quoted-pairSMTP  = %d92 %d32-126
	//                  ; i.e., backslash followed by any ASCII
	//                  ; graphic (including itself) or SPace
	// qtextSMTP      = %d32-33 / %d35-91 / %d93-126
	//                  ; i.e., within a quoted string, any
	//                  ; ASCII graphic or space is permitted
	//                  ; without blackslash-quoting except
	//                  ; double-quote and the backslash itself.
	// Domain         = sub-domain *("." sub-domain)
	// sub-domain     = Let-dig [Ldh-str]
	// Let-dig        = ALPHA / DIGIT
	// Ldh-str        = *( ALPHA / DIGIT / "-" ) Let-dig
	//
	// address-literal  = "[" ( IPv4-address-literal / IPv6-address-literal / General-address-literal ) "]"
	// IPv4-address-literal  = Snum 3("."  Snum)
	// IPv6-address-literal  = "IPv6:" IPv6-addr
	//
	// General-address-literal  = Standardized-tag ":" 1*dcontent
	// Standardized-tag  = Ldh-str
	//                     ; Standardized-tag MUST be specified in a
	//                     ; Standards-Track RFC and registered with IANA
	// dcontent       = %d33-90 / ; Printable US-ASCII
	//                  %d94-126 ; excl. "[", "\", "]"
	//
	// Snum           = 1*3DIGIT
	//                  ; representing a decimal integer
	//                  ; value in the range 0 through 255
	// IPv6-addr      = IPv6-full / IPv6-comp / IPv6v4-full / IPv6v4-comp
	// IPv6-hex       = 1*4HEXDIG
	// IPv6-full      = IPv6-hex 7(":" IPv6-hex)
	// IPv6-comp      = [IPv6-hex *5(":" IPv6-hex)] "::"
	//                  [IPv6-hex *5(":" IPv6-hex)]
	//                  ; The "::" represents at least 2 16-bit groups of
	//                  ; zeros.  No more than 6 groups in addition to the
	//                  ; "::" may be present.
	// IPv6v4-full    = IPv6-hex 5(":" IPv6-hex) ":" IPv4-address-literal
	// IPv6v4-comp    = [IPv6-hex *3(":" IPv6-hex)] "::"
	//                  [IPv6-hex *3(":" IPv6-hex) ":"]
	//                  IPv4-address-literal
	//                  ; The "::" represents at least 2 16-bit groups of
	//                  ; zeros.  No more than 4 groups in addition to the
	//                  ; "::" and IPv4-address-literal may be present.

	// net/mail parses RFC 5321 address literals itself, including the
	// "[IPv6:literal]" form, and rejects an IPv6 literal without the tag.
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Name != "" {
		return false
	}

	// To make the testsuite happy we double-check that email
	// doesn't accept non-ASCII in the domain.
	// Use idn-email for that.
	if !idn {
		idx := strings.LastIndex(addr.Address, "@")
		if idx >= 0 {
			domain := addr.Address[idx+1:]
			if domain[0] != '[' {
				if !isNonIDNDomain(domain) {
					return false
				}
			}
		}
	}

	return true
}

// isNonIDNDomain reports whether s might be a non-internationalized
// domain name.
func isNonIDNDomain(s string) bool {
	for i := range len(s) {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '.':
		case c == '-':
		default:
			return false
		}
	}
	return true
}
