package webauthn

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"fmt"

	"github.com/altshiftab/utils_go/pkg/cose"
	motmedelCrypto "github.com/altshiftab/utils_go/pkg/crypto"
	motmedelEcdsa "github.com/altshiftab/utils_go/pkg/crypto/ecdsa"
	motmedelEddsa "github.com/altshiftab/utils_go/pkg/crypto/eddsa"
	motmedelCryptoInterfaces "github.com/altshiftab/utils_go/pkg/crypto/interfaces"
	motmedelRsa "github.com/altshiftab/utils_go/pkg/crypto/rsa"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/utils"
)

// NewVerifier returns a verifier for WebAuthn assertion signatures produced with the given COSE
// algorithm and credential public key. ECDSA signatures are expected in the ASN.1 DER encoding
// WebAuthn uses (not the raw encoding of COSE signatures).
func NewVerifier(
	algorithm cose.Algorithm,
	publicKey crypto.PublicKey,
) (motmedelCryptoInterfaces.Verifier, error) {
	if utils.IsNil(publicKey) {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("public key"))
	}

	algorithmName, ok := motmedelCrypto.CoseAlgNames[int(algorithm)]
	if !ok {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: unsupported cose algorithm %d", motmedelErrors.ErrValidationError, algorithm),
			algorithm,
		)
	}

	switch typedPublicKey := publicKey.(type) {
	case *ecdsa.PublicKey:
		method, err := motmedelEcdsa.FromPublicKey(typedPublicKey)
		if err != nil {
			return nil, motmedelErrors.New(fmt.Errorf("ecdsa from public key: %w", err), typedPublicKey)
		}

		// FromPublicKey infers the algorithm from the curve; it must agree with the requested
		// algorithm so a mismatching key cannot downgrade verification.
		if inferredName := method.GetName(); inferredName != algorithmName {
			return nil, motmedelErrors.NewWithTrace(
				fmt.Errorf(
					"%w: key algorithm %s does not match cose algorithm %s",
					motmedelErrors.ErrValidationError,
					inferredName,
					algorithmName,
				),
				inferredName,
				algorithmName,
			)
		}

		return &motmedelEcdsa.Asn1DerEncodedMethod{Method: *method}, nil
	case ed25519.PublicKey:
		if algorithmName != motmedelCrypto.AlgEdDsa {
			return nil, motmedelErrors.NewWithTrace(
				fmt.Errorf(
					"%w: ed25519 key does not match cose algorithm %s",
					motmedelErrors.ErrValidationError,
					algorithmName,
				),
				algorithmName,
			)
		}

		return &motmedelEddsa.Method{PublicKey: typedPublicKey}, nil
	case *rsa.PublicKey:
		method, err := motmedelRsa.New(algorithmName, nil, typedPublicKey)
		if err != nil {
			return nil, motmedelErrors.New(
				fmt.Errorf("rsa new: %w", err),
				algorithmName, typedPublicKey,
			)
		}

		return method, nil
	default:
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: unsupported public key type %T", motmedelErrors.ErrValidationError, publicKey),
		)
	}
}
