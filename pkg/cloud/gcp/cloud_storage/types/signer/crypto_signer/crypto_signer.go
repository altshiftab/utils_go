package crypto_signer

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

// Signer adapts an in-process crypto.Signer (e.g. an *rsa.PrivateKey parsed from a
// service-account JSON file) to cloud_storage/types/signer.Signer.
type Signer struct {
	signer crypto.Signer
	email  string
}

func New(signer crypto.Signer, email string) (*Signer, error) {
	if signer == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("signer"))
	}

	if email == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("email"))
	}

	return &Signer{signer: signer, email: email}, nil
}

func (s *Signer) Email() string {
	return s.email
}

func (s *Signer) Sign(_ context.Context, payload []byte) ([]byte, error) {
	digest := sha256.Sum256(payload)

	signature, err := s.signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("signer sign: %w", err))
	}

	return signature, nil
}
