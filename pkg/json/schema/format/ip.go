// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package format

import (
	"fmt"
	"net/netip"

	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// ipv4Format requires a valid IPv4 address.
func ipv4Format(instance any, state *schema.ValidationState) error {
	s, ok := instance.(string)
	if !ok {
		return nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil || !addr.Is4() {
		return &schema.ValidationError{Message: fmt.Sprintf("%q is not a valid IPv4 address", s)}
	}
	return nil
}

// ipv6Format requires a valid IPv6 address.
func ipv6Format(instance any, state *schema.ValidationState) error {
	s, ok := instance.(string)
	if !ok {
		return nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil || !addr.Is6() || addr.Zone() != "" {
		return &schema.ValidationError{Message: fmt.Sprintf("%q is not a valid IPv6 address", s)}
	}
	return nil
}
