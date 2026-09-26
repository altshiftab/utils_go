package cloud_sql

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token_source"
	altshiftOauth2Transport "github.com/altshiftab/utils_go/pkg/oauth2/types/transport"
)

const (
	DefaultTokenInfoUrl      = "https://oauth2.googleapis.com/tokeninfo"                   //nolint:gosec // G101: a public endpoint, not a credential
	DefaultServiceAccountUrl = "https://iam.googleapis.com/v1/projects/-/serviceAccounts/" //nolint:gosec // G101: a public endpoint, not a credential
)

type tokenInfo struct {
	Email string `json:"email,omitzero"`
	// Azp is the client the token was issued to; for a service account, its unique ID.
	Azp string `json:"azp,omitzero"`
}

type serviceAccount struct {
	Email string `json:"email,omitzero"`
}

// UsernameResolver works out the database username of the principal a token source speaks for. A
// user's token names its email; a service account's names only its unique ID, which the IAM API
// turns into the email.
type UsernameResolver struct {
	TokenInfoUrl      string
	ServiceAccountUrl string
}

func NewUsernameResolver() *UsernameResolver {
	return &UsernameResolver{TokenInfoUrl: DefaultTokenInfoUrl, ServiceAccountUrl: DefaultServiceAccountUrl}
}

func (r *UsernameResolver) Resolve(ctx context.Context, tokenSource token_source.TokenSource) (string, error) {
	if tokenSource == nil {
		return "", altshiftErrors.NewWithTrace(nil_error.New("token source"))
	}

	token, err := tokenSource.Token()
	if err != nil {
		return "", altshiftErrors.NewWithTrace(fmt.Errorf("token source token: %w", err))
	}
	if token == nil || token.AccessToken == "" {
		return "", altshiftErrors.NewWithTrace(empty_error.New("access token"))
	}

	infoUrl := r.TokenInfoUrl + "?" + url.Values{"access_token": {token.AccessToken}}.Encode()
	_, info, err := altshiftHttpUtils.FetchJson[*tokenInfo](ctx, infoUrl)
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("fetch json (token info): %w", err))
	}
	if info == nil {
		return "", altshiftErrors.NewWithTrace(nil_error.New("token info"))
	}

	if info.Email != "" {
		username, err := DatabaseUsername(info.Email)
		if err != nil {
			return "", altshiftErrors.New(fmt.Errorf("database username: %w", err), info.Email)
		}
		return username, nil
	}
	if info.Azp == "" {
		return "", altshiftErrors.NewWithTrace(empty_error.New("token email and client"))
	}

	accountUrl := r.ServiceAccountUrl + url.PathEscape(info.Azp)
	_, account, err := altshiftHttpUtils.FetchJson[*serviceAccount](
		ctx,
		accountUrl,
		fetch_config.WithHttpClient(&http.Client{Transport: &altshiftOauth2Transport.Transport{Source: tokenSource}}),
	)
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("fetch json (service account): %w", err), accountUrl)
	}
	if account == nil || account.Email == "" {
		return "", altshiftErrors.NewWithTrace(empty_error.New("service account email"), info.Azp)
	}

	username, err := DatabaseUsername("serviceAccount:" + account.Email)
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("database username: %w", err), account.Email)
	}

	return username, nil
}
