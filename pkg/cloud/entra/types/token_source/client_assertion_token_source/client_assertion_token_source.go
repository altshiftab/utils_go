// Package client_assertion_token_source mints Microsoft Entra ID access tokens
// with the OAuth 2.0 client credentials grant, proving the application's
// identity with a JWT client assertion rather than a client secret.
package client_assertion_token_source

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token"
)

const (
	clientCredentialsGrantType = "client_credentials"
	jwtBearerAssertionType     = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" //nolint:gosec // G101: standard OAuth assertion-type URN, not a credential
)

// AssertionFunc returns a fresh client assertion: a JWT that Entra accepts as
// proof of the application's identity, such as a Google-issued ID token matched
// by a federated identity credential, or a JWT signed with a certificate key.
type AssertionFunc func(ctx context.Context) (string, error)

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type,omitzero"`
	ExpiresIn   int64  `json:"expires_in,omitzero"`
}

type TokenSource struct {
	ctx       context.Context //nolint:containedctx // The TokenSource interface takes no context; the construction context is deliberately captured (same pattern as x/oauth2).
	tokenUrl  string
	clientId  string
	scopes    []string
	assertion AssertionFunc
	options   []fetch_config.Option
}

func (s *TokenSource) Token() (*token.Token, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	assertion, err := s.assertion(s.ctx)
	if err != nil {
		return nil, fmt.Errorf("assertion: %w", err)
	}
	if assertion == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("client assertion"))
	}

	form := url.Values{
		"grant_type":            {clientCredentialsGrantType},
		"client_id":             {s.clientId},
		"scope":                 {strings.Join(s.scopes, " ")},
		"client_assertion_type": {jwtBearerAssertionType},
		"client_assertion":      {assertion},
	}

	options := append(
		[]fetch_config.Option{
			fetch_config.WithMethod(http.MethodPost),
			fetch_config.WithHeaders(map[string]string{"Content-Type": "application/x-www-form-urlencoded"}),
			fetch_config.WithBody([]byte(form.Encode())),
		},
		s.options...,
	)

	_, response, err := altshiftHttpUtils.FetchJson[*tokenResponse](s.ctx, s.tokenUrl, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json: %w", err), s.tokenUrl)
	}
	if response == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("token response"))
	}
	if response.AccessToken == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("access token"))
	}

	tok := &token.Token{AccessToken: response.AccessToken, TokenType: response.TokenType}
	if response.ExpiresIn > 0 {
		tok.Expiry = time.Now().Add(time.Duration(response.ExpiresIn) * time.Second)
	}

	return tok, nil
}

// New returns a token source exchanging assertions from assertion for access
// tokens at tokenUrl, the v2.0 token endpoint of one specific tenant; Entra
// refuses app-only tokens through "common" or "organizations".
func New(
	ctx context.Context,
	tokenUrl string,
	clientId string,
	scopes []string,
	assertion AssertionFunc,
	options ...fetch_config.Option,
) (*TokenSource, error) {
	if tokenUrl == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("token url"))
	}
	if clientId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("client id"))
	}
	if len(scopes) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("scopes"))
	}
	if assertion == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("assertion func"))
	}

	return &TokenSource{
		ctx:       ctx,
		tokenUrl:  tokenUrl,
		clientId:  clientId,
		scopes:    scopes,
		assertion: assertion,
		options:   options,
	}, nil
}
