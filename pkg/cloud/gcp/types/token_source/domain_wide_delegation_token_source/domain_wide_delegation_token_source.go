package domain_wide_delegation_token_source

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/iam_credentials"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/token_response"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token_source"
	altshiftOauth2Transport "github.com/altshiftab/utils_go/pkg/oauth2/types/transport"
)

const (
	defaultTokenURL    = "https://oauth2.googleapis.com/token"         //nolint:gosec // G101: public OAuth token endpoint URL, not a credential
	jwtBearerGrantType = "urn:ietf:params:oauth:grant-type:jwt-bearer" //nolint:gosec // G101: standard OAuth grant-type URN, not a credential
)

// TokenSource mints Google access tokens via Google Workspace domain-wide
// delegation without a service account key. It builds a delegation assertion
// (impersonating subject) and has the IAM Credentials API signJwt method sign it
// as saEmail — authority is proven with roles/iam.serviceAccountTokenCreator
// (held by the signer) rather than a downloaded key. The signed assertion is then
// exchanged for an access token at the OAuth token endpoint.
type TokenSource struct {
	ctx                  context.Context //nolint:containedctx // The TokenSource interface takes no context; the construction context is deliberately captured (same pattern as x/oauth2).
	iamCredentialsClient *iam_credentials.Client
	signerSource         token_source.TokenSource
	saEmail              string
	subject              string
	scopes               []string
	tokenURL             string
}

func (s *TokenSource) Token() (*token.Token, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	now := time.Now()

	claimsJSON, err := json.Marshal(map[string]any{
		"iss":   s.saEmail,
		"sub":   s.subject,
		"scope": strings.Join(s.scopes, " "),
		"aud":   s.tokenURL,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json marshal (jwt claims): %w", err))
	}

	// Authenticate the signJwt call as the signer (e.g. Application Default
	// Credentials), which must hold roles/iam.serviceAccountTokenCreator on saEmail.
	signerOption := fetch_config.WithHttpClient(&http.Client{
		Transport: &altshiftOauth2Transport.Transport{Source: s.signerSource},
	})

	signResponse, err := s.iamCredentialsClient.SignJwt(s.ctx, s.saEmail, string(claimsJSON), signerOption)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("iam credentials sign jwt: %w", err))
	}
	if signResponse == nil || signResponse.SignedJwt == "" {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("signed jwt"))
	}

	return s.exchange(signResponse.SignedJwt)
}

// exchange trades the signed delegation assertion for an access token.
func (s *TokenSource) exchange(assertion string) (*token.Token, error) {
	form := url.Values{
		"grant_type": {jwtBearerGrantType},
		"assertion":  {assertion},
	}

	options := []fetch_config.Option{
		fetch_config.WithMethod(http.MethodPost),
		fetch_config.WithHeaders(map[string]string{"Content-Type": "application/x-www-form-urlencoded"}),
		fetch_config.WithBody([]byte(form.Encode())),
	}

	_, tokenResponse, err := altshiftHttpUtils.FetchJson[*token_response.Response](s.ctx, s.tokenURL, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json: %w", err), s.tokenURL)
	}
	if tokenResponse == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("token response"))
	}

	return tokenResponse.Token(), nil
}

// New returns a domain-wide-delegation token source. signerSource authenticates
// the IAM Credentials signJwt call (typically Application Default Credentials with
// the cloud-platform scope). saEmail is the service account with domain-wide
// delegation; subject is the user to impersonate. tokenURL falls back to Google's
// endpoint when empty. The returned source caches and refreshes tokens as they expire.
func New(
	ctx context.Context,
	signerSource token_source.TokenSource,
	saEmail string,
	subject string,
	scopes []string,
	tokenURL string,
) (token_source.TokenSource, error) {
	if signerSource == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("signer source"))
	}
	if saEmail == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("service account email"))
	}
	if subject == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("subject"))
	}
	if tokenURL == "" {
		tokenURL = defaultTokenURL
	}

	src := &TokenSource{
		ctx:                  ctx,
		iamCredentialsClient: iam_credentials.NewClient(),
		signerSource:         signerSource,
		saEmail:              saEmail,
		subject:              subject,
		scopes:               scopes,
		tokenURL:             tokenURL,
	}
	return token_source.NewReusable(nil, src), nil
}
