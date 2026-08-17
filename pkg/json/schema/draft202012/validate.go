// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package draft202012

import (
	"fmt"
	"net/url"
	"strings"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/json/schema/internal/validator"
	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// resolvedRefKeyword is a special Keyword used to record what a
// $ref keyword refers to in a schema.
var resolvedRefKeyword = schema.Keyword{
	Name:      "$$resolvedRef",
	ArgType:   schema.ArgTypeSchema,
	Validate:  validator.ValidateTrue,
	Generated: true,
}

// resolvedDynamicRefKeyword is a special Keyword used to record
// what a $dynamicRef refers to in a schema.
var resolvedDynamicRefKeyword = schema.Keyword{
	Name:      "$$resolvedDynamicRef",
	ArgType:   schema.ArgTypeSchema,
	Validate:  validator.ValidateTrue,
	Generated: true,
}

// detachedDynamicRefKeyword is a special Keyword used to record
// what a $dynamicRef refers to in a schema if we did not see
// any $dynamicAnchor while evaluating. We need this fallback for
// a reference to a subschema that skips over the base schema
// that records the dynamic anchor.
var detachedDynamicRefKeyword = schema.Keyword{
	Name:      "$$detachedDynamicRef",
	ArgType:   schema.ArgTypeSchema,
	Validate:  validator.ValidateTrue,
	Generated: true,
}

// recordDynamicAnchor is the type of the value stored with
// recordDynamicAnchorKeyword and clearDynamicAnchorKeyword.
type recordDynamicAnchor struct {
	anchor string
	schema *schema.Schema
}

// recordDynamicAnchorKeyword is a special Keyword that records a
// $dynamicAnchor. The string is the name of the $dynamicAnchor.
var recordDynamicAnchorKeyword = schema.Keyword{
	Name:      "$$recordDynamicAnchorKeyword",
	ArgType:   schema.ArgTypeString,
	Validate:  validator.ArgTypeAny(validateRecordDynamicAnchor),
	Generated: true,
}

// clearDynamicAnchorKeyword is a special Keyword that removes a
// $dynamicAnchor stored during validation.
var clearDynamicAnchorKeyword = schema.Keyword{
	Name:      "$$clearDynamicAnchorKeyword",
	ArgType:   schema.ArgTypeString,
	Validate:  validator.ArgTypeAny(validateClearDynamicAnchor),
	Generated: true,
}

// validateRef validates a $ref keyword.
func validateRef(arg schema.PartString, instance any, state *schema.ValidationState) error {
	for _, part := range state.Schema.Parts {
		if part.Keyword == &resolvedRefKeyword {
			return part.Value.(schema.PartSchema).S.ValidateInPlaceSchema(instance, state)
		}
	}
	// This should never happen.
	return fmt.Errorf(`%w: reference %q unresolved`, schema.ErrInvalidSchema, arg)
}

// validateDynamicRef validates a $dynamicRef keyword.
func validateDynamicRef(arg schema.PartString, instance any, state *schema.ValidationState) error {
	// See if this was resolved non-dynamically.
	var s *schema.Schema
	for _, part := range state.Schema.Parts {
		if part.Keyword == &resolvedDynamicRefKeyword {
			s = part.Value.(schema.PartSchema).S
			break
		}
	}

	if s == nil {
		// Resolve dynamically.
		var err error
		s, err = resolveDynamicRef(arg, state)
		if err != nil {
			return err
		}

		if s == nil {
			// Last try: a detached $dynamicAnchor.
			for _, part := range state.Schema.Parts {
				if part.Keyword == &detachedDynamicRefKeyword {
					s = part.Value.(schema.PartSchema).S
					break
				}
			}

			if s == nil {
				return fmt.Errorf("%w: dynamic reference %q unresolved", schema.ErrInvalidSchema, arg)
			}
		}
	}

	return s.ValidateInPlaceSchema(instance, state)
}

// validationData is data specific to the draft used for validation.
// We record the current dynamic anchors.
type validationData struct {
	// dynamicAnchors maps an anchor name to the record keyword value
	// that registered it. Tracking the registration site, rather than
	// just the anchor's schema, lets the matching clear keyword remove
	// exactly the registration it made: a nested registration of the
	// same resource is a no-op, and its clear must not remove the
	// outer registration.
	dynamicAnchors map[string]*recordDynamicAnchor
}

// validateRecordDynamicAnchor records a dynamic anchor during validation.
// This is added by the builder to every schema of a resource that
// defines a $dynamicAnchor, as entering any schema of a resource brings
// the resource's dynamic anchors into the dynamic scope.
// Dynamic anchors use a top-down scope: the first registration wins.
func validateRecordDynamicAnchor(arg schema.PartAny, instance any, state *schema.ValidationState) error {
	da := arg.V.(*recordDynamicAnchor)
	if *state.VersionData == nil {
		*state.VersionData = &validationData{
			dynamicAnchors: make(map[string]*recordDynamicAnchor),
		}
	}
	vd := (*state.VersionData).(*validationData)
	if _, ok := vd.dynamicAnchors[da.anchor]; ok {
		// We already have this dynamic anchor.
		return nil
	}
	vd.dynamicAnchors[da.anchor] = da
	return nil
}

// validateClearDynamicAnchor clears a dynamic anchor during validation.
// This is added by the builder at the end of every schema that has a
// matching record keyword. It removes the dynamic anchor added by
// validateRecordDynamicAnchor, so that the dynamic anchor is only
// visible while processing the schema that brought it into scope.
func validateClearDynamicAnchor(arg schema.PartAny, instance any, state *schema.ValidationState) error {
	da := arg.V.(*recordDynamicAnchor)
	vd := (*state.VersionData).(*validationData)
	if vd.dynamicAnchors[da.anchor] == da {
		delete(vd.dynamicAnchors, da.anchor)
	}
	return nil
}

// resolveDynamicRef dynamically resolves a $dynamicRef.
// This returns nil if the reference can't be resolved.
func resolveDynamicRef(arg schema.PartString, state *schema.ValidationState) (*schema.Schema, error) {
	if *state.VersionData == nil {
		return nil, nil
	}

	uri, err := url.Parse(string(arg))
	if err != nil {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf("failed to parse dynamic reference %q: %w", arg, err),
			string(arg),
		)
	}
	if uri.Fragment == "" || strings.HasPrefix(uri.Fragment, "/") {
		return nil, nil
	}

	vd := (*state.VersionData).(*validationData)
	da, ok := vd.dynamicAnchors[uri.Fragment]
	if !ok {
		return nil, nil
	}
	return da.schema, nil
}
