// Package secret_manager reads secrets from Google Secret Manager.
package secret_manager

import (
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/secret_manager/secret_manager_config"
	"github.com/altshiftab/utils_go/pkg/cloud/internal/rest"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

const Domain = "secretmanager.googleapis.com"

// Scope is the OAuth scope a token for this API needs; what the token may read
// is the account's roles, roles/secretmanager.secretAccessor on the secret.
const Scope = "https://www.googleapis.com/auth/cloud-platform"

var defaultBaseUrl = &url.URL{
	Scheme: "https",
	Host:   Domain,
}

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

type payload struct {
	Data       string `json:"data"`
	DataCrc32c string `json:"dataCrc32c"`
}

type accessResponse struct {
	Name    string   `json:"name"`
	Payload *payload `json:"payload"`
}

type addVersionRequest struct {
	Payload *payload `json:"payload"`
}

// SecretVersion is one version of a secret: its resource name, with the version's number rather than "latest", and,
// read, its payload.
type SecretVersion struct {
	Name    string
	Payload []byte
}

type versionResponse struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type Client struct {
	baseUrl *url.URL
	config  *secret_manager_config.Config
}

func NewClient(options ...secret_manager_config.Option) *Client {
	config := secret_manager_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = defaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/v1/"

	return &Client{baseUrl: &u, config: config}
}

// VersionName is the resource name of a secret's version: a number, or "latest"
// for the newest one enabled.
func VersionName(project string, secret string, version string) string {
	return "projects/" + project + "/secrets/" + secret + "/versions/" + version
}

// AccessSecretVersion reads a secret's version, named as VersionName names it: its payload, and its name with the
// version's number, which "latest" stands for.
//
// The payload's checksum is verified when Secret Manager sends one, so a payload damaged on the way is an error rather
// than a secret read wrong.
func (c *Client) AccessSecretVersion(ctx context.Context, versionName string, options ...fetch_config.Option) (*SecretVersion, error) {
	if versionName == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("version name"))
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	urlString := c.resourceUrl(versionName, ":access")

	// Fetched and read here rather than by FetchJson, which puts a body it cannot read into its error: this one is a
	// secret.
	options = append(append(slices.Clip(c.config.FetchOptions), options...), fetch_config.WithMethod(http.MethodGet))
	_, body, err := altshiftHttpUtils.Fetch(ctx, urlString, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch: %w", err), urlString)
	}
	var response *accessResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: the access response is not json", altshiftErrors.ErrParseError))
	}
	if response == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("access response"))
	}
	if response.Payload == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("payload"))
	}

	data, err := base64.StdEncoding.DecodeString(response.Payload.Data)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("base64 std encoding decode string (payload): %w", err))
	}

	if response.Payload.DataCrc32c != "" {
		want, err := strconv.ParseUint(response.Payload.DataCrc32c, 10, 32)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("strconv parse uint (data crc32c): %w", err),
				response.Payload.DataCrc32c,
			)
		}
		if got := crc32.Checksum(data, castagnoli); uint64(got) != want {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: the payload's crc32c is %d, and secret manager said %d", altshiftErrors.ErrVerificationError, got, want),
			)
		}
	}

	return &SecretVersion{Name: response.Name, Payload: data}, nil
}

// SecretName is the resource name of a secret.
func SecretName(project string, secret string) string {
	return "projects/" + project + "/secrets/" + secret
}

// AddSecretVersion makes the payload the secret's newest version, and returns the version's name. It takes
// roles/secretmanager.secretVersionAdder on the secret.
//
// An empty payload is refused here, Secret Manager accepting one: a version that holds nothing is a writer that failed
// before it had anything to write, and it would be served as "latest" all the same.
func (c *Client) AddSecretVersion(ctx context.Context, secretName string, data []byte, options ...fetch_config.Option) (string, error) {
	if secretName == "" {
		return "", altshiftErrors.NewWithTrace(empty_error.New("secret name"))
	}
	if len(data) == 0 {
		return "", altshiftErrors.NewWithTrace(empty_error.New("payload"))
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("context err: %w", err)
	}

	urlString := c.resourceUrl(secretName, ":addVersion")
	body := &addVersionRequest{Payload: &payload{
		Data:       base64.StdEncoding.EncodeToString(data),
		DataCrc32c: strconv.FormatUint(uint64(crc32.Checksum(data, castagnoli)), 10),
	}}

	options = append(append(slices.Clip(c.config.FetchOptions), options...), fetch_config.WithMethod(http.MethodPost))
	_, response, err := altshiftHttpUtils.FetchJsonWithBody[*versionResponse](ctx, urlString, body, options...)
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("fetch json with body: %w", err), urlString)
	}
	if response == nil {
		return "", altshiftErrors.NewWithTrace(nil_error.New("version response"))
	}
	if response.Name == "" {
		return "", altshiftErrors.NewWithTrace(empty_error.New("version name"))
	}

	return response.Name, nil
}

// Version states, as Secret Manager names them.
const (
	StateEnabled   = "ENABLED"
	StateDisabled  = "DISABLED"
	StateDestroyed = "DESTROYED"
)

// VersionState is a version as a listing gives it: its name and its state.
type VersionState struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type listVersionsResponse struct {
	Versions      []*VersionState `json:"versions"`
	NextPageToken string          `json:"nextPageToken"`
}

// ListSecretVersions lists every version of the secret, the destroyed ones too, newest first. It takes
// roles/secretmanager.viewer on the secret, or a role that holds secretmanager.versions.list.
func (c *Client) ListSecretVersions(ctx context.Context, secretName string, options ...fetch_config.Option) ([]*VersionState, error) {
	if secretName == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("secret name"))
	}

	return rest.ListPaginated(
		ctx,
		func(pageToken string) string {
			u := *c.baseUrl
			u.Path += secretName + "/versions"
			if pageToken != "" {
				u.RawQuery = url.Values{"pageToken": {pageToken}}.Encode()
			}
			return u.String()
		},
		func(response *listVersionsResponse) ([]*VersionState, string) {
			return response.Versions, response.NextPageToken
		},
		append(slices.Clip(c.config.FetchOptions), options...),
	)
}

// DestroySecretVersion destroys a version, its payload gone for good: what a secret held before it was replaced is
// still read by whoever may read the secret, until its version is destroyed. It takes
// roles/secretmanager.secretVersionManager on the secret.
func (c *Client) DestroySecretVersion(ctx context.Context, versionName string, options ...fetch_config.Option) error {
	if versionName == "" {
		return altshiftErrors.NewWithTrace(empty_error.New("version name"))
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context err: %w", err)
	}

	urlString := c.resourceUrl(versionName, ":destroy")

	options = append(append(slices.Clip(c.config.FetchOptions), options...), fetch_config.WithMethod(http.MethodPost))
	if _, _, err := altshiftHttpUtils.FetchJsonWithBody[*versionResponse](ctx, urlString, struct{}{}, options...); err != nil {
		return altshiftErrors.New(fmt.Errorf("fetch json with body: %w", err), urlString)
	}

	return nil
}

// resourceUrl addresses a method of a resource. The name's slashes stay literal: gRPC transcoding routes against the
// raw path and rejects %2F-encoded separators.
func (c *Client) resourceUrl(name string, method string) string {
	u := *c.baseUrl
	u.Path += name + method
	return u.String()
}
