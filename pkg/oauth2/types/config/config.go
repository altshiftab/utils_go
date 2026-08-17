package config

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
	oauth2Errors "github.com/altshiftab/utils_go/pkg/oauth2/errors"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/auth_code_option"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/endpoint"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token_source"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/transport"
)

const paramGrantType = "grant_type"

type Config struct {
	ClientID     string
	ClientSecret string
	Endpoint     endpoint.Endpoint
	RedirectURL  string
	Scopes       []string

	FetchOptions []fetch_config.Option
}

func (c *Config) AuthCodeURL(state string, opts ...auth_code_option.AuthCodeOption) string {
	var buf strings.Builder
	buf.WriteString(c.Endpoint.AuthURL)

	v := url.Values{
		"response_type": {"code"},
		"client_id":     {c.ClientID},
	}
	if c.RedirectURL != "" {
		v.Set("redirect_uri", c.RedirectURL)
	}
	if len(c.Scopes) > 0 {
		v.Set("scope", strings.Join(c.Scopes, " "))
	}
	if state != "" {
		v.Set("state", state)
	}
	for _, opt := range opts {
		v.Set(opt.Key, opt.Value)
	}

	if strings.Contains(c.Endpoint.AuthURL, "?") {
		buf.WriteByte('&')
	} else {
		buf.WriteByte('?')
	}
	buf.WriteString(v.Encode())

	return buf.String()
}

func (c *Config) Exchange(ctx context.Context, code string, opts ...auth_code_option.AuthCodeOption) (*token.Token, error) {
	v := url.Values{
		paramGrantType: {"authorization_code"},
		"code":         {code},
	}
	if c.RedirectURL != "" {
		v.Set("redirect_uri", c.RedirectURL)
	}
	for _, opt := range opts {
		v.Set(opt.Key, opt.Value)
	}

	return c.retrieveToken(ctx, v)
}

func (c *Config) PasswordCredentialsToken(ctx context.Context, username, password string) (*token.Token, error) {
	v := url.Values{
		paramGrantType: {"password"},
		"username":     {username},
		"password":     {password},
	}
	if len(c.Scopes) > 0 {
		v.Set("scope", strings.Join(c.Scopes, " "))
	}

	return c.retrieveToken(ctx, v)
}

func (c *Config) ClientCredentialsToken(ctx context.Context) (*token.Token, error) {
	v := url.Values{
		paramGrantType: {"client_credentials"},
	}
	if len(c.Scopes) > 0 {
		v.Set("scope", strings.Join(c.Scopes, " "))
	}

	return c.retrieveToken(ctx, v)
}

func (c *Config) TokenSource(ctx context.Context, t *token.Token) token_source.TokenSource {
	tkr := &tokenRefresher{
		ctx:  ctx,
		conf: c,
	}
	if t != nil {
		tkr.refreshToken = t.RefreshToken
	}

	return token_source.NewReusable(t, tkr)
}

// Client returns an *http.Client that automatically sets OAuth2 authorization
// headers using a token source that refreshes the provided token as needed.
func (c *Config) Client(ctx context.Context, t *token.Token) *http.Client {
	return &http.Client{
		Transport: &transport.Transport{
			Source: c.TokenSource(ctx, t),
		},
	}
}

func (c *Config) retrieveToken(ctx context.Context, v url.Values) (*token.Token, error) {
	if c.Endpoint.TokenURL == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("token url"))
	}

	authStyle := c.Endpoint.AuthStyle

	if authStyle == endpoint.AuthStyleAutoDetect {
		// Try header first, fall back to params.
		tok, err := c.doRetrieveToken(ctx, v, endpoint.AuthStyleInHeader)
		if err == nil {
			return tok, nil
		}
		tok, err = c.doRetrieveToken(ctx, v, endpoint.AuthStyleInParams)
		if err == nil {
			return tok, nil
		}
		return nil, fmt.Errorf("do retrieve token: %w", err)
	}

	return c.doRetrieveToken(ctx, v, authStyle)
}

func (c *Config) doRetrieveToken(ctx context.Context, v url.Values, authStyle endpoint.AuthStyle) (*token.Token, error) {
	headers := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	}

	switch authStyle {
	case endpoint.AuthStyleInHeader:
		headers["Authorization"] = "Basic " + altshiftHttpUtils.BasicAuth(c.ClientID, c.ClientSecret)
	case endpoint.AuthStyleInParams:
		v.Set("client_id", c.ClientID)
		if c.ClientSecret != "" {
			v.Set("client_secret", c.ClientSecret)
		}
	default:
		// AuthStyleAutoDetect is resolved by retrieveToken before this is called.
	}

	body := []byte(v.Encode())

	options := append(
		[]fetch_config.Option{
			fetch_config.WithMethod("POST"),
			fetch_config.WithHeaders(headers),
			fetch_config.WithBody(body),
			fetch_config.WithSkipErrorOnStatus(true),
		},
		c.FetchOptions...,
	)

	response, responseBody, err := altshiftHttpUtils.Fetch(ctx, c.Endpoint.TokenURL, options...)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	if response != nil && (response.StatusCode < 200 || response.StatusCode >= 300) {
		retrieveErr := &oauth2Errors.RetrieveError{
			StatusCode: response.StatusCode,
			Body:       responseBody,
		}

		var errorResponse struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
			ErrorURI         string `json:"error_uri"`
		}
		if err := json.Unmarshal(responseBody, &errorResponse); err == nil {
			retrieveErr.ErrorCode = errorResponse.Error
			retrieveErr.ErrorDescription = errorResponse.ErrorDescription
			retrieveErr.ErrorURI = errorResponse.ErrorURI
		}

		return nil, retrieveErr
	}

	var tokenResponse struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}

	contentType := ""
	if response != nil {
		contentType = response.Header.Get("Content-Type")
	}

	if strings.Contains(contentType, "application/x-www-form-urlencoded") || strings.Contains(contentType, "text/plain") {
		vals, err := url.ParseQuery(string(responseBody))
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("url parse query (response body): %w", err), responseBody)
		}

		tokenResponse.AccessToken = vals.Get("access_token")
		tokenResponse.TokenType = vals.Get("token_type")
		tokenResponse.RefreshToken = vals.Get("refresh_token")
	} else {
		if err := json.Unmarshal(responseBody, &tokenResponse); err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("json unmarshal (response body): %w", err), responseBody)
		}
	}

	tok := &token.Token{
		AccessToken:  tokenResponse.AccessToken,
		TokenType:    tokenResponse.TokenType,
		RefreshToken: tokenResponse.RefreshToken,
	}

	if tokenResponse.ExpiresIn > 0 {
		tok.Expiry = time.Now().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second)
	}

	var raw map[string]any
	if err := json.Unmarshal(responseBody, &raw); err == nil {
		tok.Raw = raw
	}

	if tok.AccessToken == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("access token"))
	}

	return tok, nil
}

type tokenRefresher struct {
	ctx          context.Context //nolint:containedctx // The TokenSource interface takes no context; the construction context is deliberately captured (same pattern as x/oauth2).
	conf         *Config
	refreshToken string
}

func (tf *tokenRefresher) Token() (*token.Token, error) {
	if tf.refreshToken == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("refresh token"))
	}

	v := url.Values{
		paramGrantType:  {"refresh_token"},
		"refresh_token": {tf.refreshToken},
	}
	if len(tf.conf.Scopes) > 0 {
		v.Set("scope", strings.Join(tf.conf.Scopes, " "))
	}

	tok, err := tf.conf.retrieveToken(tf.ctx, v)
	if err != nil {
		return nil, fmt.Errorf("retrieve token: %w", err)
	}

	if tok.RefreshToken == "" {
		tok.RefreshToken = tf.refreshToken
	} else {
		tf.refreshToken = tok.RefreshToken
	}

	return tok, nil
}
