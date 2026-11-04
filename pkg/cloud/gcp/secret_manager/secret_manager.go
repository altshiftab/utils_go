// Package secret_manager reads secrets from Google Secret Manager.
package secret_manager

import (
	"context"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"net/url"
	"strconv"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/secret_manager/secret_manager_config"
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

// AccessSecretVersion returns the payload of a secret's version, named as
// VersionName names it.
//
// The payload's checksum is verified when Secret Manager sends one, so a payload
// damaged on the way is an error rather than a secret read wrong.
func (c *Client) AccessSecretVersion(ctx context.Context, versionName string, options ...fetch_config.Option) ([]byte, error) {
	if versionName == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("version name"))
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	// The name's slashes stay literal: gRPC transcoding routes against the raw
	// path and rejects %2F-encoded separators.
	u := *c.baseUrl
	u.Path += versionName + ":access"
	urlString := u.String()

	options = append(append(c.config.FetchOptions, options...), fetch_config.WithMethod("GET"))
	_, response, err := altshiftHttpUtils.FetchJson[*accessResponse](ctx, urlString, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
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

	return data, nil
}
