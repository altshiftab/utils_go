package jws_object

import (
	"fmt"
	"strings"

	"github.com/altshiftab/utils_go/pkg/crypto/interfaces"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/json/jose/jws"
	"github.com/altshiftab/utils_go/pkg/utils"
)

// TODO: Not sure whether I should be exporting like this.

type Object struct {
	Header    []byte
	Payload   []byte
	Signature []byte
	Raw       string
}

func (o *Object) Verify(verifier interfaces.Verifier) error {
	if utils.IsNil(verifier) {
		return motmedelErrors.NewWithTrace(nil_error.New("verifier"))
	}

	rawSplit := strings.Split(o.Raw, ".")
	if len(rawSplit) != 3 {
		return motmedelErrors.NewWithTrace(motmedelErrors.ErrBadSplit, o.Raw)
	}

	header := rawSplit[0]
	payload := rawSplit[1]
	if err := jws.Verify(header, payload, o.Signature, verifier); err != nil {
		return motmedelErrors.New(fmt.Errorf("verifier verify: %w", err), header, payload, o.Signature)
	}

	return nil
}

func New(serialization string) (*Object, error) {
	header, payload, signature, err := jws.Parse(serialization)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	return &Object{Header: header, Payload: payload, Signature: signature, Raw: serialization}, nil
}
