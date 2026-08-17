package utils

import (
	"context"
	"errors"
	"slices"
	"testing"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

func TestConvert(t *testing.T) {
	t.Parallel()

	t.Run("int success", func(t *testing.T) {
		t.Parallel()

		got, err := Convert[int](5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 5 {
			t.Fatalf("expected 5, got %d", got)
		}
	})

	t.Run("string success", func(t *testing.T) {
		t.Parallel()

		got, err := Convert[string]("x")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "x" {
			t.Fatalf("expected x, got %q", got)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		t.Parallel()

		_, err := Convert[int]("x")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrConversionNotOk) {
			t.Fatalf("expected ErrConversionNotOk, got %v", err)
		}
	})

	t.Run("nil value", func(t *testing.T) {
		t.Parallel()

		_, err := Convert[int](nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrConversionNotOk) {
			t.Fatalf("expected ErrConversionNotOk, got %v", err)
		}
	})
}

func TestConvertSlice(t *testing.T) {
	t.Parallel()

	t.Run("typed slice passthrough", func(t *testing.T) {
		t.Parallel()

		got, err := ConvertSlice[int]([]int{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(got, []int{1, 2, 3}) {
			t.Fatalf("expected [1 2 3], got %v", got)
		}
	})

	t.Run("any slice converted", func(t *testing.T) {
		t.Parallel()

		got, err := ConvertSlice[int]([]any{1, 2, 3})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(got, []int{1, 2, 3}) {
			t.Fatalf("expected [1 2 3], got %v", got)
		}
	})

	t.Run("empty any slice", func(t *testing.T) {
		t.Parallel()

		got, err := ConvertSlice[int]([]any{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected empty slice, got %v", got)
		}
	})

	t.Run("any slice with wrong element", func(t *testing.T) {
		t.Parallel()

		_, err := ConvertSlice[int]([]any{1, "x"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrConversionNotOk) {
			t.Fatalf("expected ErrConversionNotOk, got %v", err)
		}
	})

	t.Run("wrong outer type", func(t *testing.T) {
		t.Parallel()

		_, err := ConvertSlice[int]("x")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrConversionNotOk) {
			t.Fatalf("expected ErrConversionNotOk, got %v", err)
		}
	})

	t.Run("nil value", func(t *testing.T) {
		t.Parallel()

		_, err := ConvertSlice[int](nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrConversionNotOk) {
			t.Fatalf("expected ErrConversionNotOk, got %v", err)
		}
	})
}

func TestConvertToNonZero(t *testing.T) {
	t.Parallel()

	t.Run("non-zero success", func(t *testing.T) {
		t.Parallel()

		got, err := ConvertToNonZero[int](5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 5 {
			t.Fatalf("expected 5, got %d", got)
		}
	})

	t.Run("zero value", func(t *testing.T) {
		t.Parallel()

		_, err := ConvertToNonZero[int](0)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrZeroValue) {
			t.Fatalf("expected ErrZeroValue, got %v", err)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		t.Parallel()

		_, err := ConvertToNonZero[int]("x")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrConversionNotOk) {
			t.Fatalf("expected ErrConversionNotOk, got %v", err)
		}
	})
}

func TestIsNil(t *testing.T) {
	t.Parallel()

	var nilPointer *int
	var nilMap map[string]int

	testCases := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "untyped nil", value: nil, want: true},
		{name: "typed nil pointer", value: nilPointer, want: true},
		{name: "non-nil pointer", value: new(int), want: false},
		{name: "int value", value: 5, want: false},
		{name: "empty string", value: "", want: false},
		{name: "nil map is not a pointer", value: nilMap, want: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := IsNil(testCase.value); got != testCase.want {
				t.Fatalf("expected %v, got %v", testCase.want, got)
			}
		})
	}
}

func TestMust_NilError(t *testing.T) {
	t.Parallel()

	// Must with a nil error is a no-op and must return without calling os.Exit.
	Must(nil, "should not exit")
}

// TestContextValueGetters covers both GetContextValue and GetNonZeroContextValue.
// Each case supplies its own getter closure so the differing generic type
// arguments (string vs int) can be exercised through a single table.
func TestContextValueGetters(t *testing.T) {
	t.Parallel()

	type ctxKey string
	key := ctxKey("k")

	withValue := func(v any) context.Context {
		return context.WithValue(context.Background(), key, v)
	}

	testCases := []struct {
		name    string
		get     func() (string, error)
		want    string
		wantErr error // nil => expect success
	}{
		{
			name: "GetContextValue success",
			get:  func() (string, error) { return GetContextValue[string](withValue("value"), key) },
			want: "value",
		},
		{
			name: "GetContextValue wrong type",
			get: func() (string, error) {
				_, err := GetContextValue[int](withValue("value"), key)
				return "", err
			},
			wantErr: altshiftErrors.ErrConversionNotOk,
		},
		{
			name:    "GetContextValue missing key",
			get:     func() (string, error) { return GetContextValue[string](context.Background(), key) },
			wantErr: altshiftErrors.ErrConversionNotOk,
		},
		{
			name: "GetNonZeroContextValue success",
			get:  func() (string, error) { return GetNonZeroContextValue[string](withValue("value"), key) },
			want: "value",
		},
		{
			name:    "GetNonZeroContextValue zero value",
			get:     func() (string, error) { return GetNonZeroContextValue[string](withValue(""), key) },
			wantErr: altshiftErrors.ErrZeroValue,
		},
		{
			name:    "GetNonZeroContextValue missing key fails conversion",
			get:     func() (string, error) { return GetNonZeroContextValue[string](context.Background(), key) },
			wantErr: altshiftErrors.ErrConversionNotOk,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := testCase.get()
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("expected %v, got %v", testCase.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != testCase.want {
				t.Fatalf("expected %q, got %q", testCase.want, got)
			}
		})
	}
}

func TestMapGet(t *testing.T) {
	t.Parallel()

	t.Run("present", func(t *testing.T) {
		t.Parallel()

		got, err := MapGet(map[string]int{"a": 1}, "a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 1 {
			t.Fatalf("expected 1, got %d", got)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		t.Parallel()

		_, err := MapGet(map[string]int{"a": 1}, "b")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrNotInMap) {
			t.Fatalf("expected ErrNotInMap, got %v", err)
		}
	})

	t.Run("nil map", func(t *testing.T) {
		t.Parallel()

		var m map[string]int
		_, err := MapGet(m, "a")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		ne, ok := errors.AsType[*nil_error.Error](err)
		if !ok {
			t.Fatalf("expected a nil error, got %v", err)
		}
		if ne.Field != "map" {
			t.Fatalf("expected field %q, got %q", "map", ne.Field)
		}
	})
}

func TestMapGetNonZero(t *testing.T) {
	t.Parallel()

	t.Run("present non-zero", func(t *testing.T) {
		t.Parallel()

		got, err := MapGetNonZero(map[string]int{"a": 1}, "a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 1 {
			t.Fatalf("expected 1, got %d", got)
		}
	})

	t.Run("present zero", func(t *testing.T) {
		t.Parallel()

		_, err := MapGetNonZero(map[string]int{"a": 0}, "a")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrMapZeroValue) {
			t.Fatalf("expected ErrMapZeroValue, got %v", err)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		t.Parallel()

		_, err := MapGetNonZero(map[string]int{"a": 1}, "b")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrNotInMap) {
			t.Fatalf("expected ErrNotInMap, got %v", err)
		}
	})

	t.Run("nil map", func(t *testing.T) {
		t.Parallel()

		var m map[string]int
		_, err := MapGetNonZero(m, "a")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if _, ok := errors.AsType[*nil_error.Error](err); !ok {
			t.Fatalf("expected a nil error, got %v", err)
		}
	})
}

func TestMapGetConvert(t *testing.T) {
	t.Parallel()

	t.Run("present convertible", func(t *testing.T) {
		t.Parallel()

		got, err := MapGetConvert[int](map[string]any{"a": 1}, "a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 1 {
			t.Fatalf("expected 1, got %d", got)
		}
	})

	t.Run("present wrong type", func(t *testing.T) {
		t.Parallel()

		_, err := MapGetConvert[int](map[string]any{"a": "x"}, "a")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrConversionNotOk) {
			t.Fatalf("expected ErrConversionNotOk, got %v", err)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		t.Parallel()

		_, err := MapGetConvert[int](map[string]any{"a": 1}, "b")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrNotInMap) {
			t.Fatalf("expected ErrNotInMap, got %v", err)
		}
	})
}

func TestMapGetConvertSlice(t *testing.T) {
	t.Parallel()

	t.Run("typed slice", func(t *testing.T) {
		t.Parallel()

		got, err := MapGetConvertSlice[string](map[string]any{"a": []string{"x", "y"}}, "a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(got, []string{"x", "y"}) {
			t.Fatalf("expected [x y], got %v", got)
		}
	})

	t.Run("any slice converted", func(t *testing.T) {
		t.Parallel()

		got, err := MapGetConvertSlice[string](map[string]any{"a": []any{"x", "y"}}, "a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !slices.Equal(got, []string{"x", "y"}) {
			t.Fatalf("expected [x y], got %v", got)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		t.Parallel()

		_, err := MapGetConvertSlice[string](map[string]any{"a": 5}, "a")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrConversionNotOk) {
			t.Fatalf("expected ErrConversionNotOk, got %v", err)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		t.Parallel()

		_, err := MapGetConvertSlice[string](map[string]any{"a": []string{"x"}}, "b")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrNotInMap) {
			t.Fatalf("expected ErrNotInMap, got %v", err)
		}
	})
}

func TestMapGetConvertNonZero(t *testing.T) {
	t.Parallel()

	t.Run("present non-zero", func(t *testing.T) {
		t.Parallel()

		got, err := MapGetConvertNonZero[int](map[string]any{"a": 3}, "a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 3 {
			t.Fatalf("expected 3, got %d", got)
		}
	})

	t.Run("present zero", func(t *testing.T) {
		t.Parallel()

		_, err := MapGetConvertNonZero[int](map[string]any{"a": 0}, "a")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrMapZeroValue) {
			t.Fatalf("expected ErrMapZeroValue, got %v", err)
		}
	})

	t.Run("present wrong type", func(t *testing.T) {
		t.Parallel()

		_, err := MapGetConvertNonZero[int](map[string]any{"a": "x"}, "a")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrConversionNotOk) {
			t.Fatalf("expected ErrConversionNotOk, got %v", err)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		t.Parallel()

		_, err := MapGetConvertNonZero[int](map[string]any{"a": 1}, "b")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, altshiftErrors.ErrNotInMap) {
			t.Fatalf("expected ErrNotInMap, got %v", err)
		}
	})
}

func TestAnyNonZero(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		vals []int
		want bool
	}{
		{name: "no values", vals: nil, want: false},
		{name: "all zero", vals: []int{0, 0, 0}, want: false},
		{name: "one non-zero", vals: []int{0, 0, 5}, want: true},
		{name: "all non-zero", vals: []int{1, 2, 3}, want: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := AnyNonZero(testCase.vals...); got != testCase.want {
				t.Fatalf("expected %v, got %v", testCase.want, got)
			}
		})
	}
}

func TestAnyNonZero_Strings(t *testing.T) {
	t.Parallel()

	if AnyNonZero("", "") {
		t.Fatal("expected false for all empty strings")
	}
	if !AnyNonZero("", "x") {
		t.Fatal("expected true when one string is non-empty")
	}
}
