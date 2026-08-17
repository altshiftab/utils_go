package minify

import (
	"slices"

	"github.com/altshiftab/utils_go/pkg/abnf"
)

// Transform identifies a simplification that rewrites the expressions of a
// grammar into shorter equivalent ones. Every transform preserves the
// matched language; none of them touches rule names, which are the
// interface of the grammar.
type Transform string

const (
	// TransformGroup drops the parentheses of groups that do not need
	// them: "a / (b / c)" becomes "a / b / c", and "*(pchar)" becomes
	// "*pchar".
	TransformGroup Transform = "group"
	// TransformNumVal expresses a num-val as a char-val where that is
	// shorter: "%x4D.6F.6E" becomes %s"Mon".
	TransformNumVal Transform = "num-val"
	// TransformLiteral joins adjacent literals that a single element can
	// express: "%x0D %x0A" becomes "%x0D.0A".
	TransformLiteral Transform = "literal"
)

// AllTransforms returns every transform, in the order Minify applies them.
func AllTransforms() []Transform {
	return []Transform{TransformGroup, TransformNumVal, TransformLiteral}
}

// RenderRule returns the minified definition of a single rule, ending with
// the CRLF that terminates it. Any transforms given are applied first, to a
// copy, so that the rule itself is left untouched. Passing a single
// transform shows what that transform alone would do.
func RenderRule(rule *abnf.Rule, transforms ...Transform) string {
	if rule == nil {
		return ""
	}

	if len(transforms) != 0 {
		rule = cloneRule(rule)
		simplifyToFixedPoint(func() { simplifyRule(rule, transforms) }, func() string { return writeRule(rule) })
	}

	return writeRule(rule)
}

// simplifyToFixedPoint repeats a simplification until the rendering it
// produces settles. Simplifying can expose further opportunities, as
// flattening a group brings its contents up into the enclosing expression.
func simplifyToFixedPoint(simplify func(), render func() string) {
	previous := ""
	for range maxSimplificationPasses {
		simplify()

		current := render()
		if current == previous {
			return
		}
		previous = current
	}
}

func cloneRule(rule *abnf.Rule) *abnf.Rule {
	return &abnf.Rule{Name: rule.Name, Alternation: cloneAlternation(rule.Alternation)}
}

func cloneAlternation(alternation *abnf.Alternation) *abnf.Alternation {
	if alternation == nil {
		return nil
	}

	concatenations := make([]*abnf.Concatenation, 0, len(alternation.Concatenations))
	for _, concatenation := range alternation.Concatenations {
		concatenations = append(concatenations, cloneConcatenation(concatenation))
	}

	return &abnf.Alternation{Concatenations: concatenations}
}

func cloneConcatenation(concatenation *abnf.Concatenation) *abnf.Concatenation {
	if concatenation == nil {
		return nil
	}

	repetitions := make([]*abnf.Repetition, 0, len(concatenation.Repetitions))
	for _, repetition := range concatenation.Repetitions {
		repetitions = append(repetitions, cloneRepetition(repetition))
	}

	return &abnf.Concatenation{Repetitions: repetitions}
}

func cloneRepetition(repetition *abnf.Repetition) *abnf.Repetition {
	if repetition == nil {
		return nil
	}

	return &abnf.Repetition{
		Min:     repetition.Min,
		Max:     repetition.Max,
		Element: cloneElement(repetition.Element),
	}
}

func cloneElement(element abnf.Element) abnf.Element {
	switch v := element.(type) {
	case *abnf.RulenameElement:
		return &abnf.RulenameElement{Name: v.Name}
	case *abnf.GroupElement:
		return &abnf.GroupElement{Alternation: cloneAlternation(v.Alternation)}
	case *abnf.OptionElement:
		return &abnf.OptionElement{Alternation: cloneAlternation(v.Alternation)}
	case *abnf.CharValElement:
		return &abnf.CharValElement{Sensitive: v.Sensitive, Value: v.Value}
	case *abnf.NumValElement:
		return &abnf.NumValElement{
			Base:   v.Base,
			Status: v.Status,
			Values: slices.Clone(v.Values),
		}
	case *abnf.ProseValElement:
		return &abnf.ProseValElement{Value: v.Value}
	}

	return element
}

// ExpandLists rewrites the "#" list operators of a grammar into the standard
// ABNF they stand for, in place. The operator belongs to the HTTP
// specifications rather than to RFC 5234, so a definition holding one is
// read only by a parser that knows it; expanding makes the definition
// portable, at the cost of the length the operator saves.
func ExpandLists(grammar *abnf.Grammar) {
	for _, rule := range grammar.Rules {
		expandAlternation(rule.Alternation)
	}
}

func expandAlternation(alternation *abnf.Alternation) {
	if alternation == nil {
		return
	}

	for _, concatenation := range alternation.Concatenations {
		if concatenation == nil {
			continue
		}

		repetitions := make([]*abnf.Repetition, 0, len(concatenation.Repetitions))
		for _, repetition := range concatenation.Repetitions {
			if repetition == nil {
				continue
			}

			listElement, ok := repetition.Element.(*abnf.ListElement)
			if !ok || !isSingle(repetition) {
				expandElement(repetition.Element)
				repetitions = append(repetitions, repetition)
				continue
			}

			// The expansion is a single concatenation, so its repetitions
			// stand where the operator stood.
			expandElement(listElement.Element)
			expansion := listElement.Expansion
			if expansion == nil || len(expansion.Concatenations) != 1 {
				repetitions = append(repetitions, repetition)
				continue
			}
			repetitions = append(repetitions, expansion.Concatenations[0].Repetitions...)
		}

		concatenation.Repetitions = repetitions
	}
}

func expandElement(element abnf.Element) {
	switch v := element.(type) {
	case *abnf.GroupElement:
		expandAlternation(v.Alternation)
	case *abnf.OptionElement:
		expandAlternation(v.Alternation)
	case *abnf.ListElement:
		expandElement(v.Element)
	}
}
