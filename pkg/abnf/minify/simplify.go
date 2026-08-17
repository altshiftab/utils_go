package minify

import (
	"slices"
	"strings"

	"github.com/altshiftab/utils_go/pkg/abnf"
)

// simplifyGrammar rewrites the expressions of the grammar into shorter
// equivalent ones, applying the given transforms. Rule names are left
// alone: they are the interface of the grammar, referred to by the code
// that parses with it.
func simplifyGrammar(grammar *abnf.Grammar, transforms []Transform) {
	for _, rule := range grammar.Rules {
		simplifyRule(rule, transforms)
	}
}

func simplifyRule(rule *abnf.Rule, transforms []Transform) {
	simplifyAlternation(rule.Alternation, transforms)
}

func simplifyAlternation(alternation *abnf.Alternation, transforms []Transform) {
	if alternation == nil {
		return
	}

	concatenations := make([]*abnf.Concatenation, 0, len(alternation.Concatenations))
	for _, concatenation := range alternation.Concatenations {
		simplifyConcatenation(concatenation, transforms)

		// An alternative that is nothing but a group contributes the
		// alternatives of that group directly: "a / (b / c)" is "a / b / c".
		if group := soleGroup(concatenation, transforms); group != nil {
			concatenations = append(concatenations, group.Alternation.Concatenations...)
			continue
		}

		concatenations = append(concatenations, concatenation)
	}

	alternation.Concatenations = concatenations
}

func simplifyConcatenation(concatenation *abnf.Concatenation, transforms []Transform) {
	if concatenation == nil {
		return
	}

	repetitions := make([]*abnf.Repetition, 0, len(concatenation.Repetitions))
	for _, repetition := range concatenation.Repetitions {
		simplifyRepetition(repetition, transforms)

		// An unrepeated group holding a single alternative contributes its
		// elements directly: "a (b c) d" is "a b c d".
		if isSingle(repetition) && enabled(transforms, TransformGroup) {
			if group, ok := repetition.Element.(*abnf.GroupElement); ok {
				if alternation := group.Alternation; alternation != nil && len(alternation.Concatenations) == 1 {
					repetitions = append(repetitions, alternation.Concatenations[0].Repetitions...)
					continue
				}
			}
		}

		repetitions = append(repetitions, repetition)
	}

	concatenation.Repetitions = mergeLiterals(repetitions, transforms)
}

func simplifyRepetition(repetition *abnf.Repetition, transforms []Transform) {
	if repetition == nil {
		return
	}

	switch element := repetition.Element.(type) {
	case *abnf.GroupElement:
		simplifyAlternation(element.Alternation, transforms)

		// A group around a single unrepeated element needs no parentheses,
		// even when repeated: "*(pchar)" is "*pchar".
		if enabled(transforms, TransformGroup) {
			if inner := soleElement(element.Alternation); inner != nil {
				repetition.Element = inner
			}
		}
	case *abnf.OptionElement:
		simplifyAlternation(element.Alternation, transforms)
	case *abnf.NumValElement:
		if enabled(transforms, TransformNumVal) {
			repetition.Element = simplifyNumVal(element)
		}
	}
}

// simplifyNumVal rewrites a num-val into a shorter equivalent element: a
// range that spans a single value into a series, and a series of printable
// US-ASCII characters into a char-val.
func simplifyNumVal(element *abnf.NumValElement) abnf.Element {
	if len(element.Values) == 0 {
		return element
	}

	runes := make([]rune, 0, len(element.Values))
	for _, value := range element.Values {
		r, err := abnf.ParseNumVal(value, element.Base)
		if err != nil {
			// A semantically validated grammar holds no invalid numerals,
			// but leave anything unexpected as it is.
			return element
		}
		runes = append(runes, r)
	}

	if element.Status == abnf.NumValStatusRange {
		if len(runes) == 2 && runes[0] == runes[1] {
			return &abnf.NumValElement{
				Base:   element.Base,
				Status: abnf.NumValStatusSeries,
				Values: []string{element.Values[0]},
			}
		}
		return element
	}

	for _, r := range runes {
		if !isCharValRune(r) {
			return element
		}
	}

	charVal := &abnf.CharValElement{
		// A char-val matches case-insensitively unless marked otherwise,
		// so only a value holding letters needs the case-sensitive form.
		Sensitive: strings.ContainsFunc(string(runes), isAsciiLetter),
		Value:     string(runes),
	}
	if len(charVal.String()) >= len(element.String()) {
		return element
	}

	return charVal
}

// mergeLiterals joins the adjacent unrepeated literal elements of a
// concatenation that a single element can express: "%x0D %x0A" is
// "%x0D.0A".
func mergeLiterals(repetitions []*abnf.Repetition, transforms []Transform) []*abnf.Repetition {
	if !enabled(transforms, TransformLiteral) {
		return repetitions
	}

	merged := make([]*abnf.Repetition, 0, len(repetitions))

	for _, repetition := range repetitions {
		if len(merged) != 0 && isSingle(repetition) {
			if previous := merged[len(merged)-1]; isSingle(previous) {
				if element := mergeElements(previous.Element, repetition.Element); element != nil {
					// Joining num-vals can produce a series that is better
					// expressed as a char-val.
					if numVal, ok := element.(*abnf.NumValElement); ok && enabled(transforms, TransformNumVal) {
						element = simplifyNumVal(numVal)
					}
					merged[len(merged)-1] = &abnf.Repetition{Min: 1, Max: 1, Element: element}
					continue
				}
			}
		}

		merged = append(merged, repetition)
	}

	return merged
}

// mergeElements returns the element that matches the concatenation of the
// two given elements, or nil if no single element does.
func mergeElements(left abnf.Element, right abnf.Element) abnf.Element {
	switch leftElement := left.(type) {
	case *abnf.CharValElement:
		rightElement, ok := right.(*abnf.CharValElement)
		if !ok {
			return nil
		}

		// Case sensitivity only distinguishes values that hold letters.
		leftCased := strings.ContainsFunc(leftElement.Value, isAsciiLetter)
		rightCased := strings.ContainsFunc(rightElement.Value, isAsciiLetter)
		if leftCased && rightCased && leftElement.Sensitive != rightElement.Sensitive {
			return nil
		}

		return &abnf.CharValElement{
			Sensitive: (leftCased && leftElement.Sensitive) || (rightCased && rightElement.Sensitive),
			Value:     leftElement.Value + rightElement.Value,
		}

	case *abnf.NumValElement:
		rightElement, ok := right.(*abnf.NumValElement)
		if !ok {
			return nil
		}
		if leftElement.Status != abnf.NumValStatusSeries || rightElement.Status != abnf.NumValStatusSeries {
			return nil
		}
		if !strings.EqualFold(leftElement.Base, rightElement.Base) {
			return nil
		}

		return &abnf.NumValElement{
			Base:   leftElement.Base,
			Status: abnf.NumValStatusSeries,
			Values: append(slices.Clone(leftElement.Values), rightElement.Values...),
		}
	}

	return nil
}

// soleGroup returns the group element of a concatenation that holds nothing
// but an unrepeated group, or nil.
func soleGroup(concatenation *abnf.Concatenation, transforms []Transform) *abnf.GroupElement {
	if !enabled(transforms, TransformGroup) {
		return nil
	}
	if concatenation == nil || len(concatenation.Repetitions) != 1 {
		return nil
	}

	repetition := concatenation.Repetitions[0]
	if repetition == nil || !isSingle(repetition) {
		return nil
	}

	group, _ := repetition.Element.(*abnf.GroupElement)
	if group == nil || group.Alternation == nil {
		return nil
	}

	return group
}

// soleElement returns the element of an alternation that holds nothing but a
// single unrepeated element, or nil.
func soleElement(alternation *abnf.Alternation) abnf.Element {
	if alternation == nil || len(alternation.Concatenations) != 1 {
		return nil
	}

	concatenation := alternation.Concatenations[0]
	if concatenation == nil || len(concatenation.Repetitions) != 1 {
		return nil
	}

	repetition := concatenation.Repetitions[0]
	if repetition == nil || !isSingle(repetition) {
		return nil
	}

	return repetition.Element
}

// enabled reports whether the transform is among those to apply.
func enabled(transforms []Transform, transform Transform) bool {
	return slices.Contains(transforms, transform)
}

// isSingle reports whether the repetition matches its element exactly once,
// which is the form that needs no repeat prefix.
func isSingle(repetition *abnf.Repetition) bool {
	return repetition != nil && repetition.Min == 1 && repetition.Max == 1
}

// isCharValRune reports whether a char-val can hold the rune, that is,
// whether it is one of the `%x20-21 / %x23-7E` of RFC 5234 Section 4.
func isCharValRune(r rune) bool {
	return (r >= 0x20 && r <= 0x21) || (r >= 0x23 && r <= 0x7E)
}

// isAsciiLetter reports whether the rune is one of the letters that ABNF
// matches case-insensitively.
func isAsciiLetter(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}
