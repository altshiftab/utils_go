// Package loopback authorizes an installed app in the user's browser: it listens on a loopback address for the
// redirect that carries the authorization code, and exchanges the code for a token.
//
// It is the flow RFC 8252 describes for native apps, with PKCE (RFC 7636): the code is bound to a verifier only this
// process holds, so another program on the machine that catches the redirect cannot spend it.
package loopback

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/auth_code_option"
	oauth2Config "github.com/altshiftab/utils_go/pkg/oauth2/types/config"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token"
)

// ErrDenied is the user, or the authorization server, refusing the authorization.
var ErrDenied = errors.New("authorization denied")

// readHeaderTimeout bounds how long the listener waits for a request's headers; the browser's redirect arrives at
// once.
const readHeaderTimeout = 10 * time.Second

// shutdownTimeout bounds how long the listener is given to finish answering the redirect, so that the browser shows
// the page rather than a reset connection.
const shutdownTimeout = 5 * time.Second

// closedPage is what the browser shows once the redirect has been caught.
const closedPage = "Authorization received. You may close this window."

func randomString(numBytes int) (string, error) {
	buffer := make([]byte, numBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", altshiftErrors.NewWithTrace(fmt.Errorf("rand read: %w", err))
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// Challenge is the S256 code challenge of a PKCE verifier.
func Challenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

type redirect struct {
	code string
	err  error
}

// handler catches the one redirect that carries the state, and answers anything else -- the favicon a browser asks
// for -- with a 404 that ends nothing.
func handler(state string, redirects chan<- *redirect) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if request.URL.Path != "/" || !query.Has("state") {
			http.NotFound(responseWriter, request)
			return
		}
		if subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(state)) != 1 {
			http.Error(responseWriter, "The state does not match.", http.StatusBadRequest)
			return
		}

		var caught *redirect
		switch {
		case query.Get("error") != "":
			caught = &redirect{err: altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %s", ErrDenied, query.Get("error")),
				query.Get("error_description"),
			)}
			http.Error(responseWriter, "The authorization was refused.", http.StatusForbidden)
		case query.Get("code") == "":
			caught = &redirect{err: altshiftErrors.NewWithTrace(empty_error.New("code"))}
			http.Error(responseWriter, "The redirect carries no code.", http.StatusBadRequest)
		default:
			caught = &redirect{code: query.Get("code")}
			_, _ = fmt.Fprintln(responseWriter, closedPage)
		}

		select {
		case redirects <- caught:
		default:
		}
	})
}

// Authorize has the user authorize the config's client in the browser and returns the token the code is exchanged
// for. openUrl is given the authorization URL to send the user to: it may open a browser, print the URL, or both.
//
// The redirect is to 127.0.0.1 on a port chosen now, which the config's own RedirectURL is replaced by; an installed
// app's client takes any port of a loopback address. The options are added to the authorization URL, as
// access_type=offline for a refresh token or login_hint for the account to sign in as.
func Authorize(
	ctx context.Context,
	config *oauth2Config.Config,
	openUrl func(string) error,
	options ...auth_code_option.AuthCodeOption,
) (*token.Token, error) {
	if config == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("config"))
	}
	if openUrl == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("open url"))
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("listen config listen: %w", err))
	}
	defer listener.Close()

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || tcpAddr == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("listener tcp address"))
	}

	configCopy := *config
	configCopy.RedirectURL = new(url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(tcpAddr.Port)), Path: "/"}).String()

	state, err := randomString(16)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("random string (state): %w", err))
	}
	verifier, err := randomString(32)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("random string (verifier): %w", err))
	}

	redirects := make(chan *redirect, 1)
	server := &http.Server{Handler: handler(state, redirects), ReadHeaderTimeout: readHeaderTimeout}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	serveErrors := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrors <- altshiftErrors.NewWithTrace(fmt.Errorf("http server serve: %w", err))
		}
	}()

	authorizationOptions := append(
		[]auth_code_option.AuthCodeOption{
			auth_code_option.New("code_challenge", Challenge(verifier)),
			auth_code_option.New("code_challenge_method", "S256"),
		},
		options...,
	)
	if err := openUrl(configCopy.AuthCodeURL(state, authorizationOptions...)); err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("open url: %w", err))
	}

	var code string
	select {
	case caught := <-redirects:
		if caught.err != nil {
			return nil, caught.err
		}
		code = caught.code
	case err := <-serveErrors:
		return nil, err
	case <-ctx.Done():
		return nil, fmt.Errorf("context done (waiting for the redirect): %w", ctx.Err())
	}

	exchanged, err := configCopy.Exchange(ctx, code, auth_code_option.New("code_verifier", verifier))
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("config exchange: %w", err))
	}
	if exchanged == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("exchanged token"))
	}

	return exchanged, nil
}
