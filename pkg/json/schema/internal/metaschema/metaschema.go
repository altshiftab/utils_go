// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package metaschema

import (
	"embed"
	"fmt"
	"net/url"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/json/schema/internal/schemacache"
	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// metaCache is a cache of the meta-schemas.
// We use a single cache since they shouldn't change.
var metaCache schemacache.ConcurrentCache

// Load checks whether uri refers to a meta-schema in metaFS,
// and loads it if it does. If uri is not a meta-schema,
// this returns nil, nil. metaFS is for schemaID,
// and prefix is the filename prefix.
func Load(schemaID, prefix string, metaFS *embed.FS, uri *url.URL, ropts *schema.ResolveOpts) (*schema.Schema, error) {
	if uri.Scheme != "http" && uri.Scheme != "https" {
		return nil, nil
	}
	if uri.Host != "json-schema.org" {
		return nil, nil
	}
	path, ok := strings.CutPrefix(uri.Path, prefix)
	if !ok {
		return nil, nil
	}

	if s := metaCache.Load(schemaID, path); s != nil {
		return s, nil
	}

	data, err := metaFS.ReadFile("metaschema/" + path + ".json")
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("can't find meta-schema URI %q: %w", uri, err), path)
	}

	var s schema.Schema
	if err := s.UnmarshalJSON(data); err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("can't parse meta-schema URI %q: %w", uri, err), path)
	}

	r := metaCache.Store(schemaID, path, &s)
	return r, nil
}
