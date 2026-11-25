package mux

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	muxTypesRateLimiting "github.com/altshiftab/utils_go/pkg/http/mux/types/rate_limiting"
)

func TestHandleRateLimiting(t *testing.T) {
	t.Parallel()

	t.Run("nil config allows", func(t *testing.T) {
		t.Parallel()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		if responseError := HandleRateLimiting(nil, request, ""); responseError != nil {
			t.Fatalf("expected nil, got %#v", responseError)
		}
	})

	t.Run("nil request is a server error", func(t *testing.T) {
		t.Parallel()
		config := &muxTypesRateLimiting.RateLimitingConfiguration{NumRequests: 1, NumSecondsExpiration: 5}
		if responseError := HandleRateLimiting(config, nil, ""); responseError == nil || responseError.ServerError == nil {
			t.Fatalf("expected a server error, got %#v", responseError)
		}
	})

	t.Run("limits after capacity", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			config := &muxTypesRateLimiting.RateLimitingConfiguration{NumRequests: 2, NumSecondsExpiration: 5}
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			request.RemoteAddr = "203.0.113.9:5000"

			for i := range 2 {
				if responseError := HandleRateLimiting(config, request, ""); responseError != nil {
					t.Fatalf("request %d should be allowed, got %#v", i, responseError)
				}
			}

			responseError := HandleRateLimiting(config, request, "")
			if responseError == nil || responseError.ProblemDetail == nil {
				t.Fatalf("expected a problem detail, got %#v", responseError)
			}
			if responseError.ProblemDetail.Status != http.StatusTooManyRequests {
				t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, responseError.ProblemDetail.Status)
			}

			var hasRetryAfter bool
			for _, header := range responseError.Headers {
				if header != nil && header.Name == "Retry-After" {
					hasRetryAfter = true
				}
			}
			if !hasRetryAfter {
				t.Error("expected a Retry-After header")
			}
		})
	})
}

// Requests at once, from one address and from several, are each counted under the lookup's lock: the
// race detector finds no unguarded access to the map or to a limiter's timer, and exactly as many
// requests from one address are let through as the limit allows -- two requests making a limiter for
// one key at once would each count against a limiter of their own.
func TestHandleRateLimiting_Concurrent(t *testing.T) {
	t.Parallel()

	const limit, addresses, perAddress = 10, 4, 40
	config := &muxTypesRateLimiting.RateLimitingConfiguration{NumRequests: limit, NumSecondsExpiration: 3600}
	admitted := make([]atomic.Int64, addresses)

	var group sync.WaitGroup
	for index := range addresses * perAddress {
		group.Go(func() {
			address := index % addresses
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			request.RemoteAddr = fmt.Sprintf("203.0.113.%d:5000", address+1)
			if responseError := HandleRateLimiting(config, request, ""); responseError == nil {
				admitted[address].Add(1)
			} else if responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusTooManyRequests {
				t.Errorf("request %d: %#v", index, responseError)
			}
		})
	}
	group.Wait()

	for address := range addresses {
		if got := admitted[address].Load(); got != limit {
			t.Errorf("address %d: %d requests let through, the limit is %d", address, got, limit)
		}
	}
}

// A request is counted under the client the mux resolved, where it resolved one, rather than the
// connection's peer, which behind a proxy is the proxy every client shares; GetKey, where given, wins
// over both.
func TestHandleRateLimiting_Key(t *testing.T) {
	t.Parallel()

	const peer = "198.51.100.1:5000"

	testCases := []struct {
		name string
		// clientAddresses are the addresses the mux resolved for two requests from one peer.
		clientAddresses [2]string
		getKey          func(*http.Request) (string, error)
		wantSecondFull  bool
	}{
		{name: "different clients behind one proxy", clientAddresses: [2]string{"203.0.113.1", "203.0.113.2"}},
		{name: "one client", clientAddresses: [2]string{"203.0.113.1", "203.0.113.1"}, wantSecondFull: true},
		{name: "no client resolved keys on the peer", wantSecondFull: true},
		{
			name:            "get key wins over the client",
			clientAddresses: [2]string{"203.0.113.1", "203.0.113.2"},
			getKey:          func(*http.Request) (string, error) { return "everyone", nil },
			wantSecondFull:  true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				config := &muxTypesRateLimiting.RateLimitingConfiguration{
					NumRequests:          1,
					NumSecondsExpiration: 5,
					GetKey:               testCase.getKey,
				}

				var full [2]bool
				for index, clientAddress := range testCase.clientAddresses {
					request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
					request.RemoteAddr = peer
					responseError := HandleRateLimiting(config, request, clientAddress)
					if responseError != nil && (responseError.ProblemDetail == nil || responseError.ProblemDetail.Status != http.StatusTooManyRequests) {
						t.Fatalf("request %d: %#v", index, responseError)
					}
					full[index] = responseError != nil
				}

				if full[0] {
					t.Fatal("the first request was limited")
				}
				if full[1] != testCase.wantSecondFull {
					t.Fatalf("second request limited: %v, want %v", full[1], testCase.wantSecondFull)
				}
			})
		})
	}
}
