package gcp

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/gcp_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/credentials_file"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/token_source/authorized_user_token_source"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/token_source/metadata_token_source"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/types/token_source/service_account_token_source"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	motmedelHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token_source"
)

const (
	credentialTypeAuthorizedUser = "authorized_user"
	credentialTypeServiceAccount = "service_account"

	metadataFlavorHeader = "Metadata-Flavor"
	metadataFlavorGoogle = "Google"

	DefaultTokenUrl = "https://oauth2.googleapis.com/token" //nolint:gosec // G101: public OAuth token endpoint URL, not a credential
)

var (
	defaultMetadataBaseUrl = &url.URL{
		Scheme: "http",
		Host:   "metadata.google.internal",
		Path:   "/computeMetadata/v1",
	}
)

type Client struct {
	metadataBaseUrl *url.URL
	tokenUrl        string
	config          *gcp_config.Config
}

func NewClient(options ...gcp_config.Option) *Client {
	return NewClientWithUrls(defaultMetadataBaseUrl, DefaultTokenUrl, options...)
}

func NewClientWithUrls(metadataBaseUrl *url.URL, tokenUrl string, options ...gcp_config.Option) *Client {
	return &Client{
		metadataBaseUrl: metadataBaseUrl,
		tokenUrl:        tokenUrl,
		config:          gcp_config.New(options...),
	}
}

func (c *Client) GetIdToken(ctx context.Context, audience string, options ...fetch_config.Option) (string, error) {
	if audience == "" {
		return "", motmedelErrors.NewWithTrace(empty_error.New("audience"))
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("context err: %w", err)
	}

	identityUrl := *c.metadataBaseUrl
	identityUrl.Path += "/instance/service-accounts/default/identity"
	identityUrl.RawQuery = url.Values{"audience": {audience}}.Encode()

	identityUrlString := identityUrl.String()
	options = append(
		append(c.config.FetchOptions, fetch_config.WithHeaders(map[string]string{metadataFlavorHeader: metadataFlavorGoogle})),
		options...,
	)
	_, responseBody, err := motmedelHttpUtils.Fetch(
		ctx,
		identityUrlString,
		options...,
	)
	if err != nil {
		return "", motmedelErrors.New(fmt.Errorf("fetch: %w", err), identityUrlString)
	}

	return string(responseBody), nil
}

// GetServiceAccountEmail returns the email address of the runtime's default service
// account by querying the GCP metadata server. Only works when running inside a GCP
// environment that exposes the metadata server (Compute Engine, Cloud Run, GKE, etc.).
func (c *Client) GetServiceAccountEmail(ctx context.Context, options ...fetch_config.Option) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("context err: %w", err)
	}

	requestUrl := *c.metadataBaseUrl
	requestUrl.Path += "/instance/service-accounts/default/email"

	urlString := requestUrl.String()
	options = append(
		append(c.config.FetchOptions, fetch_config.WithHeaders(map[string]string{metadataFlavorHeader: metadataFlavorGoogle})),
		options...,
	)
	_, responseBody, err := motmedelHttpUtils.Fetch(
		ctx,
		urlString,
		options...,
	)
	if err != nil {
		return "", motmedelErrors.New(fmt.Errorf("fetch: %w", err), urlString)
	}

	return string(responseBody), nil
}

func (c *Client) GetProjectId(ctx context.Context, options ...fetch_config.Option) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("context err: %w", err)
	}

	requestUrl := *c.metadataBaseUrl
	requestUrl.Path += "/project/project-id"

	urlString := requestUrl.String()
	options = append(
		append(c.config.FetchOptions, fetch_config.WithHeaders(map[string]string{metadataFlavorHeader: metadataFlavorGoogle})),
		options...,
	)
	_, responseBody, err := motmedelHttpUtils.Fetch(
		ctx,
		urlString,
		options...,
	)
	if err != nil {
		return "", motmedelErrors.New(fmt.Errorf("fetch: %w", err), urlString)
	}

	return string(responseBody), nil
}

func wellKnownCredentialsPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "gcloud", "application_default_credentials.json")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// TODO: Use XDG?

	return filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
}

func (c *Client) credentialsFileTokenSource(ctx context.Context, data []byte, scopes []string, options ...fetch_config.Option) (token_source.TokenSource, error) {
	var credentialsFile credentials_file.File
	if err := json.Unmarshal(data, &credentialsFile); err != nil {
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf("json unmarshal (credentials file): %w", err),
		)
	}

	switch credentialsFile.Type {
	case credentialTypeAuthorizedUser:
		tokenSource, err := authorized_user_token_source.NewFromCredentialsFile(ctx, c.tokenUrl, &credentialsFile, options...)
		if err != nil {
			return nil, motmedelErrors.NewWithTrace(fmt.Errorf("authorized user token source new: %w", err), credentialsFile)
		}

		return token_source.NewReusable(nil, tokenSource), nil
	case credentialTypeServiceAccount:
		tokenUrl := credentialsFile.TokenURL
		if tokenUrl == "" {
			tokenUrl = c.tokenUrl
		}

		tokenSource, err := service_account_token_source.NewFromCredentialsFile(ctx, tokenUrl, &credentialsFile, scopes, options...)
		if err != nil {
			return nil, motmedelErrors.NewWithTrace(fmt.Errorf("service account token source new: %w", err), credentialsFile)
		}

		return token_source.NewReusable(nil, tokenSource), nil
	default:
		return nil, motmedelErrors.NewWithTrace(
			fmt.Errorf("%w: unsupported credential type: %s", motmedelErrors.ErrUnexpectedType, credentialsFile.Type),
		)
	}
}

// FindDefaultCredentials discovers Google credentials using the Application Default Credentials chain:
//  1. GOOGLE_APPLICATION_CREDENTIALS env var — reads the JSON file it points to.
//  2. Well-known file — user credentials from gcloud auth application-default login.
//  3. Metadata server — if running on GCP (Compute Engine, Cloud Run, etc.).
func (c *Client) FindDefaultCredentials(ctx context.Context, scopes []string, options ...fetch_config.Option) (token_source.TokenSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	// 1. GOOGLE_APPLICATION_CREDENTIALS env var.
	if envPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); envPath != "" {
		data, err := os.ReadFile(envPath) //nolint:gosec // ADC: reads the file GOOGLE_APPLICATION_CREDENTIALS points to
		if err != nil {
			return nil, motmedelErrors.NewWithTrace(fmt.Errorf("os read file: %w", err), envPath)
		}
		options = append(c.config.FetchOptions, options...)
		return c.credentialsFileTokenSource(ctx, data, scopes, options...)
	}

	// 2. Well-known file.
	if wellKnownPath := wellKnownCredentialsPath(); wellKnownPath != "" {
		if data, err := os.ReadFile(wellKnownPath); err == nil {
			options = append(c.config.FetchOptions, options...)
			return c.credentialsFileTokenSource(ctx, data, scopes, options...)
		}
	}

	// 3. Metadata server.
	if metadataBaseUrl := c.metadataBaseUrl; metadataBaseUrl != nil {
		options = append(c.config.FetchOptions, options...)
		metadataTokenSource, err := metadata_token_source.New(ctx, c.metadataBaseUrl, scopes, options...)
		if err != nil {
			return nil, motmedelErrors.NewWithTrace(fmt.Errorf("metadata token source new: %w", err))
		}
		return token_source.NewReusable(nil, metadataTokenSource), nil
	}

	return nil, motmedelErrors.NewWithTrace(nil_error.New("default credentials"))
}
