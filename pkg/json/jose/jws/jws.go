package jws

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	motmedelCryptoInterfaces "github.com/altshiftab/utils_go/pkg/crypto/interfaces"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/utils"
)

const (
	Delimiter = "."
)

// TODO: Implement tests for verification.

func Verify(header string, payload string, signature []byte, verifier motmedelCryptoInterfaces.Verifier) error {
	if utils.IsNil(verifier) {
		return motmedelErrors.NewWithTrace(nil_error.New("verifier"))
	}

	err := verifier.Verify([]byte(strings.Join([]string{header, payload}, ".")), signature)
	if err != nil {
		return fmt.Errorf("%w: verifier verify: %w", motmedelErrors.ErrVerificationError, err)
	}

	return nil
}

func VerifyCompactSerialization(serialization string, verifier motmedelCryptoInterfaces.Verifier) error {
	if utils.IsNil(verifier) {
		return motmedelErrors.NewWithTrace(nil_error.New("verifier"))
	}

	if serialization == "" {
		return motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: %w", motmedelErrors.ErrParseError, empty_error.New("serialization")),
		)
	}

	rawSplit := strings.Split(serialization, ".")
	if len(rawSplit) != 3 {
		return motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: %w", motmedelErrors.ErrParseError, motmedelErrors.ErrBadSplit),
		)
	}

	header := rawSplit[0]
	payload := rawSplit[1]

	signature, err := base64.RawURLEncoding.DecodeString(rawSplit[2])
	if err != nil {
		return motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: %w", motmedelErrors.ErrParseError, motmedelErrors.ErrBadSplit),
		)
	}

	if err := Verify(header, payload, signature, verifier); err != nil {
		return motmedelErrors.New(fmt.Errorf("verifier verify: %w", err), header, payload, signature)
	}

	return nil
}

func Split(serialization string) ([3]string, error) {
	var parts [3]string

	splitParts := strings.SplitN(serialization, Delimiter, 3)
	if len(splitParts) != 3 {
		return parts, motmedelErrors.NewWithTrace(motmedelErrors.ErrBadSplit)
	}

	parts[0] = splitParts[0]
	parts[1] = splitParts[1]
	parts[2] = splitParts[2]

	return parts, nil
}

func Parse(serialization string) ([]byte, []byte, []byte, error) {
	parts, err := Split(serialization)
	if err != nil {
		wrappedErr := fmt.Errorf("jws split: %w", err)
		if errors.Is(err, motmedelErrors.ErrBadSplit) {
			return nil, nil, nil, fmt.Errorf("%w: %w", motmedelErrors.ErrParseError, wrappedErr)
		}

		return nil, nil, nil, wrappedErr
	}

	var decodedParts [3][]byte

	for i := range parts {
		decodedParts[i], err = base64.RawURLEncoding.DecodeString(parts[i])
		if err != nil {
			var partName string
			switch i {
			case 0:
				partName = " (header part)"
			case 1:
				partName = " (payload part)"
			case 2:
				partName = " (signature part)"
			}
			return nil, nil, nil,
				motmedelErrors.NewWithTrace(
					fmt.Errorf(
						"%w: base64 raw url encoding decode string%s: %w",
						motmedelErrors.ErrParseError, partName, err,
					),
				)
		}
	}

	return decodedParts[0], decodedParts[1], decodedParts[2], nil
}
