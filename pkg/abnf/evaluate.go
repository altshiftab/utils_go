package abnf

import (
	"fmt"
	"strconv"
	"strings"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// evaluateGrammar evaluates a rulelist path, as produced by parsing an ABNF
// grammar definition with the ABNF grammar itself, into a Grammar.
func evaluateGrammar(input []byte, path *Path) (*Grammar, error) {
	// The rules are collected in definition order, and indexed by lowercase
	// name to resolve redefinitions and incremental alternatives.
	var rules []*Rule
	rulemap := map[string]*Rule{}

	listPath := path.Subpaths[0]
	subpath := listPath.Subpaths[0]
	for i := range listPath.Subpaths {
		// Only work on rules, skipping empty and comment lines.
		if subpath.MatchRule == ruleNameRule {
			rule, err := evaluateRule(input, subpath)
			if err != nil {
				return nil, fmt.Errorf("evaluate rule: %w", err)
			}
			lowerName := strings.ToLower(rule.Name)

			if coreRules[lowerName] != nil {
				return nil, &CoreRuleModificationError{Rulename: rule.Name}
			}

			// Determine the "defined-as" characters: a new rule ("=") or an
			// incremental alternative ("=/").
			definedAsPath := subpath.Subpaths[1]
			switch len(definedAsPath.Subpaths) {
			case 1:
				definedAsPath = definedAsPath.Subpaths[0]
			case 2:
				if definedAsPath.Subpaths[0].Subpaths[0].Subpaths == nil {
					definedAsPath = definedAsPath.Subpaths[0]
				} else {
					definedAsPath = definedAsPath.Subpaths[1]
				}
			default: // 3 subpaths
				definedAsPath = definedAsPath.Subpaths[1]
			}

			switch strings.TrimSpace(string(input[definedAsPath.Start:definedAsPath.End])) { //nolint:nilaway // False positive rooted in //go:embed byte slices treated as nilable; a nil grammar never reaches here.
			case "=":
				if rulemap[lowerName] != nil {
					return nil, &DuplicatedRuleError{Rulename: rule.Name}
				}
				rules = append(rules, rule)
				rulemap[lowerName] = rule
			case "=/":
				existingRule := rulemap[lowerName]
				if existingRule == nil {
					return nil, &RuleNotFoundError{Rulename: rule.Name}
				}
				existingRule.Alternation.Concatenations = append(
					existingRule.Alternation.Concatenations,
					rule.Alternation.Concatenations...,
				)
			}
		}

		if i+1 < len(listPath.Subpaths) {
			subpath = listPath.Subpaths[i+1].Subpaths[0]
		}
	}

	return &Grammar{Rules: rules, Rulemap: rulemap}, nil
}

func evaluateRule(input []byte, path *Path) (*Rule, error) {
	rulename := string(input[path.Subpaths[0].Start:path.Subpaths[0].End])

	// rule -> elements -> alternation
	alternation, err := evaluateAlternation(input, path.Subpaths[2].Subpaths[0])
	if err != nil {
		return nil, fmt.Errorf("evaluate alternation: %w", err)
	}

	return &Rule{Name: rulename, Alternation: alternation}, nil
}

// listItemPaths returns the item paths of an "item *(separators item)"
// style match: the first subpath, followed by each later hit of the named
// item rule.
func listItemPaths(path *Path, itemRuleName string) []*Path {
	itemPaths := []*Path{path.Subpaths[0]}

	// If nothing follows the first item, don't start the follow-up
	// extraction.
	if len(path.Subpaths) == 1 {
		return itemPaths
	}

	// Determine the first item hit index.
	subpaths := path.Subpaths[1].Subpaths
	i := 1
	for i < len(subpaths) && !strings.EqualFold(subpaths[i].MatchRule, itemRuleName) {
		i++
	}
	itemPaths = append(itemPaths, subpaths[i])

	// The following subpaths are hits too; the last subpath of each is
	// another item.
	for _, subpath := range subpaths[i+1:] {
		itemPaths = append(itemPaths, subpath.Subpaths[len(subpath.Subpaths)-1])
	}

	return itemPaths
}

func evaluateAlternation(input []byte, path *Path) (*Alternation, error) {
	var concatenations []*Concatenation
	for _, itemPath := range listItemPaths(path, ruleNameConcatenation) {
		concatenation, err := evaluateConcatenation(input, itemPath)
		if err != nil {
			return nil, fmt.Errorf("evaluate concatenation: %w", err)
		}
		concatenations = append(concatenations, concatenation)
	}
	return &Alternation{Concatenations: concatenations}, nil
}

func evaluateConcatenation(input []byte, path *Path) (*Concatenation, error) {
	var repetitions []*Repetition
	for _, itemPath := range listItemPaths(path, ruleNameRepetition) {
		repetition, err := evaluateRepetition(input, itemPath)
		if err != nil {
			return nil, fmt.Errorf("evaluate repetition: %w", err)
		}
		repetitions = append(repetitions, repetition)
	}
	return &Concatenation{Repetitions: repetitions}, nil
}

func evaluateRepetition(input []byte, path *Path) (*Repetition, error) {
	minCount, maxCount := 1, 1 // Default to exactly one.

	var elementPath *Path
	switch len(path.Subpaths) {
	case 1:
		elementPath = path.Subpaths[0]
	case 2:
		// -> option (hit) -> repeat -> hit
		repeatPath := path.Subpaths[0].Subpaths[0].Subpaths[0]
		elementPath = path.Subpaths[1]

		// A "#" marks the list operator, which is an element of its own
		// rather than a repeat.
		for i := repeatPath.Start; i < repeatPath.End; i++ {
			if input[i] != '#' {
				continue
			}

			listElement, err := evaluateListRepeat(input, repeatPath, i, elementPath)
			if err != nil {
				return nil, fmt.Errorf("evaluate list repeat: %w", err)
			}
			return one(listElement), nil
		}

		// Look for "*" to determine the repeat form.
		starIndex := -1
		for i := repeatPath.Start; i < repeatPath.End; i++ {
			if input[i] == '*' {
				starIndex = i
				break
			}
		}

		if starIndex == -1 {
			// No "*": an exact repetition count.
			countString := string(input[repeatPath.Start:repeatPath.End])
			count, err := strconv.Atoi(countString)
			if err != nil {
				return nil, motmedelErrors.NewWithTrace(
					fmt.Errorf("strconv atoi (repeat count): %w", err),
					countString,
				)
			}
			minCount, maxCount = count, count
		} else {
			minString := string(input[repeatPath.Start:starIndex])
			if minString == "" {
				minCount = 0
			} else {
				count, err := strconv.Atoi(minString)
				if err != nil {
					return nil, motmedelErrors.NewWithTrace(
						fmt.Errorf("strconv atoi (repeat minimum): %w", err),
						minString,
					)
				}
				minCount = count
			}

			maxString := string(input[starIndex+1 : repeatPath.End])
			if maxString == "" {
				maxCount = Inf
			} else {
				count, err := strconv.Atoi(maxString)
				if err != nil {
					return nil, motmedelErrors.NewWithTrace(
						fmt.Errorf("strconv atoi (repeat maximum): %w", err),
						maxString,
					)
				}
				maxCount = count
			}
		}
	default:
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf(
				"%w: unexpected number of repetition subpaths: %d",
				errUnexpectedPathStructure,
				len(path.Subpaths),
			),
		)
	}

	element, err := evaluateElement(input, elementPath.Subpaths[0])
	if err != nil {
		return nil, fmt.Errorf("evaluate element: %w", err)
	}

	return &Repetition{Min: minCount, Max: maxCount, Element: element}, nil
}

func evaluateElement(input []byte, path *Path) (Element, error) {
	for {
		switch path.MatchRule {
		case ruleNameRulename:
			return &RulenameElement{Name: string(input[path.Start:path.End])}, nil

		case ruleNameGroup, ruleNameOption:
			var alternationPath *Path
			for _, subpath := range path.Subpaths {
				if strings.EqualFold(subpath.MatchRule, ruleNameAlternation) {
					alternationPath = subpath
					break
				}
			}
			if alternationPath == nil {
				return nil, motmedelErrors.NewWithTrace(
					fmt.Errorf(
						"%w: no alternation subpath in %s path",
						errUnexpectedPathStructure,
						path.MatchRule,
					),
				)
			}

			alternation, err := evaluateAlternation(input, alternationPath)
			if err != nil {
				return nil, fmt.Errorf("evaluate alternation: %w", err)
			}
			if path.MatchRule == ruleNameOption {
				return &OptionElement{Alternation: alternation}, nil
			}
			return &GroupElement{Alternation: alternation}, nil

		case ruleNameCharVal:
			value := ""
			for _, subpath := range path.Subpaths[0].Subpaths {
				if strings.EqualFold(subpath.MatchRule, ruleNameQuotedString) {
					// Skip if empty char-val.
					if len(subpath.Subpaths) == 2 {
						continue
					}
					value = string(input[subpath.Subpaths[1].Start:subpath.Subpaths[1].End])
					break
				}
			}

			return &CharValElement{
				// Insensitive by default (cf. RFC 7405).
				Sensitive: strings.EqualFold(path.Subpaths[0].MatchRule, ruleNameCaseSensitiveString),
				Value:     value,
			}, nil

		case ruleNameNumVal:
			basePath := path.Subpaths[1].Subpaths[0]

			var base string
			switch basePath.MatchRule {
			case ruleNameBinVal:
				base = "b"
			case ruleNameDecVal:
				base = "d"
			case ruleNameHexVal:
				base = "x"
			}

			status := NumValStatusSeries
			values := []string{
				// The first hit is always at the same spot.
				string(input[basePath.Subpaths[1].Start:basePath.Subpaths[1].End]),
			}

			// Find whether it is a series or a range.
			if len(basePath.Subpaths) > 2 {
				hitPath := basePath.Subpaths[2].Subpaths[0]
				if input[hitPath.Subpaths[0].Start] == '-' {
					status = NumValStatusRange
				}

				// The second hit is always at the same spot.
				values = append(values, string(input[hitPath.Subpaths[1].Start:hitPath.Subpaths[1].End]))

				// Others follow in their own subpaths.
				for _, subpath := range hitPath.Subpaths[2:] {
					values = append(values, string(input[subpath.Subpaths[1].Start:subpath.Subpaths[1].End]))
				}
			}

			return &NumValElement{Base: base, Status: status, Values: values}, nil

		case ruleNameProseVal:
			return &ProseValElement{Value: string(input[path.Start+1 : path.End-1])}, nil
		}

		// Pass through structural paths with a single subpath.
		if len(path.Subpaths) != 1 {
			return nil, motmedelErrors.NewWithTrace(
				fmt.Errorf(
					"%w: unhandled path from %d to %d",
					errUnexpectedPathStructure,
					path.Start,
					path.End,
				),
				string(input[path.Start:path.End]),
			)
		}
		path = path.Subpaths[0]
	}
}
