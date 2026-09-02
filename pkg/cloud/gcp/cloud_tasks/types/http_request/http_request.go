package http_request

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_tasks/types/oauth_token"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_tasks/types/oidc_token"
)

// Request is the HTTP request Cloud Tasks makes when it dispatches the task.
//
// The queue decides how often it is made and how often it is retried; this says only what is sent.
type Request struct {
	// Url is the full URL the request is made to.
	Url string `json:"url,omitzero"`
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
