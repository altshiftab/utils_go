package artifact_registry

import (
	"context"
	"fmt"
	"net/url"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/artifact_registry_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/types/index"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/types/manifest"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	motmedelHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

const DomainSuffix = "docker.pkg.dev"

type Client struct {
	baseUrl *url.URL
	config  *artifact_registry_config.Config
}

func NewClient(location string, options ...artifact_registry_config.Option) *Client {
	config := artifact_registry_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = &url.URL{
			Scheme: "https",
			Host:   location + "-" + DomainSuffix,
		}
	}
	u := *baseUrl
	u.Path = "/v2/"

	return &Client{baseUrl: &u, config: config}
}

// GetManifest fetches an OCI image manifest by tag or digest.
// It returns the digest from the Docker-Content-Digest response header and the parsed manifest.
func (c *Client) GetManifest(ctx context.Context, name string, reference string, options ...fetch_config.Option) (string, *manifest.Manifest, error) {
	if name == "" {
		return "", nil, motmedelErrors.NewWithTrace(empty_error.New("name"))
	}
	if reference == "" {
		return "", nil, motmedelErrors.NewWithTrace(empty_error.New("reference"))
	}

	if err := ctx.Err(); err != nil {
		return "", nil, fmt.Errorf("context err: %w", err)
	}

	u := *c.baseUrl
	u.Path += name + "/manifests/" + reference
	urlString := u.String()

	options = append(
		append(c.config.FetchOptions, fetch_config.WithHeaders(map[string]string{
			"Accept": "application/vnd.oci.image.manifest.v1+json",
		})),
		options...,
	)

	response, m, err := motmedelHttpUtils.FetchJson[*manifest.Manifest](ctx, urlString, options...)
	if err != nil {
		return "", nil, motmedelErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
	}
	if m == nil {
		return "", nil, motmedelErrors.NewWithTrace(nil_error.New("manifest"))
	}

	var digest string
	if response != nil {
		digest = response.Header.Get("Docker-Content-Digest")
	}

	return digest, m, nil
}

// ListReferrers lists OCI artifacts that reference the given digest.
// An optional artifactType filter can be provided; pass an empty string to list all referrers.
func (c *Client) ListReferrers(ctx context.Context, name string, digest string, artifactType string, options ...fetch_config.Option) (*index.Index, error) {
	if name == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("name"))
	}
	if digest == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("digest"))
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	u := *c.baseUrl
	u.Path += name + "/referrers/" + digest
	if artifactType != "" {
		u.RawQuery = url.Values{"artifactType": {artifactType}}.Encode()
	}
	urlString := u.String()

	options = append(c.config.FetchOptions, options...)
	_, idx, err := motmedelHttpUtils.FetchJson[*index.Index](ctx, urlString, options...)
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
	}

	if idx == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("idx"))
	}

	return idx, nil
}

// GetBlob downloads a blob by digest and returns the raw bytes.
func (c *Client) GetBlob(ctx context.Context, name string, digest string, options ...fetch_config.Option) ([]byte, error) {
	if name == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("name"))
	}
	if digest == "" {
		return nil, motmedelErrors.NewWithTrace(empty_error.New("digest"))
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	u := *c.baseUrl
	u.Path += name + "/blobs/" + digest
	urlString := u.String()

	options = append(c.config.FetchOptions, options...)
	_, responseBody, err := motmedelHttpUtils.Fetch(ctx, urlString, options...)
	if err != nil {
		return nil, motmedelErrors.New(fmt.Errorf("fetch: %w", err), urlString)
	}

	return responseBody, nil
}
