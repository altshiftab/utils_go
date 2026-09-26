package cloud_sql

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/altshiftab/utils_go/pkg/oauth2/types/token"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token_source"
)

func TestUsernameResolver(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		info          *tokenInfo
		accountStatus int
		account       *serviceAccount
		want          string
		wantErr       bool
	}{
		{name: "user token", info: &tokenInfo{Email: "V@Example.com", Azp: "764086051850-x.apps.googleusercontent.com"}, want: "v@example.com"},
		{
			name:    "service account token",
			info:    &tokenInfo{Azp: "116509308811345740388"},
			account: &serviceAccount{Email: "tpm-vph@intil-main.iam.gserviceaccount.com"},
			want:    "tpm-vph@intil-main.iam",
		},
		{name: "neither email nor client", info: &tokenInfo{}, wantErr: true},
		{name: "service account lookup refused", info: &tokenInfo{Azp: "116509308811345740388"}, accountStatus: http.StatusForbidden, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/tokeninfo":
					if r.URL.Query().Get("access_token") != "the-token" {
						http.Error(w, "wrong token", http.StatusBadRequest)
						return
					}
					_ = json.MarshalWrite(w, testCase.info)
				case strings.HasPrefix(r.URL.Path, "/serviceAccounts/"):
					if r.Header.Get("Authorization") != "Bearer the-token" {
						http.Error(w, "unauthenticated", http.StatusUnauthorized)
						return
					}
					if testCase.accountStatus != 0 {
						http.Error(w, "denied", testCase.accountStatus)
						return
					}
					if strings.TrimPrefix(r.URL.Path, "/serviceAccounts/") != testCase.info.Azp {
						http.NotFound(w, r)
						return
					}
					_ = json.MarshalWrite(w, testCase.account)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			tokenSource := token_source.NewStatic(&token.Token{AccessToken: "the-token", Expiry: time.Now().Add(time.Hour)})
			resolver := NewUsernameResolver()
			resolver.TokenInfoUrl = server.URL + "/tokeninfo"
			resolver.ServiceAccountUrl = server.URL + "/serviceAccounts/"

			got, err := resolver.Resolve(context.Background(), tokenSource)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("resolve: %q, %v", got, err)
			}
			if got != testCase.want {
				t.Errorf("resolve = %q, want %q", got, testCase.want)
			}
		})
	}
}
