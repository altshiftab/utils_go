package cloud_kms

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_kms/cloud_kms_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_kms/types/asymmetric_sign_request"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_kms/types/asymmetric_sign_response"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_kms/types/public_key"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

const Domain = "cloudkms.googleapis.com"

var defaultBaseUrl = &url.URL{
	Scheme: "https",
	Host:   Domain,
}

type Client struct {
	baseUrl *url.URL
	config  *cloud_kms_config.Config
}

func NewClient(options ...cloud_kms_config.Option) *Client {
	config := cloud_kms_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = defaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/v1/"

	return &Client{baseUrl: &u, config: config}
}

// keyVersionUrl addresses a method of a key version. The version is a resource
// name, projects/.../cryptoKeyVersions/N, and its slashes stay literal: gRPC
// transcoding routes against the raw path and rejects %2F-encoded separators.
func (c *Client) keyVersionUrl(keyVersion string, suffix string) string {
	u := *c.baseUrl
	u.Path += keyVersion + suffix
	return u.String()
}

// AsymmetricSign signs a SHA-256 digest with an asymmetric key version and
// returns the signature as KMS produced it: ASN.1 DER for an elliptic curve key,
// PKCS #1 for an RSA one.
//
// The digest is sent rather than the data it was taken of. That is what lets
// the data be of any size, and it is what KMS signs either way: a key version
// signs the digest of what it is given. The key never leaves KMS, so the caller
// needs roles/cloudkms.signerVerifier on the key and nothing that could read,
// export or destroy it.
func (c *Client) AsymmetricSign(
	ctx context.Context,
	keyVersion string,
	digest []byte,
	options ...fetch_config.Option,
) ([]byte, error) {
	if keyVersion == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("key version"))
	}
	if len(digest) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("digest"))
	}
	// A digest of the wrong length is a caller mistake KMS would answer with a
	// bare 400; saying what is wrong costs nothing here.
	if len(digest) != sha256.Size {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf(
				"%w: the digest is %d bytes, and a SHA-256 digest is %d",
				altshiftErrors.ErrValidationError, len(digest), sha256.Size,
			),
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	urlString := c.keyVersionUrl(keyVersion, ":asymmetricSign")

	body := &asymmetric_sign_request.Request{
		Digest: &asymmetric_sign_request.Digest{Sha256: base64.StdEncoding.EncodeToString(digest)},
	}

	options = append(append(c.config.FetchOptions, options...), fetch_config.WithMethod(http.MethodPost))
	_, response, err := altshiftHttpUtils.FetchJsonWithBody[*asymmetric_sign_response.Response](
		ctx, urlString, body, options...,
	)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json with body: %w", err), urlString)
	}
	if response == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("asymmetric sign response"))
	}
	if response.Signature == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("signature"))
	}

	signature, err := base64.StdEncoding.DecodeString(response.Signature)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("base64 std encoding decode string (signature): %w", err),
			response.Signature,
		)
	}

	return signature, nil
}

// GetPublicKey returns the public half of an asymmetric key version as KMS
// states it: the PEM-encoded key and the algorithm the version signs with.
//
// The algorithm is returned beside the key rather than folded into a parsed
// key, because the key alone does not say it: a P-256 key could sign over any
// digest, and a verifier that means to accept one combination checks for it.
// altshiftCrypto.PublicKeyFromPem parses the PEM into the key type expected.
//
// Reading it takes cloudkms.cryptoKeyVersions.viewPublicKey, which
// roles/cloudkms.publicKeyViewer grants without the right to sign.
func (c *Client) GetPublicKey(
	ctx context.Context,
	keyVersion string,
	options ...fetch_config.Option,
) (*public_key.PublicKey, error) {
	if keyVersion == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("key version"))
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	urlString := c.keyVersionUrl(keyVersion, "/publicKey")

	options = append(c.config.FetchOptions, options...)
	_, publicKey, err := altshiftHttpUtils.FetchJson[*public_key.PublicKey](ctx, urlString, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
	}
	if publicKey == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("public key"))
	}
	if publicKey.Pem == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("public key pem"))
	}

	return publicKey, nil
}
