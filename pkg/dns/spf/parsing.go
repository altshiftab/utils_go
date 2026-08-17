package spf

import (
	"errors"
	"fmt"
	"net"
	"strings"

	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	motmedelNet "github.com/altshiftab/utils_go/pkg/net"
)

var ErrUnexpectedMatchRule = errors.New("unexpected matching rule")

func extractLabelValues[T TermPtr](record *Record, passOnly bool, labels ...string) []string {
	var values []string

	for _, term := range record.Terms {
		if _, ok := term.(T); ok {
			switch typedTerm := term.(type) {
			case *Directive:
				for _, label := range labels {
					if passOnly && (typedTerm.Qualifier != "" && typedTerm.Qualifier != "+") {
						continue
					}

					if strings.ToLower(typedTerm.Mechanism.Label) == label {
						values = append(values, typedTerm.Mechanism.Value)
					}
				}
			case *Modifier:
				for _, label := range labels {
					if strings.ToLower(typedTerm.Label) == label {
						values = append(values, typedTerm.Value)
					}
				}
			}
		}
	}

	return values
}

func ExtractIncludeValues(record *Record) []string {
	if record == nil {
		return nil
	}

	return extractLabelValues[*Directive](record, false, "include")
}

func ExtractRedirectValues(record *Record) []string {
	if record == nil {
		return nil
	}

	return extractLabelValues[*Modifier](record, false, "redirect")
}

func ExtractNetworks(record *Record, passOnly bool) []*net.IPNet {
	if record == nil {
		return nil
	}

	var networks []*net.IPNet

	for _, networkString := range extractLabelValues[*Directive](record, passOnly, "ip4", "ip6") {
		if network, _ := motmedelNet.ParseAddressNet(networkString); network != nil {
			networks = append(networks, network)
		}
	}

	return networks
}

func ParseSpfRecord(data []byte) (*Record, error) {
	paths, err := abnfUtils.GetParsedDataPaths(SpfGrammar, data, "record")
	if err != nil {
		return nil, fmt.Errorf("get parsed data paths: %w", err)
	}
	if len(paths) == 0 {
		return nil, motmedelErrors.NewWithTrace(motmedelErrors.ErrSyntaxError)
	}

	var record Record
	record.Raw = string(data)

	var terms []any
	for i, termPath := range abnfUtils.SearchPath(paths[0], []string{"directive", "modifier"}, 2, false) {
		switch matchRule := termPath.MatchRule; matchRule {
		case "directive":
			directive := Directive{Index: i}

			qualifierPath := abnfUtils.SearchPathSingleName(termPath, "qualifier", 1, false)
			if qualifierPath != nil {
				directive.Qualifier = string(abnfUtils.ExtractPathValue(data, qualifierPath))
			}
			mechanismPath := abnfUtils.SearchPathSingleName(termPath, "mechanism", 1, false)
			if mechanismPath != nil {
				mechanismPair := strings.SplitN(string(abnfUtils.ExtractPathValue(data, mechanismPath)), ":", 2)
				directive.Mechanism = &Mechanism{Label: mechanismPair[0]} //nolint:nilaway // strings.SplitN never returns nil for a non-zero count.
				if len(mechanismPair) == 2 {
					directive.Mechanism.Value = mechanismPair[1]
				}
			}

			terms = append(terms, &directive)
		case "modifier":
			modifierPair := strings.SplitN(string(abnfUtils.ExtractPathValue(data, termPath)), "=", 2)
			// NOTE: According to the grammar there should always be two elements.
			terms = append(terms, &Modifier{Index: i, Label: modifierPair[0], Value: modifierPair[1]})
		default:
			return nil, motmedelErrors.NewWithTrace(
				fmt.Errorf("%w: %w: %s", motmedelErrors.ErrSemanticError, ErrUnexpectedMatchRule, matchRule),
			)
		}
	}

	record.Terms = terms

	return &record, nil
}
