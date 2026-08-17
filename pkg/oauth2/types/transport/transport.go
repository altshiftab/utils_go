package transport

import (
	"fmt"
	"net/http"

	"github.com/altshiftab/utils_go/pkg/oauth2/types/token_source"
)

// Transport is an http.RoundTripper that makes OAuth2-authenticated requests.
// It obtains a token from Source before each request and sets the Authorization header.
type Transport struct {
	// Source supplies tokens for authenticating requests.
	Source token_source.TokenSource

	// Base is the underlying RoundTripper. If nil, http.DefaultTransport is used.
	Base http.RoundTripper
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	tok, err := t.Source.Token()
	if err != nil {
		return nil, fmt.Errorf("token: %w", err)
	}

	req2 := req.Clone(req.Context())
	tok.SetAuthHeader(req2)

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	return base.RoundTrip(req2)
}
