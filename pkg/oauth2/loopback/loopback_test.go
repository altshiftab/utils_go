package loopback

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/auth_code_option"
	oauth2Config "github.com/altshiftab/utils_go/pkg/oauth2/types/config"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/endpoint"
)

func TestChallenge(t *testing.T) {
	t.Parallel()

	// The example of RFC 7636, appendix B.
	if got := Challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Errorf("challenge = %s", got)
	}
}

// browse is what a browser does with the authorization URL: what the authorization server would redirect it to,
// requested of the loopback listener.
type browse func(t *testing.T, authorization *url.URL, redirectUri string, state string)

func get(t *testing.T, target string) {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Errorf("new request %s: %v", target, err)
		return
	}
	response, err := http.DefaultClient.Do(request) //nolint:gosec // A test's request to its own listener.
	if err != nil || response == nil {
		t.Errorf("get %s: %v", target, err)
		return
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
}

func TestAuthorize(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		browse  browse
		wantErr error
		// wantTimeout says the flow is to wait on, the redirect it was sent being none it takes.
		wantTimeout bool
	}{
		{
			name: "authorized, after a favicon",
			browse: func(t *testing.T, _ *url.URL, redirectUri string, state string) {
				get(t, redirectUri+"favicon.ico")
				get(t, redirectUri+"?"+url.Values{"state": {state}, "code": {"the-code"}}.Encode())
			},
		},
		{
			name: "denied",
			browse: func(t *testing.T, _ *url.URL, redirectUri string, state string) {
				get(t, redirectUri+"?"+url.Values{"state": {state}, "error": {"access_denied"}}.Encode())
			},
			wantErr: ErrDenied,
		},
		{
			name: "another state",
			browse: func(t *testing.T, _ *url.URL, redirectUri string, _ string) {
				get(t, redirectUri+"?"+url.Values{"state": {"forged"}, "code": {"the-code"}}.Encode())
			},
			wantTimeout: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var challenge string
			tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					t.Errorf("parse form: %v", err)
				}
				if r.PostForm.Get("code") != "the-code" {
					t.Errorf("code = %q", r.PostForm.Get("code"))
				}
				if Challenge(r.PostForm.Get("code_verifier")) != challenge {
					t.Errorf("the verifier %q is not the challenge's", r.PostForm.Get("code_verifier"))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"access_token":"at","refresh_token":"rt","token_type":"Bearer","expires_in":3600}`)
			}))
			t.Cleanup(tokenServer.Close)

			config := &oauth2Config.Config{
				ClientID:     "client",
				Endpoint:     endpoint.Endpoint{AuthURL: "https://accounts.example.com/auth", TokenURL: tokenServer.URL, AuthStyle: endpoint.AuthStyleInParams},
				FetchOptions: []fetch_config.Option{fetch_config.WithHttpClient(tokenServer.Client())},
				RedirectURL:  "https://ignored.example.com/",
				Scopes:       []string{"scope-a"},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			// The browser is done before the test is, whatever the flow made of what it sent.
			var browsing sync.WaitGroup
			defer browsing.Wait()

			openUrl := func(target string) error {
				authorization, err := url.Parse(target)
				if err != nil {
					return err
				}
				query := authorization.Query()
				challenge = query.Get("code_challenge")
				if query.Get("code_challenge_method") != "S256" || challenge == "" {
					t.Errorf("%s: no S256 challenge in %s", testCase.name, target)
				}
				if query.Get("access_type") != "offline" {
					t.Errorf("%s: the options are not in %s", testCase.name, target)
				}
				redirectUri := query.Get("redirect_uri")
				if parsed, err := url.Parse(redirectUri); err != nil || parsed.Hostname() != "127.0.0.1" {
					t.Errorf("%s: redirect uri %q is not a loopback address", testCase.name, redirectUri)
				}
				browsing.Go(func() { testCase.browse(t, authorization, redirectUri, query.Get("state")) })
				return nil
			}

			got, err := Authorize(ctx, config, openUrl, auth_code_option.New("access_type", "offline"))
			switch {
			case testCase.wantTimeout:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("%s: err = %v, want the deadline", testCase.name, err)
				}
			case testCase.wantErr != nil:
				if !errors.Is(err, testCase.wantErr) {
					t.Errorf("%s: err = %v, want %v", testCase.name, err, testCase.wantErr)
				}
			default:
				if err != nil {
					t.Fatalf("%s: unexpected error: %v", testCase.name, err)
				}
				if got.AccessToken != "at" || got.RefreshToken != "rt" {
					t.Errorf("%s: token = %+v", testCase.name, got)
				}
			}
		})
	}
}

func TestAuthorizeArgumentErrors(t *testing.T) {
	t.Parallel()

	open := func(string) error { return nil }
	if _, err := Authorize(context.Background(), nil, open); err == nil {
		t.Error("expected an error for a nil config")
	}
	if _, err := Authorize(context.Background(), &oauth2Config.Config{}, nil); err == nil {
		t.Error("expected an error for a nil open url")
	}
}
