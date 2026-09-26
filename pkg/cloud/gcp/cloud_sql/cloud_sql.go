package cloud_sql

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/cloud_sql_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/types/connect_settings"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/types/generate_ephemeral_cert_request"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_sql/types/generate_ephemeral_cert_response"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

const (
	Domain = "sqladmin.googleapis.com"

	// LoginScope is the OAuth scope a token needs to log into a database as its IAM principal.
	LoginScope = "https://www.googleapis.com/auth/sqlservice.login"
	AdminScope = "https://www.googleapis.com/auth/sqlservice.admin"

	serviceAccountSuffix = ".gserviceaccount.com"
)

var defaultBaseUrl = &url.URL{
	Scheme: "https",
	Host:   Domain,
}

// Client calls the two Cloud SQL Admin API methods a connection needs. Authentication is the
// caller's: pass an authenticating http.Client through cloud_sql_config.WithFetchOptions.
type Client struct {
	baseUrl *url.URL
	config  *cloud_sql_config.Config
}

func NewClient(options ...cloud_sql_config.Option) *Client {
	config := cloud_sql_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = defaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/sql/v1beta4/"

	return &Client{baseUrl: &u, config: config}
}

func (c *Client) instanceUrl(project string, instance string, suffix string) string {
	u := *c.baseUrl
	u.Path += "projects/" + project + "/instances/" + instance + suffix
	u.RawPath = c.baseUrl.Path + "projects/" + url.PathEscape(project) + "/instances/" + url.PathEscape(instance) + suffix
	return u.String()
}

// GetConnectSettings returns the instance's server CA, addresses and DNS name.
func (c *Client) GetConnectSettings(ctx context.Context, project string, instance string, options ...fetch_config.Option) (*connect_settings.ConnectSettings, error) {
	if project == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("project"))
	}
	if instance == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("instance"))
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	urlString := c.instanceUrl(project, instance, "/connectSettings")

	options = slices.Concat(c.config.FetchOptions, options)
	_, response, err := altshiftHttpUtils.FetchJson[*connect_settings.ConnectSettings](ctx, urlString, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
	}
	if response == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("connect settings response"))
	}

	return response, nil
}

// GenerateEphemeralCert has the instance's CA sign publicKeyPem for a client certificate. When
// accessToken is set, the certificate carries it, and the database accepts it in place of a
// password for the token's principal.
func (c *Client) GenerateEphemeralCert(
	ctx context.Context,
	project string,
	instance string,
	publicKeyPem string,
	accessToken string,
	options ...fetch_config.Option,
) (*generate_ephemeral_cert_response.Response, error) {
	if project == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("project"))
	}
	if instance == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("instance"))
	}
	if publicKeyPem == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("public key"))
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	urlString := c.instanceUrl(project, instance, ":generateEphemeralCert")
	body := &generate_ephemeral_cert_request.Request{PublicKey: publicKeyPem, AccessToken: accessToken}

	options = slices.Concat(c.config.FetchOptions, options, []fetch_config.Option{fetch_config.WithMethod(http.MethodPost)})
	_, response, err := altshiftHttpUtils.FetchJsonWithBody[*generate_ephemeral_cert_response.Response](ctx, urlString, body, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json with body: %w", err), urlString)
	}
	if response == nil || response.EphemeralCert == nil || response.EphemeralCert.Cert == "" {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("ephemeral cert"))
	}

	return response, nil
}

// DatabaseUsername returns the Postgres role Cloud SQL creates for an IAM principal, given as an
// IAM member (`serviceAccount:x`, `user:x`, `group:x`) or a bare email. A service account's role
// drops the `.gserviceaccount.com` suffix; every name is lowercase.
func DatabaseUsername(principal string) (string, error) {
	email := principal
	var kind string
	if before, after, found := strings.Cut(principal, ":"); found {
		kind, email = before, after
	}

	switch kind {
	case "", "user", "group", "serviceAccount":
	default:
		return "", altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: unsupported principal type: %s", altshiftErrors.ErrValidationError, kind),
			principal,
		)
	}

	if email == "" || !strings.Contains(email, "@") {
		return "", altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: principal is not an email", altshiftErrors.ErrValidationError),
			principal,
		)
	}

	email = strings.ToLower(email)
	if kind == "serviceAccount" || (kind == "" && strings.HasSuffix(email, serviceAccountSuffix)) {
		email = strings.TrimSuffix(email, serviceAccountSuffix)
	}

	return email, nil
}
