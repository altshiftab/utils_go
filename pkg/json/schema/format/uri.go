// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package format

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"

	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// uriOrIRI is an enum.
type uriOrIRI int

const (
	isURI uriOrIRI = iota + 1
	isIRI
)

// uriFormat requires a valid URI.
func uriFormat(instance any, state *schema.ValidationState) error {
	return uriIriFormat(instance, state, isURI)
}

// iriFormat requires a valid IRI.
func iriFormat(instance any, state *schema.ValidationState) error {
	return uriIriFormat(instance, state, isIRI)
}

// uriIriFormat checks for a URI or IRI.
func uriIriFormat(instance any, state *schema.ValidationState, ui uriOrIRI) error {
	s, ok := instance.(string)
	if !ok {
		return nil
	}
	uri, err := url.Parse(s)
	if err != nil {
		return &schema.ValidationError{Message: fmt.Sprintf("%q is not a valid URI: %v", s, err)}
	}
	if !uri.IsAbs() {
		return &schema.ValidationError{Message: fmt.Sprintf("%q is not an absolute URI", s)}
	}

	if !checkURI(uri, ui) {
		return &schema.ValidationError{Message: fmt.Sprintf("%q failed JSON schema checks", s)}
	}

	return nil
}

// uriReferenceFormat requires a valid URI, which may be a reference.
func uriReferenceFormat(instance any, state *schema.ValidationState) error {
	return uriIriReferenceFormat(instance, state, isURI)
}

// iriReferenceFormat requires a valid URI, which may be a reference.
func iriReferenceFormat(instance any, state *schema.ValidationState) error {
	return uriIriReferenceFormat(instance, state, isIRI)
}

// uriIriReferenceFormat checks for a URI or IRI, which may be a reference.
func uriIriReferenceFormat(instance any, state *schema.ValidationState, ui uriOrIRI) error {
	s, ok := instance.(string)
	if !ok {
		return nil
	}

	// This keeps the testsuite happy, and avoids parsing
	// what looks like an absolute URI as a relative one.
	if strings.HasPrefix(s, `\\`) {
		return &schema.ValidationError{Message: fmt.Sprintf(`%q starts with \\`, s)}
	}

	uri, err := url.Parse(s)
	if err != nil {
		return &schema.ValidationError{Message: fmt.Sprintf("%q is not a valid URI: %v", s, err)}
	}

	if !checkURI(uri, ui) {
		return &schema.ValidationError{Message: fmt.Sprintf("%q failed JSON schema checks", s)}
	}

	return nil
}

// checkURI reports whether the URI is valid for the JSON schema testsuite.
func checkURI(uri *url.URL, ui uriOrIRI) bool {
	// An IPv6 address should be in square brackets;
	// otherwise the colons can confuse the parse.
	if addr, err := netip.ParseAddr(uri.Host); err == nil && addr.Is6() {
		return false
	}

	// The testsuite does not want backslashes in fragments.
	if strings.Contains(uri.Fragment, `\`) {
		return false
	}

	// We apply further checks to URIs.
	if ui == isIRI {
		return true
	}

	// The testsuite expects various things to be rejected.
	for i := range uri.RawPath {
		c := uri.RawPath[i]
		if ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') {
			continue
		}
		switch c {
		case '-', '_', '.', '~', '@', '&', '=', '+', '$', '/', ';', ',', '(', ')', '#':
			continue
		default:
			return false
		}
	}

	return true
}
