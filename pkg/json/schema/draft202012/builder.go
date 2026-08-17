// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package draft202012

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/json/schema/builder"
	"github.com/altshiftab/utils_go/pkg/json/schema/internal/schemacache"
	"github.com/altshiftab/utils_go/pkg/json/schema/jsonpointer"
	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// Names of the core keywords handled during reference resolution.
const (
	idKeywordName            = "$id"
	anchorKeywordName        = "$anchor"
	dynamicAnchorKeywordName = "$dynamicAnchor"
	refKeywordName           = "$ref"
	dynamicRefKeywordName    = "$dynamicRef"
)

// Builder is a JSON schema builder.
// Builder provides a list of methods that may be used to add
// new elements to the schema.
// This should be used by programs that need to create a JSON schema
// from scratch, rather than unmarshaling it from a JSON representation
// or using [schemareflect.Reflect] to construct it from a Go type.
//
// Programs should use [NewBuilder] or [NewSubBuilder] to get a Builder.
type Builder struct {
	b *builder.Builder
}

// NewBuilder returns a [Builder] to use to build a JSON schema.
// Use this to build an entirely new schema.
func NewBuilder() *Builder {
	b := &Builder{builder.New(Vocabulary)}
	return b.AddString(&schema.SchemaKeyword, SchemaID)
}

// NewSubBuilder returns a [Builder] like [NewBuilder],
// but is for a schema that will be part of some larger schema.
func NewSubBuilder() *Builder {
	return &Builder{builder.New(Vocabulary)}
}

// Build returns a newly built schema.
func (b *Builder) Build() *schema.Schema {
	return b.b.Build()
}

// NewSubBuilder returns a new [Builder] with the same vocabulary.
// This is like the [NewSubBuilder] function in that it is for schemas
// that will be part of some larger schema.
func (b *Builder) NewSubBuilder() *Builder {
	return &Builder{builder.New(Vocabulary)}
}

// BoolSchema returns a newly built schema.
// If acceptAll is true the schema accepts all instance values,
// if false it accepts none.
// This is the JSON schema true and false values.
func (b *Builder) BoolSchema(acceptAll bool) *schema.Schema {
	b2 := b.NewSubBuilder()
	b2.b.AddBool(&schema.BoolKeyword, acceptAll)
	return b2.Build()
}

// AddSchemaParts adds a list of parts.
func (b *Builder) AddSchemaParts(parts []schema.Part) *Builder {
	b.b = b.b.AddSchemaParts(parts)
	return b
}

// Infer adds schema elements to b designed to validate JSON values
// that unmarshal into values of the given type.
// See [builder.Infer] for details.
func Infer[T any](b *Builder, opts *builder.InferOpts) (*Builder, error) {
	return builder.Infer[T](b, opts)
}

// InferType is like [Infer] buts takes a [reflect.Type] rather than
// a type argument.
func InferType(b *Builder, typ reflect.Type, opts *builder.InferOpts) (*Builder, error) {
	return builder.InferType(b, typ, opts)
}

// AddItemsSchema is for builder.Infer. Use the AddItems method instead.
func (b *Builder) AddItemsSchema(s *schema.Schema) *Builder {
	return b.AddItems(s)
}

// resolveState holds state during resolveSchema.
type resolveState struct {
	ropts   *schema.ResolveOpts
	root    *schema.Schema
	schemas map[*schema.Schema]schemaData
	uris    map[string]*schema.Schema
	anchors map[string]anchorData
	cache   schemacache.Cache
	// pendingAnchors maps a resource root to the dynamic anchors
	// found within that resource, pending installation of the
	// record and clear keywords.
	pendingAnchors map[*schema.Schema][]*recordDynamicAnchor
}

// schemaData is information we keep for some schemas.
type schemaData struct {
	uri *url.URL
}

// anchorData is information we keep for an anchor.
type anchorData struct {
	schema  *schema.Schema
	dynamic bool // true for $dynamicAnchor
}

// subInfo holds information we pass down to subschemas.
type subInfo struct {
	uri  *url.URL
	name []string
}

// Name returns the name of the current subschema.
func (si subInfo) Name() string {
	return "/" + strings.Join(si.name, "/")
}

// resolveSchema is the Vocabulary.Resolve field.
// It is called to resolve a schema decoded from JSON to
// handle $ref and friends.
func resolveSchema(schema *schema.Schema, ropts *schema.ResolveOpts) error {
	state := &resolveState{
		ropts: ropts,
		root:  schema,
	}
	var uri *url.URL
	if ropts != nil {
		uri = ropts.URI
	}
	return resolveRefSchema(uri, schema, state)
}

// resolveRefSchema resolves a schema that may have a known URI.
func resolveRefSchema(uri *url.URL, schema *schema.Schema, state *resolveState) error {
	subData := subInfo{
		uri: uri,
	}
	if err := resolveIDs(schema, schema, state, subData); err != nil {
		return err
	}
	installDynamicAnchors(state)
	return resolveRefs(schema, state, subData)
}

// installDynamicAnchors adds the record and clear keywords for the
// dynamic anchors collected by resolveIDs. Entering any schema of a
// resource brings the resource's dynamic anchors into the dynamic
// scope — a reference may enter a resource through a subschema,
// skipping the resource root — so the keywords are installed on every
// schema of the resource.
func installDynamicAnchors(state *resolveState) {
	for base, anchors := range state.pendingAnchors {
		// A resource defines each dynamic anchor at most once;
		// keep the first occurrence.
		seen := make(map[string]bool)
		kept := anchors[:0]
		for _, anchor := range anchors {
			if seen[anchor.anchor] {
				continue
			}
			seen[anchor.anchor] = true
			kept = append(kept, anchor)
		}
		installAnchorParts(base, kept, true)
	}
	state.pendingAnchors = nil
}

// installAnchorParts installs record and clear keyword pairs for
// anchors on s and its subschemas, stopping at nested resources.
// Each installation site gets its own record value, so that clearing
// is limited to the site that actually registered the anchor.
func installAnchorParts(s *schema.Schema, anchors []*recordDynamicAnchor, isResourceRoot bool) {
	if s == nil {
		return
	}
	if !isResourceRoot {
		if _, ok := s.LookupKeyword(idKeywordName); ok {
			// A nested resource; its own anchors are
			// installed separately.
			return
		}
	}

	for _, anchor := range anchors {
		val := &recordDynamicAnchor{
			anchor: anchor.anchor,
			schema: anchor.schema,
		}
		s.Parts = append([]schema.Part{
			{
				Keyword: &recordDynamicAnchorKeyword,
				Value:   schema.PartAny{V: val},
			},
		}, s.Parts...)
		s.Parts = append(s.Parts,
			schema.Part{
				Keyword: &clearDynamicAnchorKeyword,
				Value:   schema.PartAny{V: val},
			},
		)
	}

	for _, sub := range s.Children() {
		installAnchorParts(sub, anchors, false)
	}
}

// resolveIDs finds the IDs and anchors in a schema.
func resolveIDs(subSchema, base *schema.Schema, state *resolveState, subData subInfo) error {
	if subSchema == nil {
		return nil
	}

	var dynamicAnchor string
	for _, part := range subSchema.Parts {
		var err error
		switch part.Keyword.Name {
		case idKeywordName:
			subData, err = resolveID(subSchema, part.Value, state, subData)
			base = subSchema
		case anchorKeywordName:
			_, err = resolveAnchor(subSchema, false, part.Value, state, subData)
		case dynamicAnchorKeywordName:
			if dynamicAnchor != "" {
				return fmt.Errorf("%w: %s: more than one $dynamicAnchor", schema.ErrInvalidSchema, subData.Name())
			}
			dynamicAnchor, err = resolveAnchor(subSchema, true, part.Value, state, subData)
		case refKeywordName, dynamicRefKeywordName:
			// We need the URI when resolving references.
			if state.schemas == nil {
				state.schemas = make(map[*schema.Schema]schemaData)
			}
			state.schemas[subSchema] = schemaData{uri: subData.uri}
		}
		if err != nil {
			return err
		}
	}

	if dynamicAnchor != "" {
		// Record the dynamic anchor for its resource.
		// The record and clear keywords that implement the
		// dynamic scoping are installed by installDynamicAnchors
		// once all resources have been walked.
		if state.pendingAnchors == nil {
			state.pendingAnchors = make(map[*schema.Schema][]*recordDynamicAnchor)
		}
		state.pendingAnchors[base] = append(state.pendingAnchors[base], &recordDynamicAnchor{
			anchor: dynamicAnchor,
			schema: subSchema,
		})
	}

	for name, subsub := range subSchema.Children() {
		subsubData := subInfo{
			uri:  subData.uri,
			name: append(subData.name, name),
		}
		if err := resolveIDs(subsub, base, state, subsubData); err != nil {
			return err
		}
	}

	return nil
}

// resolveID handles the $id keyword when searching for anchors.
func resolveID(subSchema *schema.Schema, value schema.PartValue, state *resolveState, subData subInfo) (subInfo, error) {
	arg := value.(schema.PartString)
	uri, err := url.Parse(string(arg))
	if err != nil {
		return subInfo{}, altshiftErrors.NewWithTrace(
			fmt.Errorf(`%s: failed to parse "$id" %q: %w`, subData.Name(), arg, err),
			string(arg),
		)
	}
	if uri.Fragment != "" {
		return subInfo{}, fmt.Errorf(`%w: %s: "$id" %q contains non-empty fragment`, schema.ErrInvalidSchema, subData.Name(), arg)
	}
	var newURI *url.URL
	if uri.IsAbs() || subData.uri == nil {
		newURI = uri
	} else {
		newURI = subData.uri.ResolveReference(uri)
	}

	if state.uris == nil {
		state.uris = make(map[string]*schema.Schema)
	}
	state.uris[newURI.String()] = subSchema

	si := subInfo{
		uri:  newURI,
		name: subData.name,
	}
	return si, nil
}

// resolveAnchor handles the $anchor and $dynamicAnchor keywords
// when searching for anchors.
func resolveAnchor(subSchema *schema.Schema, dynamic bool, value schema.PartValue, state *resolveState, subData subInfo) (string, error) {
	anchor := string(value.(schema.PartString))
	if state.anchors == nil {
		state.anchors = make(map[string]anchorData)
	}

	var anchorURIBase url.URL
	if subData.uri != nil {
		anchorURIBase = *subData.uri
	}
	if anchorURIBase.Fragment != "" {
		panic("can't happen")
	}
	anchorURIBase.Fragment = anchor
	anchorURI := &anchorURIBase
	anchorStr := anchorURI.String()

	if _, ok := state.anchors[anchorStr]; ok {
		return "", fmt.Errorf("%w: %s: duplicate anchor %q", schema.ErrInvalidSchema, subData.Name(), anchorStr)
	}
	state.anchors[anchorStr] = anchorData{
		schema:  subSchema,
		dynamic: dynamic,
	}
	return anchor, nil
}

// resolveRefs resolves all $ref and $dynamicRef keywords in the schema.
func resolveRefs(subSchema *schema.Schema, state *resolveState, subData subInfo) error {
	if subSchema == nil {
		return nil
	}

	sawRef, sawDynamicRef := false, false
	for _, part := range subSchema.Parts {
		var err error
		switch part.Keyword.Name {
		case refKeywordName:
			if sawRef {
				return fmt.Errorf("%w: %s: more than one $ref", schema.ErrInvalidSchema, subData.Name())
			}
			sawRef = true
			err = resolveRef(subSchema, false, part.Value, state, subData)
		case dynamicRefKeywordName:
			if sawDynamicRef {
				return fmt.Errorf("%w: %s: more than one $dynamicRef", schema.ErrInvalidSchema, subData.Name())
			}
			sawDynamicRef = true
			err = resolveRef(subSchema, true, part.Value, state, subData)
		}
		if err != nil {
			return err
		}
	}

	for name, subsub := range subSchema.Children() {
		subsubData := subInfo{
			name: append(subData.name, name),
		}
		if err := resolveRefs(subsub, state, subsubData); err != nil {
			return err
		}
	}

	return nil
}

// resolveRef resolves a $ref or $dynamicRef in the schema.
// We record the resolved reference using a magic keyword.
func resolveRef(subSchema *schema.Schema, dynamic bool, value schema.PartValue, state *resolveState, subData subInfo) error {
	ref := string(value.(schema.PartString))
	refURI, err := url.Parse(ref)
	if err != nil {
		return altshiftErrors.NewWithTrace(
			fmt.Errorf("%s: failed to parse reference %q: %w", subData.Name(), ref, err),
			ref,
		)
	}

	sd, ok := state.schemas[subSchema]
	if !ok {
		// Should have been handled in resolveIDs.
		panic("resolveIDs did not resolve schema URI")
	}
	if sd.uri != nil {
		refURI = sd.uri.ResolveReference(refURI)
	}

	frag := refURI.Fragment

	// A $dynamicRef with a JSON pointer is not really dynamic.
	dynamicFrag := dynamic
	if dynamic && (frag == "" || strings.HasPrefix(frag, "/")) {
		dynamicFrag = false
	}

	addRef := func(refSchema *schema.Schema, detached bool) {
		resolvedKey := &resolvedRefKeyword
		if dynamic {
			resolvedKey = &resolvedDynamicRefKeyword
		}
		if detached {
			// This is a backup for a $dynamicRef to a
			// $dynamicAnchor, to be used if we skip over
			// the recordDynamicAnchor.
			resolvedKey = &detachedDynamicRefKeyword
		}

		subSchema.Parts = append(subSchema.Parts,
			schema.Part{
				Keyword: resolvedKey,
				Value:   schema.PartSchema{S: refSchema},
			},
		)
	}

	if ad, ok := state.anchors[refURI.String()]; ok {
		addRef(ad.schema, dynamicFrag && ad.dynamic)
		return nil
	}

	refSchema, err := resolveURI(refURI, state, subData)
	if err != nil {
		return err
	}

	// Loading and resolving the schema may have resolved
	// the reference. The schema was loaded without any fragment,
	// but refURI may include a fragment.
	if ad, ok := state.anchors[refURI.String()]; ok {
		addRef(ad.schema, dynamicFrag && ad.dynamic)
		return nil
	}

	// Otherwise, if there is a fragment, we expect it to be a
	// JSON pointer. A reference to an anchor should have been resolved by
	// looking in state.anchors.

	if frag != "" {
		if !strings.HasPrefix(frag, "/") {
			return fmt.Errorf("%w: %s: could not find fragment %q from URI %q", schema.ErrInvalidSchema, subData.Name(), frag, refURI)
		}

		if refSchema, err = jsonpointer.DerefSchema(SchemaID, refSchema, frag); err != nil {
			return fmt.Errorf("%w: %s: could not resolve JSON pointer %q from URI %q: %w", schema.ErrInvalidSchema, subData.Name(), frag, refURI, err)
		}
	}

	addRef(refSchema, false)
	return nil
}

// resolveURI returns the schema for a URI.
func resolveURI(refURI *url.URL, state *resolveState, subData subInfo) (*schema.Schema, error) {
	// The URI, ignoring the fragment, is either the empty string,
	// meaning the root, or a reference to some $id elsewhere in
	// the schema tree, or a URI to be loaded externally.

	noFragURIBase := *refURI
	noFragURIBase.Fragment = ""
	noFragURI := &noFragURIBase
	noFragStr := noFragURI.String()

	// An empty URI means the schema root.
	if noFragStr == "" {
		return state.root, nil
	}

	// Check for a reference to a known schema $id.
	refSchema, ok := state.uris[noFragStr]
	if ok {
		return refSchema, nil
	}

	// The URI refers to something elsewhere.
	if !noFragURI.IsAbs() {
		return nil, fmt.Errorf("%w: %s: could not resolve ref to %q", schema.ErrInvalidSchema, subData.Name(), noFragURI)
	}

	// Check for a reference to the metaschema.
	refSchema, err := checkMetaSchema(noFragURI, state.ropts)
	if err != nil {
		return nil, err
	}
	if refSchema != nil {
		return refSchema, nil
	}

	// We need to load the schema from a remote source.
	if state.ropts.Loader == nil {
		return nil, fmt.Errorf("%w: %s: remote loading of URI %q not permitted", schema.ErrInvalidSchema, subData.Name(), noFragURI)
	}

	// Check the cache.
	refSchema = state.cache.Load(SchemaID, noFragStr)
	if refSchema != nil {
		return refSchema, nil
	}

	// Load the schema remotely.
	refSchema, err = state.ropts.Loader(SchemaID, noFragURI)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%s: loading of URI %q failed: %w", subData.Name(), noFragURI, err),
			noFragStr,
		)
	}
	if refSchema == nil {
		return nil, fmt.Errorf("%w: %s: loading of URI %q returned no schema and no error", schema.ErrInvalidSchema, subData.Name(), noFragURI)
	}

	// Cache the schema. We must do before resolving the schema,
	// as resolving the schema may try to load it again.
	state.cache.Store(SchemaID, noFragStr, refSchema)

	// Resolve the schema in the current resolution state.
	if err := resolveRefSchema(noFragURI, refSchema, state); err != nil {
		return nil, fmt.Errorf("%w: %s: resolving schema at URI %q failed: %w", schema.ErrInvalidSchema, subData.Name(), noFragURI, err)
	}

	return refSchema, nil
}
