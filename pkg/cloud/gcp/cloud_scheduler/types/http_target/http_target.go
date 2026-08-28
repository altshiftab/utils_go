package http_target

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_scheduler/types/oauth_token"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_scheduler/types/oidc_token"
)

// Target is an HTTP request Cloud Scheduler makes on the job's schedule.
type Target struct {
	// Uri is the full URL the request is made to.
	Uri string `json:"uri,omitzero"`
	// HttpMethod defaults to POST where it is left out.
	HttpMethod string            `json:"httpMethod,omitzero"`
	Headers    map[string]string `json:"headers,omitzero"`
	// Body is base64 encoded, and is only sent with a method that takes one.
	Body []byte `json:"body,omitzero"`

	// At most one of these. OidcToken is for a service of one's own; OauthToken is for calling a
	// Google API.
	OidcToken  *oidc_token.Token  `json:"oidcToken,omitzero"`
	OauthToken *oauth_token.Token `json:"oauthToken,omitzero"`
}
