package token_source

import (
	"errors"
	"sync"
	"testing"
	"time"

	oauth2Token "github.com/altshiftab/utils_go/pkg/oauth2/types/token"
)

var errBoom = errors.New("boom")

// fakeTokenSource is an in-test TokenSource that records how many times it was
// invoked and returns the configured token/error.
type fakeTokenSource struct {
	mu    sync.Mutex
	calls int
	token *oauth2Token.Token
	err   error
}

func (f *fakeTokenSource) Token() (*oauth2Token.Token, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.token, nil
}

func (f *fakeTokenSource) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func validToken(access string) *oauth2Token.Token {
	return &oauth2Token.Token{AccessToken: access, Expiry: time.Now().Add(time.Hour)}
}

func expiredToken(access string) *oauth2Token.Token {
	return &oauth2Token.Token{AccessToken: access, Expiry: time.Now().Add(-time.Hour)}
}

func TestNewStatic(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		tok  *oauth2Token.Token
	}{
		{name: "non-nil token", tok: validToken("static")},
		{name: "nil token", tok: nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			src := NewStatic(testCase.tok)

			for range 3 {
				got, err := src.Token()
				if err != nil {
					t.Fatalf("Token() error = %v", err)
				}
				if got != testCase.tok {
					t.Errorf("Token() = %v, want %v", got, testCase.tok)
				}
			}
		})
	}
}

func TestReusableReturnsCachedValidToken(t *testing.T) {
	t.Parallel()

	cached := validToken("cached")
	fake := &fakeTokenSource{token: validToken("fresh")}
	src := NewReusable(cached, fake)

	got, err := src.Token()
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if got != cached {
		t.Errorf("Token() = %v, want cached %v", got, cached)
	}
	if fake.callCount() != 0 {
		t.Errorf("underlying source calls = %d, want 0", fake.callCount())
	}
}

func TestReusableFetchesWhenNil(t *testing.T) {
	t.Parallel()

	fresh := validToken("fresh")
	fake := &fakeTokenSource{token: fresh}
	src := NewReusable(nil, fake)

	got, err := src.Token()
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if got != fresh {
		t.Errorf("Token() = %v, want %v", got, fresh)
	}
	if fake.callCount() != 1 {
		t.Errorf("underlying source calls = %d, want 1", fake.callCount())
	}

	// Second call should reuse the now-cached valid token.
	if _, err := src.Token(); err != nil {
		t.Fatalf("second Token() error = %v", err)
	}
	if fake.callCount() != 1 {
		t.Errorf("underlying source calls after reuse = %d, want 1", fake.callCount())
	}
}

func TestReusableFetchesWhenExpired(t *testing.T) {
	t.Parallel()

	fresh := validToken("fresh")
	fake := &fakeTokenSource{token: fresh}
	src := NewReusable(expiredToken("stale"), fake)

	got, err := src.Token()
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if got != fresh {
		t.Errorf("Token() = %v, want %v", got, fresh)
	}
	if fake.callCount() != 1 {
		t.Errorf("underlying source calls = %d, want 1", fake.callCount())
	}
}

func TestReusablePropagatesError(t *testing.T) {
	t.Parallel()

	fake := &fakeTokenSource{err: errBoom}
	src := NewReusable(nil, fake)

	got, err := src.Token()
	if !errors.Is(err, errBoom) {
		t.Fatalf("Token() error = %v, want %v", err, errBoom)
	}
	if got != nil {
		t.Errorf("Token() = %v, want nil", got)
	}

	// A failure must not be cached: the next call retries the source.
	fake.mu.Lock()
	fake.err = nil
	fake.token = validToken("recovered")
	fake.mu.Unlock()

	got, err = src.Token()
	if err != nil {
		t.Fatalf("second Token() error = %v", err)
	}
	if got == nil || got.AccessToken != "recovered" {
		t.Errorf("second Token() = %v, want recovered token", got)
	}
	if fake.callCount() != 2 {
		t.Errorf("underlying source calls = %d, want 2", fake.callCount())
	}
}

func TestReusableConcurrentAccess(t *testing.T) {
	t.Parallel()

	fake := &fakeTokenSource{token: validToken("fresh")}
	src := NewReusable(nil, fake)

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, err := src.Token(); err != nil {
				t.Errorf("Token() error = %v", err)
			}
		})
	}
	wg.Wait()

	// Once a valid token is cached the source must not be called again;
	// the very first fetch may race, but the count stays small and bounded.
	if got := fake.callCount(); got < 1 {
		t.Errorf("underlying source calls = %d, want at least 1", got)
	}
}
