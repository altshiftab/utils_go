package translate

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/go/code_generation"
)

var identRx = regexp.MustCompile(`[^0-9A-Za-z_]`)

func toIdent(s string) string {
	if s == "" {
		return "_"
	}
	s = identRx.ReplaceAllString(s, "_")
	// If the identifier starts with a digit, prefix it with `_`.
	if s[0] >= '0' && s[0] <= '9' {
		s = "_" + s
	}
	// Avoid Go keywords, minimally.
	switch s {
	case "const", "var", "type", "func", "package", "import", "map", "chan", "interface", "struct", "range", "go", "defer", "select", "case", "break", "continue", "default", "fallthrough", "if", "else", "switch", "for", "return":
		return s + "_"
	}
	return s
}

func isConst(v any) bool {
	switch v.(type) {
	case string, bool, float64:
		return true
	default:
		return false
	}
}

func Map(m map[string]any) (string, error) {
	if len(m) == 0 {
		return "", nil
	}

	type pair struct {
		key   string
		value any
	}
	var consts []pair
	var vars []pair

	for key, value := range m {
		if isConst(value) {
			consts = append(consts, pair{toIdent(key), value})
		} else {
			vars = append(vars, pair{toIdent(key), value})
		}
	}

	sort.Slice(consts, func(i, j int) bool { return consts[i].key < consts[j].key })
	sort.Slice(vars, func(i, j int) bool { return vars[i].key < vars[j].key })

	var out bytes.Buffer

	if len(consts) > 0 {
		out.WriteString("const (\n")
		for _, constPair := range consts {
			reflectValueOf := reflect.ValueOf(constPair.value)
			literal, _, err := code_generation.GenerateLiteral(
				reflectValueOf,
				nil,
			)
			if err != nil {
				return "", motmedelErrors.New(fmt.Errorf("generate literal: %w", err), reflectValueOf)
			}

			fmt.Fprintf(&out, "%s = %s\n", constPair.key, literal)
		}
		out.WriteString(")\n\n")
	}

	if len(vars) > 0 {
		out.WriteString("var (\n")
		for _, varPair := range vars {
			reflectValueOf := reflect.ValueOf(varPair.value)
			literal, _, err := code_generation.GenerateLiteral(
				reflectValueOf,
				nil,
			)
			if err != nil {
				return "", motmedelErrors.New(fmt.Errorf("generate literal: %w", err), reflectValueOf)
			}

			fmt.Fprintf(&out, "%s = %s\n", varPair.key, literal)
		}
		out.WriteString(")\n")
	}

	return strings.TrimSpace(out.String()), nil
}
