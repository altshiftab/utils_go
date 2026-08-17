// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package format

import (
	"fmt"

	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// uuidFormat requires a valid URI.
func uuidFormat(instance any, state *schema.ValidationState) error {
	s, ok := instance.(string)
	if !ok {
		return nil
	}

	orig := s
	bad := func() error {
		return &schema.ValidationError{Message: fmt.Sprintf("%q is not a valid UUID", orig)}
	}

	hexOctets := func(want int) bool {
		if len(s) < 2*want {
			return false
		}
		for i := range 2 * want {
			b := s[i]
			switch {
			case b >= '0' && b <= '9':
			case b >= 'A' && b <= 'F':
			case b >= 'a' && b <= 'f':
			default:
				return false
			}
		}
		s = s[2*want:]
		return true
	}

	dash := func() bool {
		if len(s) == 0 || s[0] != '-' {
			return false
		}
		s = s[1:]
		return true
	}

	if !hexOctets(4) {
		return bad()
	}
	if !dash() {
		return bad()
	}
	if !hexOctets(2) {
		return bad()
	}
	if !dash() {
		return bad()
	}
	if !hexOctets(2) {
		return bad()
	}
	if !dash() {
		return bad()
	}
	if !hexOctets(2) {
		return bad()
	}
	if !dash() {
		return bad()
	}
	if !hexOctets(6) {
		return bad()
	}
	if len(s) != 0 {
		return bad()
	}
	return nil
}
