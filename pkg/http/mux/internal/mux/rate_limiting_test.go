package mux

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"

	muxTypesRateLimiting "github.com/altshiftab/utils_go/pkg/http/mux/types/rate_limiting"
)

func TestHandleRateLimiting(t *testing.T) {
	t.Parallel()

	t.Run("nil config allows", func(t *testing.T) {
		t.Parallel()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		if responseError := HandleRateLimiting(nil, request); responseError != nil {
			t.Fatalf("expected nil, got %#v", responseError)
		}
	})

	t.Run("nil request is a server error", func(t *testing.T) {
		t.Parallel()
		config := &muxTypesRateLimiting.RateLimitingConfiguration{NumRequests: 1, NumSecondsExpiration: 5}
		if responseError := HandleRateLimiting(config, nil); responseError == nil || responseError.ServerError == nil {
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
				if responseError := HandleRateLimiting(config, request); responseError != nil {
					t.Fatalf("request %d should be allowed, got %#v", i, responseError)
				}
			}

			responseError := HandleRateLimiting(config, request)
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
