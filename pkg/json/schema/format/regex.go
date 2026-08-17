// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package format

import (
	"fmt"
	"regexp/syntax"

	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// regexFormat requires a valid regex.
func regexFormat(instance any, state *schema.ValidationState) error {
	s, ok := instance.(string)
	if !ok {
		return nil
	}
	if _, err := syntax.Parse(s, syntax.Perl); err != nil {
		return &schema.ValidationError{Message: fmt.Sprintf("%q is not a valid regexp (note that only Go style regexps are supported)", s)}
	}
	return nil
}
