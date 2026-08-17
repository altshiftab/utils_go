package abnf

import (
	"fmt"
	"strconv"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

// ruleNameOws is the rule the list expansion separates elements with.
// RFC 9110 Section 5.6.3 defines it as `OWS = *( SP / HTAB )`, and a grammar
// using the list operator has to define it, as it has to define every other
// rule it refers to.
const ruleNameOws = "OWS"

// evaluateListRepeat reads the `<n>#<m>element` list operator of RFC 9110
// Section 5.6.1 into a ListElement, which keeps the operator as written and
// carries the standard ABNF it stands for alongside it.
func evaluateListRepeat(input []byte, repeatPath *Path, hashIndex int, elementPath *Path) (*ListElement, error) {
	minimum, maximum, err := listRepeatCounts(input, repeatPath, hashIndex)
	if err != nil {
		return nil, fmt.Errorf("list repeat counts: %w", err)
	}

	element, err := evaluateElement(input, elementPath.Subpaths[0])
	if err != nil {
		return nil, fmt.Errorf("evaluate element: %w", err)
	}

	return &ListElement{
		Min:       minimum,
		Max:       maximum,
		Element:   element,
		Expansion: expandList(minimum, maximum, element),
	}, nil
}

// expandList returns the standard ABNF that RFC 7230 Section 7 gives for the
// list operator.
//
// RFC 7230 gives two expansions: one that senders have to satisfy, and one
// that recipients have to accept, which tolerates the empty list elements
// that senders must not generate but that recipients "MUST parse and
// ignore". A grammar is used here to read what someone else sent, so the
// recipient expansion is the one that applies:
//
//	#element  => [ ( "," / element ) *( OWS "," [ OWS element ] ) ]
//	1#element => *( "," OWS ) element *( OWS "," [ OWS element ] )
//
// RFC 7230 gives no recipient expansion for a bounded list, so a list with a
// count of its own takes the only expansion there is for it:
//
//	<n>#<m>element => element <n-1>*<m-1>( OWS "," OWS element )
func expandList(minimum int, maximum int, element Element) *Alternation {
	// `*( OWS "," [ OWS element ] )`, which carries the empty elements a
	// recipient has to tolerate.
	tolerant := rep(
		0,
		Inf,
		grp(
			cat(
				one(ref(ruleNameOws)),
				one(str(",")),
				one(opt(cat(one(ref(ruleNameOws)), one(element)))),
			),
		),
	)

	switch {
	case minimum == 0 && maximum == Inf:
		// `#element => [ ( "," / element ) *( OWS "," [ OWS element ] ) ]`
		return alt(
			cat(
				one(
					opt(
						cat(
							one(grp(cat(one(str(","))), cat(one(element)))),
							tolerant,
						),
					),
				),
			),
		)

	case minimum == 1 && maximum == Inf:
		// `1#element => *( "," OWS ) element *( OWS "," [ OWS element ] )`
		return alt(
			cat(
				rep(0, Inf, grp(cat(one(str(",")), one(ref(ruleNameOws))))),
				one(element),
				tolerant,
			),
		)
	}

	// `<n>#<m>element => element <n-1>*<m-1>( OWS "," OWS element )`
	separated := grp(
		cat(
			one(ref(ruleNameOws)),
			one(str(",")),
			one(ref(ruleNameOws)),
			one(element),
		),
	)

	remainingMaximum := maximum
	if maximum != Inf {
		remainingMaximum = max(maximum-1, 0)
	}

	if minimum == 0 {
		// A list that may hold nothing at all is the whole concatenation,
		// made optional.
		return alt(cat(one(opt(cat(one(element), rep(0, remainingMaximum, separated))))))
	}

	return alt(cat(one(element), rep(minimum-1, remainingMaximum, separated)))
}

// listRepeatCounts reads the counts on either side of the "#" of a list
// operator. An absent minimum is zero, and an absent maximum unbounded.
func listRepeatCounts(input []byte, repeatPath *Path, hashIndex int) (int, int, error) {
	minimum, maximum := 0, Inf

	if text := string(input[repeatPath.Start:hashIndex]); text != "" {
		count, err := strconv.Atoi(text)
		if err != nil {
			return 0, 0, motmedelErrors.NewWithTrace(
				fmt.Errorf("strconv atoi (list minimum): %w", err),
				text,
			)
		}
		minimum = count
	}

	if text := string(input[hashIndex+1 : repeatPath.End]); text != "" {
		count, err := strconv.Atoi(text)
		if err != nil {
			return 0, 0, motmedelErrors.NewWithTrace(
				fmt.Errorf("strconv atoi (list maximum): %w", err),
				text,
			)
		}
		maximum = count
	}

	return minimum, maximum, nil
}
