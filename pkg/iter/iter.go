package iter

import (
	"iter"
	"reflect"
)

func SetDifference[T any, U comparable](set1 map[U]T, set2 map[U]T) map[U]T {
	result := make(map[U]T)

	for key := range set1 {
		if _, ok := set2[key]; !ok {
			var t T
			result[key] = t
		}
	}

	return result
}

func SetIntersection[T any, U comparable](set1 map[U]T, set2 map[U]T) map[U]T {
	var base map[U]T
	var other map[U]T

	if len(set1) < len(set2) {
		base = set1
		other = set2
	} else {
		base = set2
		other = set1
	}

	result := make(map[U]T)

	for key := range base {
		if _, ok := other[key]; ok {
			var t T
			result[key] = t
		}
	}

	return result
}

func Concat[V any](sequences ...iter.Seq[V]) iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, sequence := range sequences {
			for element := range sequence {
				if !yield(element) {
					return
				}
			}
		}
	}
}

func Concat2[K any, V any](sequences ...iter.Seq2[K, V]) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, sequence := range sequences {
			for kElement, vElement := range sequence {
				if !yield(kElement, vElement) {
					return
				}
			}
		}
	}
}

func Map[InputType any, OutputType any](inputSlice []InputType, f func(InputType) OutputType) []OutputType {
	outputSlice := make([]OutputType, len(inputSlice))
	for i, inputValue := range inputSlice {
		outputSlice[i] = f(inputValue)
	}
	return outputSlice
}

func Filter[T any](inputSlice []T, f func(T) bool) []T {
	var outputSlice []T
	for _, inputValue := range inputSlice {
		if f(inputValue) {
			outputSlice = append(outputSlice, inputValue)
		}
	}
	return outputSlice
}

func MapFilter[InputType any, OutputType any](inputSlice []InputType, f func(InputType) OutputType) []OutputType {
	var outputSlice []OutputType
	for _, inputValue := range inputSlice {
		if outputValue := f(inputValue); !reflect.ValueOf(outputValue).IsZero() {
			outputSlice = append(outputSlice, outputValue)
		}
	}
	return outputSlice
}

// Set returns the elements of the slices with duplicates removed, in the order
// they were first seen.
//
// The order is part of what it returns, not an accident of how it is
// implemented: collecting the keys of a map would hand back a different order on
// every run, and a caller that picks one element out of the result - or renders
// it - would then answer differently each time for the same input.
func Set[T comparable](elementSlices ...[]T) []T {
	if len(elementSlices) == 0 {
		return nil
	}

	setMap := make(map[T]struct{})
	var elements []T

	for _, elementSlice := range elementSlices {
		for _, element := range elementSlice {
			if _, ok := setMap[element]; ok {
				continue
			}
			setMap[element] = struct{}{}
			elements = append(elements, element)
		}
	}

	return elements
}
