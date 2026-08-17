package webauthn

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"fmt"

	"github.com/altshiftab/utils_go/pkg/cose"
	altshiftCrypto "github.com/altshiftab/utils_go/pkg/crypto"
	altshiftEcdsa "github.com/altshiftab/utils_go/pkg/crypto/ecdsa"
	altshiftEddsa "github.com/altshiftab/utils_go/pkg/crypto/eddsa"
	altshiftCryptoInterfaces "github.com/altshiftab/utils_go/pkg/crypto/interfaces"
	altshiftRsa "github.com/altshiftab/utils_go/pkg/crypto/rsa"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/utils"
)

// NewVerifier returns a verifier for WebAuthn assertion signatures produced with the given COSE
// algorithm and credential public key. ECDSA signatures are expected in the ASN.1 DER encoding
// WebAuthn uses (not the raw encoding of COSE signatures).
func NewVerifier(
	algorithm cose.Algorithm,
	publicKey crypto.PublicKey,
) (altshiftCryptoInterfaces.Verifier, error) {
	if utils.IsNil(publicKey) {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("public key"))
	}

	algorithmName, ok := altshiftCrypto.CoseAlgNames[int(algorithm)]
	if !ok {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: unsupported cose algorithm %d", altshiftErrors.ErrValidationError, algorithm),
			algorithm,
		)
	}

	switch typedPublicKey := publicKey.(type) {
	case *ecdsa.PublicKey:
		method, err := altshiftEcdsa.FromPublicKey(typedPublicKey)
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("ecdsa from public key: %w", err), typedPublicKey)
		}

		// FromPublicKey infers the algorithm from the curve; it must agree with the requested
		// algorithm so a mismatching key cannot downgrade verification.
		if inferredName := method.GetName(); inferredName != algorithmName {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf(
					"%w: key algorithm %s does not match cose algorithm %s",
					altshiftErrors.ErrValidationError,
					inferredName,
					algorithmName,
				),
				inferredName,
				algorithmName,
			)
		}

		return &altshiftEcdsa.Asn1DerEncodedMethod{Method: *method}, nil
	case ed25519.PublicKey:
		if algorithmName != altshiftCrypto.AlgEdDsa {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf(
					"%w: ed25519 key does not match cose algorithm %s",
					altshiftErrors.ErrValidationError,
					algorithmName,
				),
				algorithmName,
			)
		}

		return &altshiftEddsa.Method{PublicKey: typedPublicKey}, nil
	case *rsa.PublicKey:
		method, err := altshiftRsa.New(algorithmName, nil, typedPublicKey)
		if err != nil {
			return nil, altshiftErrors.New(
				fmt.Errorf("rsa new: %w", err),
				algorithmName, typedPublicKey,
			)
		}

		return method, nil
	default:
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: unsupported public key type %T", altshiftErrors.ErrValidationError, publicKey),
		)
	}
}
