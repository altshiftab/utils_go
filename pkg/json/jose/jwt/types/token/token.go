package token

import (
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"maps"
	"strings"

	motmedelCryptoInterfaces "github.com/altshiftab/utils_go/pkg/crypto/interfaces"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/json/jose/jws/types/jws_object"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type Token struct {
	Header  map[string]any
	Payload map[string]any
}

func (t *Token) Encode(signer motmedelCryptoInterfaces.NamedSigner) (string, error) {
	if utils.IsNil(signer) {
		return "", motmedelErrors.NewWithTrace(nil_error.New("signer"))
	}

	payloadBytes, err := json.Marshal(t.Payload)
	if err != nil {
		return "", motmedelErrors.NewWithTrace(fmt.Errorf("json marshal (payload): %w", err), t.Payload)
	}

	var header map[string]any
	if tokenHeader := t.Header; tokenHeader != nil {
		header = maps.Clone(tokenHeader)
		if header == nil {
			return "", motmedelErrors.NewWithTrace(nil_error.NewWithInstance("map", "header clone"))
		}
	} else {
		header = make(map[string]any)
		header["typ"] = "JWT"
	}

	header["alg"] = signer.GetName()

	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", motmedelErrors.NewWithTrace(fmt.Errorf("json marshal (header): %w", err), header)
	}

	headerBase64 := base64.RawURLEncoding.EncodeToString(headerBytes)
	payloadBase64 := base64.RawURLEncoding.EncodeToString(payloadBytes)

	signatureInput := []byte(strings.Join([]string{headerBase64, payloadBase64}, "."))

	signature, err := signer.Sign(signatureInput)
	if err != nil {
		return "", motmedelErrors.NewWithTrace(fmt.Errorf("signer sign: %w", err), signatureInput)
	}

	return strings.Join(
		[]string{headerBase64, payloadBase64, base64.RawURLEncoding.EncodeToString(signature)},
		".",
	), nil
}

func NewFromJws(jws *jws_object.Object) (*Token, error) {
	if jws == nil {
		return nil, nil
	}

	var token Token

	if err := json.Unmarshal(jws.Header, &token.Header); err != nil {
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("json unmarshal (header): %w", err), jws.Header)
	}

	if err := json.Unmarshal(jws.Payload, &token.Payload); err != nil {
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("json unmarshal (payload): %w", err), jws.Payload)
	}

	return &token, nil
}

func New(tokenString string) (*Token, error) {
	rawToken, err := jws_object.New(tokenString)
	if err != nil {
		return nil, fmt.Errorf("raw token new: %w", err)
	}

	token, err := NewFromJws(rawToken)
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("from raw token: %w", err), rawToken)
	}

	return token, nil
}
