package dkim

import (
	_ "embed"
	"fmt"

	"github.com/altshiftab/utils_go/pkg/abnf"
)

//go:embed grammar.abnf
var grammar []byte

var DkimGrammar *abnf.Grammar

func init() {
	var err error
	DkimGrammar, err = abnf.ParseABNF(grammar)
	if err != nil {
		panic(fmt.Sprintf("goabnf parse abnf: %v", err))
	}
}
