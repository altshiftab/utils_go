package iam_credentials_signer

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/iam_credentials"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

// Signer adapts an iam_credentials.Client to cloud_storage/types/signer.Signer.
// Use this when the runtime has no local private key (e.g. Cloud Run / metadata-server
// authentication). The runtime identity must hold roles/iam.serviceAccountTokenCreator
// on the target service account (which may be itself).
type Signer struct {
	client       *iam_credentials.Client
	email        string
	fetchOptions []fetch_config.Option
}

func New(client *iam_credentials.Client, email string, fetchOptions ...fetch_config.Option) (*Signer, error) {
	if client == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("iam credentials client"))
	}

	if email == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("email"))
	}

	return &Signer{client: client, email: email, fetchOptions: fetchOptions}, nil
}

func (s *Signer) Email() string {
	return s.email
}

func (s *Signer) Sign(ctx context.Context, payload []byte) ([]byte, error) {
	response, err := s.client.SignBlob(ctx, s.email, payload, s.fetchOptions...)
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("iam credentials sign blob: %w", err), s.email)
	}

	signature, err := base64.StdEncoding.DecodeString(response.SignedBlob)
	if err != nil {
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("base64 decode signed blob: %w", err))
	}

	return signature, nil
}
