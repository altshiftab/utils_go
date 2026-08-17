package oidc

import (
	"context"
	"fmt"
	"net/url"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	motmedelHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
	"github.com/altshiftab/utils_go/pkg/json/oidc/types/provider_metadata"
)

func FetchProviderMetadata(
	ctx context.Context,
	providerUrl *url.URL,
	options ...fetch_config.Option,
) (*provider_metadata.Metadata, error) {
	if providerUrl == nil {
		return nil, nil
	}

	metadataUrl := *providerUrl
	metadataUrl.Path = "/.well-known/openid-configuration"

	providerUrlString := metadataUrl.String()
	_, metadata, err := motmedelHttpUtils.FetchJson[*provider_metadata.Metadata](ctx, providerUrlString, options...)
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("fetch json: %w", err), providerUrlString)
	}

	return metadata, nil
}
