package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	oauth2Token "github.com/altshiftab/utils_go/pkg/oauth2/types/token"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token_source"
)

var errNoToken = errors.New("no token")

// fakeSource is an in-test TokenSource.
type fakeSource struct {
	tok *oauth2Token.Token
	err error
}

func (f *fakeSource) Token() (*oauth2Token.Token, error) {
	return f.tok, f.err
}

// captureRoundTripper records the request it receives and returns a canned response.
type captureRoundTripper struct {
	got *http.Request
}

func (c *captureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.got = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       http.NoBody,
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func newRequest(t *testing.T, target string) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext: %v", err)
	}
	return req
}

func TestRoundTripSetsAuthHeaderWithBase(t *testing.T) {
	t.Parallel()

	capture := &captureRoundTripper{}
	tr := &Transport{
		Source: token_source.NewStatic(&oauth2Token.Token{AccessToken: "abc123", TokenType: "Bearer"}),
		Base:   capture,
	}

	req := newRequest(t, "https://example.test/resource")

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip error = %v", err)
	}
	if resp == nil {
		t.Fatal("RoundTrip response = nil, want non-nil")
	}
	defer resp.Body.Close()

	if capture.got == nil {
		t.Fatal("base round tripper did not receive a request")
	}
	if got := capture.got.Header.Get("Authorization"); got != "Bearer abc123" {
		t.Errorf("forwarded Authorization = %q, want %q", got, "Bearer abc123")
	}

	// The original request must not be mutated (RoundTrip clones it).
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("original request Authorization = %q, want empty", got)
	}
}

func TestRoundTripDefaultTokenType(t *testing.T) {
	t.Parallel()

	capture := &captureRoundTripper{}
	tr := &Transport{
		Source: &fakeSource{tok: &oauth2Token.Token{AccessToken: "xyz"}},
		Base:   capture,
	}

	req := newRequest(t, "https://example.test")

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip error = %v", err)
	}
	if resp == nil {
		t.Fatal("RoundTrip response = nil, want non-nil")
	}
	defer resp.Body.Close()

	if got := capture.got.Header.Get("Authorization"); got != "Bearer xyz" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer xyz")
	}
}

func TestRoundTripTokenError(t *testing.T) {
	t.Parallel()

	tr := &Transport{Source: &fakeSource{err: errNoToken}}

	req := newRequest(t, "https://example.test")

	resp, rtErr := tr.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
		t.Errorf("response = %v, want nil", resp)
	}
	if !errors.Is(rtErr, errNoToken) {
		t.Errorf("error = %v, want to wrap %v", rtErr, errNoToken)
	}
}

func TestRoundTripNilBaseUsesDefaultTransport(t *testing.T) {
	t.Parallel()

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	tr := &Transport{
		Source: token_source.NewStatic(&oauth2Token.Token{AccessToken: "server-tok"}),
		// Base is nil -> http.DefaultTransport.
	}

	req := newRequest(t, server.URL)

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip error = %v", err)
	}
	if resp == nil {
		t.Fatal("RoundTrip response = nil, want non-nil")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if gotAuth != "Bearer server-tok" {
		t.Errorf("server saw Authorization = %q, want %q", gotAuth, "Bearer server-tok")
	}
}
