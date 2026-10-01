package entra_config

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.AuthorityUrl != nil {
		t.Errorf("default AuthorityUrl = %v, want nil", defaults.AuthorityUrl)
	}
	if len(defaults.FetchOptions) != 0 {
		t.Errorf("default FetchOptions len = %d, want 0", len(defaults.FetchOptions))
	}

	authorityUrl := &url.URL{Scheme: "https", Host: "login.microsoftonline.us"}
	config := New(
		WithAuthorityUrl(authorityUrl),
		WithFetchOptions(fetch_config.WithMethod(http.MethodPost)),
	)
	if config.AuthorityUrl != authorityUrl {
		t.Errorf("AuthorityUrl = %v, want %v", config.AuthorityUrl, authorityUrl)
	}
	if applied := fetch_config.New(config.FetchOptions...); applied.Method != http.MethodPost {
		t.Errorf("applied fetch Method = %q, want %q", applied.Method, http.MethodPost)
	}
}
