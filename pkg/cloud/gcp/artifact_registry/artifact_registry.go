package artifact_registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/artifact_registry_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/types/descriptor"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/types/index"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/artifact_registry/types/manifest"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

const DomainSuffix = "docker.pkg.dev"

// The manifest media types a registry may answer with. An image pushed by
// Docker is described by a Docker manifest and one pushed by BuildKit or an OCI
// tool by an OCI one, and a reference may name a multi-platform list of either.
// MediaTypeOciEmpty is the config of an artifact that has none, such as a
// signature or an attestation attached to an image as its referrer.
const (
	MediaTypeOciManifest        = "application/vnd.oci.image.manifest.v1+json"
	MediaTypeOciIndex           = "application/vnd.oci.image.index.v1+json"
	MediaTypeDockerManifest     = "application/vnd.docker.distribution.manifest.v2+json"
	MediaTypeDockerManifestList = "application/vnd.docker.distribution.manifest.list.v2+json"
	MediaTypeOciEmpty           = "application/vnd.oci.empty.v1+json"
)

const (
	headerDockerContentDigest = "Docker-Content-Digest"
	headerOciSubject          = "OCI-Subject"
	sha256DigestPrefix        = "sha256:"
	octetStreamContentType    = "application/octet-stream"

	manifestAcceptForAnyImageKind = MediaTypeOciManifest + ", " + MediaTypeOciIndex + ", " +
		MediaTypeDockerManifest + ", " + MediaTypeDockerManifestList
)

// Digest is the content address the registry stores data under.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return sha256DigestPrefix + hex.EncodeToString(sum[:])
}

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
		return "", nil, altshiftErrors.NewWithTrace(empty_error.New("name"))
	}
	if reference == "" {
		return "", nil, altshiftErrors.NewWithTrace(empty_error.New("reference"))
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

	response, m, err := altshiftHttpUtils.FetchJson[*manifest.Manifest](ctx, urlString, options...)
	if err != nil {
		return "", nil, altshiftErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
	}
	if m == nil {
		return "", nil, altshiftErrors.NewWithTrace(nil_error.New("manifest"))
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
		return nil, altshiftErrors.NewWithTrace(empty_error.New("name"))
	}
	if digest == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("digest"))
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
	_, idx, err := altshiftHttpUtils.FetchJson[*index.Index](ctx, urlString, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
	}

	if idx == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("idx"))
	}

	return idx, nil
}

// GetBlob downloads a blob by digest and returns the raw bytes.
func (c *Client) GetBlob(ctx context.Context, name string, digest string, options ...fetch_config.Option) ([]byte, error) {
	if name == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("name"))
	}
	if digest == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("digest"))
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	u := *c.baseUrl
	u.Path += name + "/blobs/" + digest
	urlString := u.String()

	options = append(c.config.FetchOptions, options...)
	_, responseBody, err := altshiftHttpUtils.Fetch(ctx, urlString, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch: %w", err), urlString)
	}

	return responseBody, nil
}

// HeadManifest describes what a tag or digest names without downloading it: the
// manifest's digest, its media type and its size, which together are what a
// descriptor pointing at it is made of. A referrer's subject is such a
// descriptor, and all three are checked by a registry indexing it.
//
// Every kind of image manifest is accepted, because the media type is the
// registry's to state rather than the caller's to guess: an image pushed by
// Docker has a Docker manifest, and one asked for as anything else may be
// refused as unknown.
func (c *Client) HeadManifest(ctx context.Context, name string, reference string, options ...fetch_config.Option) (*descriptor.Descriptor, error) {
	if name == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("name"))
	}
	if reference == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("reference"))
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	u := *c.baseUrl
	u.Path += name + "/manifests/" + reference
	urlString := u.String()

	options = append(
		append(c.config.FetchOptions, options...),
		fetch_config.WithMethod(http.MethodHead),
		fetch_config.WithHeaders(map[string]string{"Accept": manifestAcceptForAnyImageKind}),
	)

	response, _, err := altshiftHttpUtils.Fetch(ctx, urlString, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch: %w", err), urlString)
	}
	if response == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("response"))
	}

	digest := response.Header.Get(headerDockerContentDigest)
	if !strings.HasPrefix(digest, sha256DigestPrefix) {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("manifest digest"), urlString, digest)
	}

	// The type is what the registry stored the manifest as; a parameter on it
	// would make it a different string from the one a descriptor carries.
	mediaType, _, _ := strings.Cut(response.Header.Get("Content-Type"), ";")
	mediaType = strings.TrimSpace(mediaType)
	if mediaType == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("manifest media type"), urlString)
	}

	// A HEAD response carries no body, so the length is the header's: the size of
	// the manifest a GET would have returned.
	if response.ContentLength <= 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("manifest size"), urlString)
	}

	return &descriptor.Descriptor{MediaType: mediaType, Digest: digest, Size: response.ContentLength}, nil
}

// PutBlob uploads data as a blob of the repository and returns its digest.
//
// The upload is the two-request one every registry implements: a POST opens an
// upload session, and a PUT to where the registry said the session is closes it
// with the whole content and its digest. The registry checks the digest against
// what arrived, so a blob is stored under its own content address or not at all.
//
// The session's location must be on the registry's own host. The location is
// the registry's to state, and following it to anywhere else would send the
// caller's credentials there with the content.
func (c *Client) PutBlob(ctx context.Context, name string, data []byte, options ...fetch_config.Option) (string, error) {
	if name == "" {
		return "", altshiftErrors.NewWithTrace(empty_error.New("name"))
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("context err: %w", err)
	}

	digest := Digest(data)

	u := *c.baseUrl
	u.Path += name + "/blobs/uploads/"
	startUrlString := u.String()

	startOptions := append(append(c.config.FetchOptions, options...), fetch_config.WithMethod(http.MethodPost))
	startResponse, _, err := altshiftHttpUtils.Fetch(ctx, startUrlString, startOptions...)
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("fetch (start upload): %w", err), startUrlString)
	}
	if startResponse == nil {
		return "", altshiftErrors.NewWithTrace(nil_error.New("start upload response"))
	}

	location := startResponse.Header.Get("Location")
	if location == "" {
		return "", altshiftErrors.NewWithTrace(empty_error.New("upload location"), startUrlString)
	}

	locationUrl, err := u.Parse(location)
	if err != nil {
		return "", altshiftErrors.NewWithTrace(fmt.Errorf("url parse (upload location): %w", err), location)
	}
	if locationUrl.Scheme != u.Scheme || locationUrl.Host != u.Host {
		return "", altshiftErrors.NewWithTrace(
			mismatch_error.New(
				"upload location host",
				(&url.URL{Scheme: u.Scheme, Host: u.Host}).String(),
				(&url.URL{Scheme: locationUrl.Scheme, Host: locationUrl.Host}).String(),
			),
		)
	}

	// The location may already carry a query -- the session's state, say -- whose
	// values must reach the registry as they were handed out, beside the digest.
	query, err := url.ParseQuery(locationUrl.RawQuery)
	if err != nil {
		return "", altshiftErrors.NewWithTrace(fmt.Errorf("url parse query (upload location): %w", err), location)
	}
	query.Set("digest", digest)
	locationUrl.RawQuery = query.Encode()
	finishUrlString := locationUrl.String()

	finishOptions := append(
		append(c.config.FetchOptions, options...),
		fetch_config.WithMethod(http.MethodPut),
		fetch_config.WithBody(data),
		fetch_config.WithHeaders(map[string]string{"Content-Type": octetStreamContentType}),
	)
	finishResponse, _, err := altshiftHttpUtils.Fetch(ctx, finishUrlString, finishOptions...)
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("fetch (finish upload): %w", err), finishUrlString)
	}
	if finishResponse == nil {
		return "", altshiftErrors.NewWithTrace(nil_error.New("finish upload response"))
	}

	if stored := finishResponse.Header.Get(headerDockerContentDigest); stored != "" && stored != digest {
		return "", altshiftErrors.NewWithTrace(mismatch_error.New("blob digest", digest, stored))
	}

	return digest, nil
}

// PutManifest stores a manifest under a reference -- a tag, or the manifest's
// own digest to store it untagged -- and returns the digest it is stored under
// and the subject the registry indexed it as a referrer of.
//
// The subject is the registry's answer to a manifest that names one: a registry
// with the referrers API states the subject's digest in the OCI-Subject header,
// and one without it states nothing. Empty therefore means either that the
// manifest names no subject or that the registry will not list it as a
// referrer, which a caller attaching an artifact to an image needs to know.
func (c *Client) PutManifest(
	ctx context.Context,
	name string,
	reference string,
	mediaType string,
	data []byte,
	options ...fetch_config.Option,
) (string, string, error) {
	if name == "" {
		return "", "", altshiftErrors.NewWithTrace(empty_error.New("name"))
	}
	if reference == "" {
		return "", "", altshiftErrors.NewWithTrace(empty_error.New("reference"))
	}
	if mediaType == "" {
		return "", "", altshiftErrors.NewWithTrace(empty_error.New("media type"))
	}
	if len(data) == 0 {
		return "", "", altshiftErrors.NewWithTrace(empty_error.New("data"))
	}

	if err := ctx.Err(); err != nil {
		return "", "", fmt.Errorf("context err: %w", err)
	}

	digest := Digest(data)

	u := *c.baseUrl
	u.Path += name + "/manifests/" + reference
	urlString := u.String()

	options = append(
		append(c.config.FetchOptions, options...),
		fetch_config.WithMethod(http.MethodPut),
		fetch_config.WithBody(data),
		fetch_config.WithHeaders(map[string]string{"Content-Type": mediaType}),
	)
	response, _, err := altshiftHttpUtils.Fetch(ctx, urlString, options...)
	if err != nil {
		return "", "", altshiftErrors.New(fmt.Errorf("fetch: %w", err), urlString)
	}
	if response == nil {
		return "", "", altshiftErrors.NewWithTrace(nil_error.New("response"))
	}

	if stored := response.Header.Get(headerDockerContentDigest); stored != "" && stored != digest {
		return "", "", altshiftErrors.NewWithTrace(mismatch_error.New("manifest digest", digest, stored))
	}

	return digest, response.Header.Get(headerOciSubject), nil
}
