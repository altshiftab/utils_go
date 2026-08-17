// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package schemacache

import (
	"testing"

	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

func TestCache(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name  string
		cache interface {
			Load(schemaID, path string) *schema.Schema
			Store(schemaID, path string, s *schema.Schema) *schema.Schema
		}
	}{
		{name: "Cache", cache: &Cache{}},
		{name: "ConcurrentCache", cache: &ConcurrentCache{}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			cache := testCase.cache

			if got := cache.Load("draft", "path"); got != nil {
				t.Errorf("Load on empty cache = %v, want nil", got)
			}

			first := &schema.Schema{}
			if got := cache.Store("draft", "path", first); got != first {
				t.Errorf("Store returned %v, want the stored schema", got)
			}

			// Storing under the same key returns the first schema.
			second := &schema.Schema{}
			if got := cache.Store("draft", "path", second); got != first {
				t.Errorf("second Store returned %v, want the first schema", got)
			}

			if got := cache.Load("draft", "path"); got != first {
				t.Errorf("Load = %v, want the first schema", got)
			}

			// A different schema ID is a different key.
			if got := cache.Load("other-draft", "path"); got != nil {
				t.Errorf("Load with other schema ID = %v, want nil", got)
			}
		})
	}
}
