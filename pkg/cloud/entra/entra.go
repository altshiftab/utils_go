// Package entra obtains Microsoft Entra ID (Azure AD) access tokens for
// app-only calls to Microsoft APIs such as Microsoft Graph.
//
// A failed token request carries its form, client assertion included, in the
// error's HTTP context; services logging errors should mask the request bodies
// of TokenUrl's endpoints.
package entra

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/altshiftab/utils_go/pkg/cloud/entra/entra_config"
	"github.com/altshiftab/utils_go/pkg/cloud/entra/types/token_source/client_assertion_token_source"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token_source"
)

const (
	DefaultAuthorityHost = "login.microsoftonline.com"

	// FederatedAudience is the audience Entra requires of an external token
	// presented through a federated identity credential.
	FederatedAudience = "api://AzureADTokenExchange"

	// ScopeMicrosoftGraph requests the application permissions granted to the
	// app on Microsoft Graph; app-only tokens take only ".default" scopes.
	ScopeMicrosoftGraph = "https://graph.microsoft.com/.default"
)

// multiTenantAliases are the authority aliases Entra refuses for app-only tokens.
var multiTenantAliases = []string{"common", "organizations", "consumers"}

type Client struct {
	authorityUrl *url.URL
	config       *entra_config.Config
}

func NewClient(options ...entra_config.Option) *Client {
	config := entra_config.New(options...)
	authorityUrl := config.AuthorityUrl
	if authorityUrl == nil {
		authorityUrl = &url.URL{Scheme: "https", Host: DefaultAuthorityHost}
	}
	return &Client{authorityUrl: authorityUrl, config: config}
}

// TokenUrl returns the v2.0 token endpoint of the tenant identified by tenantId
// (a GUID or a verified domain).
func (c *Client) TokenUrl(tenantId string) string {
	u := *c.authorityUrl
	u.Path = "/" + tenantId + "/oauth2/v2.0/token"
	u.RawPath = "/" + url.PathEscape(tenantId) + "/oauth2/v2.0/token"
	return u.String()
}

// NewClientAssertionTokenSource returns a caching token source for the app
// clientId in the tenant tenantId, authenticating with assertions from assertion.
func (c *Client) NewClientAssertionTokenSource(
	ctx context.Context,
	tenantId string,
	clientId string,
	scopes []string,
	assertion client_assertion_token_source.AssertionFunc,
) (token_source.TokenSource, error) {
	if tenantId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("tenant id"))
	}
	if slices.Contains(multiTenantAliases, strings.ToLower(tenantId)) {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: app-only tokens need a concrete tenant, not %q", altshiftErrors.ErrValidationError, tenantId),
		)
	}

	tokenSource, err := client_assertion_token_source.New(
		ctx,
		c.TokenUrl(tenantId),
		clientId,
		scopes,
		assertion,
		c.config.FetchOptions...,
	)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("client assertion token source new: %w", err), tenantId, clientId)
	}

	return token_source.NewReusable(nil, tokenSource), nil
}

// NewGoogleFederatedTokenSource returns a caching token source that
// authenticates without a secret, presenting the ID token of the GCP runtime's
// service account. The app registration needs a federated identity credential
// with issuer https://accounts.google.com, the service account's numeric unique
// id as subject, and FederatedAudience as audience. A multi-tenant app uses
// the same credential in every tenant that has consented to it.
func (c *Client) NewGoogleFederatedTokenSource(
	ctx context.Context,
	gcpClient *gcp.Client,
	tenantId string,
	clientId string,
	scopes []string,
) (token_source.TokenSource, error) {
	if gcpClient == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("gcp client"))
	}

	return c.NewClientAssertionTokenSource(
		ctx,
		tenantId,
		clientId,
		scopes,
		func(ctx context.Context) (string, error) {
			return gcpClient.GetIdToken(ctx, FederatedAudience)
		},
	)
}
