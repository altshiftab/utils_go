package iter

import (
	"iter"
	"slices"
	"testing"
)

// seqFrom builds an iter.Seq that yields the given elements in order.
func seqFrom[V any](elements ...V) iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, element := range elements {
			if !yield(element) {
				return
			}
		}
	}
}

// seq2From builds an iter.Seq2 that yields the given key/value pairs in order.
func seq2From[K any, V any](keys []K, vals []V) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for i := range keys {
			if !yield(keys[i], vals[i]) {
				return
			}
		}
	}
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func TestSetDifference(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		set1 map[string]int
		set2 map[string]int
		want []string
	}{
		{
			name: "both empty",
			set1: map[string]int{},
			set2: map[string]int{},
			want: []string{},
		},
		{
			name: "nil inputs",
			set1: nil,
			set2: nil,
			want: []string{},
		},
		{
			name: "set2 empty returns all of set1",
			set1: map[string]int{"a": 1, "b": 2},
			set2: map[string]int{},
			want: []string{"a", "b"},
		},
		{
			name: "set1 empty returns nothing",
			set1: map[string]int{},
			set2: map[string]int{"a": 1},
			want: []string{},
		},
		{
			name: "disjoint returns all of set1",
			set1: map[string]int{"a": 1, "b": 2},
			set2: map[string]int{"c": 3, "d": 4},
			want: []string{"a", "b"},
		},
		{
			name: "overlapping removes shared keys",
			set1: map[string]int{"a": 1, "b": 2, "c": 3},
			set2: map[string]int{"b": 20, "c": 30, "e": 50},
			want: []string{"a"},
		},
		{
			name: "identical sets yield empty",
			set1: map[string]int{"a": 1, "b": 2},
			set2: map[string]int{"a": 1, "b": 2},
			want: []string{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := SetDifference(testCase.set1, testCase.set2)
			if gotKeys := sortedKeys(got); !slices.Equal(gotKeys, testCase.want) {
				t.Fatalf("expected keys %v, got %v", testCase.want, gotKeys)
			}
			// Result values must be the zero value of T.
			for key, value := range got {
				if value != 0 {
					t.Fatalf("expected zero value for key %q, got %d", key, value)
				}
			}
		})
	}
}

func TestSetIntersection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		set1 map[string]int
		set2 map[string]int
		want []string
	}{
		{
			name: "both empty",
			set1: map[string]int{},
			set2: map[string]int{},
			want: []string{},
		},
		{
			name: "one empty",
			set1: map[string]int{"a": 1},
			set2: map[string]int{},
			want: []string{},
		},
		{
			name: "disjoint",
			set1: map[string]int{"a": 1, "b": 2},
			set2: map[string]int{"c": 3},
			want: []string{},
		},
		{
			name: "overlapping keeps shared keys",
			set1: map[string]int{"a": 1, "b": 2, "c": 3},
			set2: map[string]int{"b": 20, "c": 30, "d": 40},
			want: []string{"b", "c"},
		},
		{
			name: "identical sets",
			set1: map[string]int{"a": 1, "b": 2},
			set2: map[string]int{"a": 1, "b": 2},
			want: []string{"a", "b"},
		},
		{
			name: "smaller base is second argument",
			set1: map[string]int{"a": 1, "b": 2, "c": 3},
			set2: map[string]int{"b": 20},
			want: []string{"b"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := SetIntersection(testCase.set1, testCase.set2)
			if gotKeys := sortedKeys(got); !slices.Equal(gotKeys, testCase.want) {
				t.Fatalf("expected keys %v, got %v", testCase.want, gotKeys)
			}
			for key, value := range got {
				if value != 0 {
					t.Fatalf("expected zero value for key %q, got %d", key, value)
				}
			}
		})
	}
}

func TestConcat(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		sequences []iter.Seq[int]
		want      []int
	}{
		{
			name:      "no sequences",
			sequences: nil,
			want:      nil,
		},
		{
			name:      "single sequence",
			sequences: []iter.Seq[int]{seqFrom(1, 2, 3)},
			want:      []int{1, 2, 3},
		},
		{
			name:      "multiple sequences",
			sequences: []iter.Seq[int]{seqFrom(1, 2), seqFrom(3), seqFrom(4, 5)},
			want:      []int{1, 2, 3, 4, 5},
		},
		{
			name:      "empty sequences interleaved",
			sequences: []iter.Seq[int]{seqFrom[int](), seqFrom(1), seqFrom[int](), seqFrom(2)},
			want:      []int{1, 2},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := slices.Collect(Concat(testCase.sequences...))
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("expected %v, got %v", testCase.want, got)
			}
		})
	}
}

func TestConcat_EarlyTermination(t *testing.T) {
	t.Parallel()

	// A yield returning false immediately must stop after the first element and
	// never iterate the remaining sequences.
	pulled := 0
	Concat(seqFrom(1, 2, 3), seqFrom(4, 5, 6))(func(int) bool {
		pulled++
		return false
	})
	if pulled != 1 {
		t.Fatalf("expected iteration to stop after 1 element, pulled %d", pulled)
	}

	// Stopping partway through the second sequence must include everything up to
	// and including the element at which yield returned false.
	var collected []int
	Concat(seqFrom(1, 2), seqFrom(3, 4))(func(v int) bool {
		collected = append(collected, v)
		return v != 3
	})
	if want := []int{1, 2, 3}; !slices.Equal(collected, want) {
		t.Fatalf("expected %v, got %v", want, collected)
	}
}

func TestConcat2(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		sequences []iter.Seq2[string, int]
		wantKeys  []string
		wantVals  []int
	}{
		{
			name:      "no sequences",
			sequences: nil,
			wantKeys:  nil,
			wantVals:  nil,
		},
		{
			name: "multiple sequences",
			sequences: []iter.Seq2[string, int]{
				seq2From([]string{"a", "b"}, []int{1, 2}),
				seq2From([]string{"c"}, []int{3}),
			},
			wantKeys: []string{"a", "b", "c"},
			wantVals: []int{1, 2, 3},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var gotKeys []string
			var gotVals []int
			for k, v := range Concat2(testCase.sequences...) {
				gotKeys = append(gotKeys, k)
				gotVals = append(gotVals, v)
			}
			if !slices.Equal(gotKeys, testCase.wantKeys) {
				t.Fatalf("expected keys %v, got %v", testCase.wantKeys, gotKeys)
			}
			if !slices.Equal(gotVals, testCase.wantVals) {
				t.Fatalf("expected vals %v, got %v", testCase.wantVals, gotVals)
			}
		})
	}
}

func TestConcat2_EarlyTermination(t *testing.T) {
	t.Parallel()

	pulled := 0
	Concat2(
		seq2From([]string{"a", "b"}, []int{1, 2}),
		seq2From([]string{"c"}, []int{3}),
	)(func(string, int) bool {
		pulled++
		return false
	})
	if pulled != 1 {
		t.Fatalf("expected iteration to stop after 1 pair, pulled %d", pulled)
	}
}

func TestMap(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input []int
		want  []string
	}{
		{name: "nil input", input: nil, want: []string{}},
		{name: "empty input", input: []int{}, want: []string{}},
		{name: "non-empty", input: []int{1, 2, 3}, want: []string{"1", "2", "3"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Map(testCase.input, func(v int) string {
				if v == 1 {
					return "1"
				}
				if v == 2 {
					return "2"
				}
				return "3"
			})
			// Map always allocates a slice of len(input); compare content.
			if len(got) != len(testCase.want) {
				t.Fatalf("expected len %d, got %d (%v)", len(testCase.want), len(got), got)
			}
			for i := range got {
				if got[i] != testCase.want[i] {
					t.Fatalf("index %d: expected %q, got %q", i, testCase.want[i], got[i])
				}
			}
		})
	}
}

func TestFilter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input []int
		want  []int
	}{
		{name: "nil input", input: nil, want: nil},
		{name: "empty input", input: []int{}, want: nil},
		{name: "keep none", input: []int{1, 3, 5}, want: nil},
		{name: "keep some", input: []int{1, 2, 3, 4}, want: []int{2, 4}},
		{name: "keep all", input: []int{2, 4, 6}, want: []int{2, 4, 6}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Filter(testCase.input, func(v int) bool { return v%2 == 0 })
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("expected %v, got %v", testCase.want, got)
			}
		})
	}
}

func TestMapFilter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input []int
		want  []int
	}{
		{name: "nil input", input: nil, want: nil},
		{name: "all mapped to zero are dropped", input: []int{1, 3, 5}, want: nil},
		{name: "zero results filtered out", input: []int{1, 2, 3, 4}, want: []int{2, 4}},
		{name: "no zero results", input: []int{2, 4}, want: []int{2, 4}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Map odd values to 0 (filtered), keep even values as-is.
			got := MapFilter(testCase.input, func(v int) int {
				if v%2 == 0 {
					return v
				}
				return 0
			})
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("expected %v, got %v", testCase.want, got)
			}
		})
	}
}

func TestMapFilter_StringZeroValue(t *testing.T) {
	t.Parallel()

	// Empty string is the zero value and must be dropped.
	got := MapFilter([]int{1, 2, 3}, func(v int) string {
		if v == 2 {
			return ""
		}
		return "kept"
	})
	if want := []string{"kept", "kept"}; !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestSet(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		elementSlices [][]int
		want          []int
	}{
		{name: "no slices returns nil", elementSlices: nil, want: nil},
		{name: "single empty slice", elementSlices: [][]int{{}}, want: nil},
		{name: "single slice deduped", elementSlices: [][]int{{1, 2, 2, 3, 3, 3}}, want: []int{1, 2, 3}},
		{name: "multiple slices merged and deduped", elementSlices: [][]int{{1, 2}, {2, 3}, {3, 4}}, want: []int{1, 2, 3, 4}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Set(testCase.elementSlices...)
			// Result order is nondeterministic (map iteration); compare as a sorted set.
			slices.Sort(got)
			if !slices.Equal(got, testCase.want) {
				t.Fatalf("expected %v, got %v", testCase.want, got)
			}
		})
	}
}

func TestSet_Strings(t *testing.T) {
	t.Parallel()

	got := Set([]string{"a", "b", "a"}, []string{"c", "b"})
	slices.Sort(got)
	if want := []string{"a", "b", "c"}; !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

// Set's order is part of its contract: a caller that picks one element out of
// the result, or renders it, must get the same answer for the same input every
// run. Collecting map keys would not.
func TestSetPreservesFirstSeenOrder(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input [][]string
		want  []string
	}{
		{
			name:  "no slices",
			input: nil,
			want:  nil,
		},
		{
			name:  "duplicates collapse to the first occurrence",
			input: [][]string{{"nginx", "NGINX", "Nginx", "NGINX"}},
			want:  []string{"nginx", "NGINX", "Nginx"},
		},
		{
			name:  "order is the order given, not sorted",
			input: [][]string{{"c", "a", "b"}},
			want:  []string{"c", "a", "b"},
		},
		{
			name:  "several slices concatenate before deduplicating",
			input: [][]string{{"a", "b"}, {"b", "c"}, {"a", "d"}},
			want:  []string{"a", "b", "c", "d"},
		},
		{
			name:  "an empty slice contributes nothing",
			input: [][]string{{}, {"a"}, {}},
			want:  []string{"a"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Set(testCase.input...)
			if len(got) != len(testCase.want) {
				t.Fatalf("Set() = %v, want %v", got, testCase.want)
			}
			for i := range got {
				if got[i] != testCase.want[i] {
					t.Fatalf("Set() = %v, want %v", got, testCase.want)
				}
			}
		})
	}
}

// Repeating the same call must give the same answer; the previous
// implementation did not, which made anything built on it non-reproducible.
func TestSetIsDeterministic(t *testing.T) {
	t.Parallel()

	input := []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta"}

	first := Set(input)
	if len(first) != len(input) {
		t.Fatalf("Set() = %v, want %d elements", first, len(input))
	}

	for range 200 {
		again := Set(input)
		if len(again) != len(first) {
			t.Fatalf("Set() returned %d elements, then %d", len(first), len(again))
		}

		for i := range first {
			if again[i] != first[i] {
				t.Fatalf("Set() varied between calls: %v then %v", first, again)
			}
		}
	}
}
