package authorized_user_token_source

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/credentials_file"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/token_response"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	"github.com/altshiftab/utils_go/pkg/http/utils"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token"
)

type TokenSource struct {
	ctx          context.Context //nolint:containedctx // The TokenSource interface takes no context; the construction context is deliberately captured (same pattern as x/oauth2).
	clientID     string
	clientSecret string
	refreshToken string
	tokenUrl     string
	options      []fetch_config.Option

	credentialsFile *credentials_file.File
}

func (ts *TokenSource) Token() (*token.Token, error) {
	if err := ts.ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	v := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {ts.clientID},
		"client_secret": {ts.clientSecret},
		"refresh_token": {ts.refreshToken},
	}

	options := append(
		[]fetch_config.Option{
			fetch_config.WithMethod(http.MethodPost),
			fetch_config.WithHeaders(map[string]string{
				"Content-Type": "application/x-www-form-urlencoded",
			}),
			fetch_config.WithBody([]byte(v.Encode())),
		},
		ts.options...,
	)

	_, tokenResponse, err := utils.FetchJson[*token_response.Response](ts.ctx, ts.tokenUrl, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json: %w", err), ts.tokenUrl)
	}
	if tokenResponse == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("token response"))
	}

	return tokenResponse.Token(), nil
}

func (ts *TokenSource) CredentialsFile() *credentials_file.File {
	return ts.credentialsFile
}

func NewFromCredentialsFile(
	ctx context.Context,
	tokenUrl string,
	credentialsFile *credentials_file.File,
	options ...fetch_config.Option,
) (*TokenSource, error) {
	if tokenUrl == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("token url"))
	}

	if credentialsFile == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("credentials file"))
	}

	return &TokenSource{
		ctx:             ctx,
		clientID:        credentialsFile.ClientID,
		clientSecret:    credentialsFile.ClientSecret,
		refreshToken:    credentialsFile.RefreshToken,
		tokenUrl:        tokenUrl,
		options:         options,
		credentialsFile: credentialsFile,
	}, nil
}
