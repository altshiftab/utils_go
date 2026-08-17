package claim_strings

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type ClaimStrings []string

func (s *ClaimStrings) UnmarshalJSON(data []byte) error {
	var value any

	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}

	var aud []string

	switch v := value.(type) {
	case string:
		aud = append(aud, v)
	case []string:
		aud = v
	case []any:
		for _, vv := range v {
			vs, err := utils.Convert[string](vv)
			if err != nil {
				return altshiftErrors.NewWithTrace(fmt.Errorf("convert: %w", err), vv)
			}
			aud = append(aud, vs)
		}
	case nil:
		return nil
	default:
		return altshiftErrors.NewWithTrace(fmt.Errorf("%w: %T", altshiftErrors.ErrUnexpectedType, v), v)
	}

	*s = aud

	return nil
}

func (s ClaimStrings) MarshalJSON() ([]byte, error) {
	// By default a single-element value marshals as a one-element array. Pass
	// SingleAsString() to the marshal call to emit a single element as a bare
	// string instead (RFC 7519 permits either form for "aud", and UnmarshalJSON
	// accepts both).
	return json.Marshal([]string(s))
}

// SingleAsString returns a json/v2 marshal option that serializes any
// single-element ClaimStrings — at any nesting depth — as a bare JSON string
// instead of a one-element array (e.g. "aud":"x" rather than "aud":["x"]).
// Empty and multi-element values are unaffected. Pass it to a marshal call:
//
//	data, err := json.Marshal(claims, claim_strings.SingleAsString())
//
// Being a per-call option rather than a global, it is safe for concurrent use.
func SingleAsString() json.Options {
	return json.WithMarshalers(json.MarshalToFunc(marshalSingleClaimStringAsString))
}

// marshalSingleClaimStringAsString emits a single-element value as a bare string
// and returns SkipFunc for every other length so the default array marshaling
// runs. Functions supplied via WithMarshalers take precedence over the type's
// own MarshalJSON, so this applies wherever ClaimStrings appears in the value.
func marshalSingleClaimStringAsString(encoder *jsontext.Encoder, claimStrings ClaimStrings) error {
	if len(claimStrings) != 1 {
		return json.SkipFunc
	}
	return json.MarshalEncode(encoder, claimStrings[0])
}

func Convert(value any) (ClaimStrings, error) {
	var claimsString []string

	switch typedValue := value.(type) {
	case ClaimStrings:
		return typedValue, nil
	case string:
		claimsString = append(claimsString, typedValue)
	case []string:
		claimsString = typedValue
	case []any:
		for _, a := range typedValue {
			vs, err := utils.Convert[string](a)
			if err != nil {
				return nil, altshiftErrors.NewWithTrace(fmt.Errorf("convert: %w", err), a)
			}
			claimsString = append(claimsString, vs)
		}
	default:
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %T", altshiftErrors.ErrUnexpectedType, typedValue),
			typedValue,
		)
	}

	return claimsString, nil
}
