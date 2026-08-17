// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package format

import (
	"fmt"
	"strings"

	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// jsonPointerFormat requires a valid JSON pointer.
func jsonPointerFormat(instance any, state *schema.ValidationState) error {
	s, ok := instance.(string)
	if !ok {
		return nil
	}

	if len(s) == 0 {
		return nil
	}
	if !strings.HasPrefix(s, "/") {
		return &schema.ValidationError{Message: fmt.Sprintf("%q is not a valid JSON pointer", s)}
	}
	if !checkJSONPointerEscapes(s) {
		return &schema.ValidationError{Message: fmt.Sprintf("%q has invalid escaping for a JSON pointer", s)}
	}
	return nil
}

// relativeJSONPointerFormat requires a valid relative JSON pointer.
func relativeJSONPointerFormat(instance any, state *schema.ValidationState) error {
	s, ok := instance.(string)
	if !ok {
		return nil
	}

	orig := s
	bad := func() error {
		return &schema.ValidationError{Message: fmt.Sprintf("%q is not a valid relative JSON pointer", orig)}
	}

	if len(s) == 0 {
		return bad()
	}
	if s[0] == '0' {
		s = s[1:]
	} else {
		if s[0] < '1' || s[0] > '9' {
			return bad()
		}
		s = s[1:]
		for len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
			s = s[1:]
		}
	}
	if len(s) == 0 || s == "#" {
		return nil
	}
	if s[0] != '/' {
		return bad()
	}

	if !checkJSONPointerEscapes(s) {
		return &schema.ValidationError{Message: fmt.Sprintf("%q has invalid escaping for a JSON pointer", s)}
	}
	return nil
}

// checkJSONPointerEscapes reports whether s has valid escapes.
func checkJSONPointerEscapes(s string) bool {
	for {
		_, after, ok := strings.Cut(s, "~")
		if !ok {
			break
		}
		if len(after) == 0 || (after[0] != '0' && after[0] != '1') {
			return false
		}
		s = after
	}
	return true
}
