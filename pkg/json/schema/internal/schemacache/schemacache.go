// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package schemacache is a simple in-process cache for schemas
// that have been parsed.
package schemacache

import (
	"sync"

	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// Cache is a cache that holds schemas.
type Cache struct {
	m map[cacheKey]*schema.Schema
}

// cacheKey is the key type of cachedSchemas.
// We need to track both the schema draft and the path,
// as it is possible, at least in the testsuite,
// for the same path to be used by different schema drafts.
type cacheKey struct {
	schemaID string
	path     string
}

// Load checks the cache for a schema.
// It returns nil if the path is not cached.
func (c *Cache) Load(schemaID, path string) *schema.Schema {
	return c.m[cacheKey{schemaID, path}]
}

// Store stores a schema in the cache.
// It returns the schema to use, which may differ
// if it has already been cached.
func (c *Cache) Store(schemaID, path string, s *schema.Schema) *schema.Schema {
	key := cacheKey{schemaID, path}
	if sc := c.m[key]; sc != nil {
		return sc
	}

	if c.m == nil {
		c.m = make(map[cacheKey]*schema.Schema)
	}

	c.m[key] = s
	return s
}

// ConcurrentCache is a cache that permits concurrent access.
type ConcurrentCache struct {
	cache Cache
	mu    sync.Mutex
}

// Load checks the cache for a schema.
// It returns nil if the path is not cached.
func (cc *ConcurrentCache) Load(schemaID, path string) *schema.Schema {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.cache.Load(schemaID, path)
}

// Store stores a schema in the cache.
// It returns the schema to use, which may differ
// if some other goroutine already cached it.
func (cc *ConcurrentCache) Store(schemaID, path string, s *schema.Schema) *schema.Schema {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.cache.Store(schemaID, path, s)
}
