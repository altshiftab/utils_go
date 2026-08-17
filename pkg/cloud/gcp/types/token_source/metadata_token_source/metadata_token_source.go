package metadata_token_source

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/token_response"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	"github.com/altshiftab/utils_go/pkg/http/utils"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token"
)

type TokenSource struct {
	ctx             context.Context //nolint:containedctx // The TokenSource interface takes no context; the construction context is deliberately captured (same pattern as x/oauth2).
	metadataBaseUrl *url.URL
	scopes          []string
	options         []fetch_config.Option
}

func (s *TokenSource) Token() (*token.Token, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	tokenUrl := *s.metadataBaseUrl
	tokenUrl.Path += "/instance/service-accounts/default/token"
	if len(s.scopes) > 0 {
		tokenUrl.RawQuery = url.Values{"scopes": {strings.Join(s.scopes, ",")}}.Encode()
	}

	urlString := tokenUrl.String()

	options := append(
		[]fetch_config.Option{
			fetch_config.WithHeaders(map[string]string{"Metadata-Flavor": "Google"}),
		},
		s.options...,
	)

	_, tokenResponse, err := utils.FetchJson[*token_response.Response](s.ctx, urlString, options...)
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
	}
	if tokenResponse == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("token response"))
	}

	return tokenResponse.Token(), nil
}

func New(
	ctx context.Context,
	metadataBaseUrl *url.URL,
	scopes []string,
	options ...fetch_config.Option,
) (*TokenSource, error) {
	if metadataBaseUrl == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("metadata base url"))
	}

	return &TokenSource{
		ctx:             ctx,
		metadataBaseUrl: metadataBaseUrl,
		scopes:          scopes,
		options:         options,
	}, nil
}
