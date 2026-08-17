package service_account_token_source

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json/v2"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/credentials_file"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/token_response"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	"github.com/altshiftab/utils_go/pkg/http/utils"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token"
)

func parsePrivateKey(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.New("pem decode: no PEM block found"))
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("x509 parse pkcs1 private key: %w", err))
		}
		return rsaKey, nil
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("x509 parse pkcs8 private key: %w", err))
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %T", altshiftErrors.ErrUnexpectedType, key))
		}
		return rsaKey, nil
	default:
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: unsupported PEM block type: %s", altshiftErrors.ErrUnexpectedType, block.Type))
	}
}

type TokenSource struct {
	ctx          context.Context //nolint:containedctx // The TokenSource interface takes no context; the construction context is deliberately captured (same pattern as x/oauth2).
	clientEmail  string
	privateKeyID string
	privateKey   *rsa.PrivateKey
	tokenURI     string
	scopes       []string
	subject      string
	options      []fetch_config.Option

	credentialsFile *credentials_file.File
}

func (s *TokenSource) Token() (*token.Token, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	// TODO: Use JWT library?

	now := time.Now()

	headerJSON, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
		"kid": s.privateKeyID,
	})
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json marshal (jwt header): %w", err))
	}

	claims := map[string]any{
		"iss":   s.clientEmail,
		"scope": strings.Join(s.scopes, " "),
		"aud":   s.tokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	// For Google Workspace domain-wide delegation, impersonate subject via the
	// "sub" claim. Omitted when empty (the service account acts as itself).
	if s.subject != "" {
		claims["sub"] = s.subject
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json marshal (jwt claims): %w", err))
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) +
		"." +
		base64.RawURLEncoding.EncodeToString(claimsJSON)

	h := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.privateKey, crypto.SHA256, h[:])
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("rsa sign pkcs1v15: %w", err))
	}

	assertion := signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)

	v := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}

	options := append(
		[]fetch_config.Option{
			fetch_config.WithMethod(http.MethodPost),
			fetch_config.WithHeaders(map[string]string{
				"Content-Type": "application/x-www-form-urlencoded",
			}),
			fetch_config.WithBody([]byte(v.Encode())),
		},
		s.options...,
	)

	_, tokenResponse, err := utils.FetchJson[*token_response.Response](s.ctx, s.tokenURI, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json: %w", err), s.tokenURI)
	}
	if tokenResponse == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("token response"))
	}

	return tokenResponse.Token(), nil
}

func (s *TokenSource) CredentialsFile() *credentials_file.File {
	return s.credentialsFile
}

func (s *TokenSource) Signer() crypto.Signer {
	return s.privateKey
}

func (s *TokenSource) ClientEmail() string {
	return s.clientEmail
}

func NewFromCredentialsFile(
	ctx context.Context,
	tokenUrl string,
	credentialsFile *credentials_file.File,
	scopes []string,
	options ...fetch_config.Option,
) (*TokenSource, error) {
	return NewFromCredentialsFileWithSubject(ctx, tokenUrl, credentialsFile, scopes, "", options...)
}

// NewFromCredentialsFileWithSubject is like NewFromCredentialsFile but mints
// assertions that impersonate subject via the JWT "sub" claim — i.e. Google
// Workspace domain-wide delegation. Pass an empty subject for no impersonation.
func NewFromCredentialsFileWithSubject(
	ctx context.Context,
	tokenUrl string,
	credentialsFile *credentials_file.File,
	scopes []string,
	subject string,
	options ...fetch_config.Option,
) (*TokenSource, error) {
	if tokenUrl == "" {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.New("token url"))
	}

	if credentialsFile == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("credentials file"))
	}

	rsaKey, err := parsePrivateKey(credentialsFile.PrivateKey)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("parse private key: %w", err))
	}

	return &TokenSource{
		ctx:             ctx,
		clientEmail:     credentialsFile.ClientEmail,
		privateKeyID:    credentialsFile.PrivateKeyID,
		privateKey:      rsaKey,
		tokenURI:        tokenUrl,
		scopes:          scopes,
		subject:         subject,
		options:         options,
		credentialsFile: credentialsFile,
	}, nil
}
