package rate_limiting

import (
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestClaim(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rateLimiter := &RateLimiter{Bucket: make([]*time.Time, 2), NumSecondsExpiry: 5}

		wantExpiry := time.Now().Add(5 * time.Second)

		expiry1, full1 := rateLimiter.Claim()
		expiry2, full2 := rateLimiter.Claim()
		if full1 || full2 {
			t.Fatalf("claims within capacity must not be limited (full1=%v full2=%v)", full1, full2)
		}
		if expiry1 == nil || !expiry1.Equal(wantExpiry) {
			t.Fatalf("expiry1 = %v, want %v", expiry1, wantExpiry)
		}
		if expiry2 == nil || !expiry2.Equal(wantExpiry) {
			t.Fatalf("expiry2 = %v, want %v", expiry2, wantExpiry)
		}

		if _, full := rateLimiter.Claim(); !full {
			t.Fatal("a claim beyond capacity must be limited")
		}

		// Bucket slots are freed one second before expiry; advancing past the window
		// fires those timers.
		time.Sleep(5 * time.Second)
		synctest.Wait()

		if _, full := rateLimiter.Claim(); full {
			t.Fatal("a claim after the window elapsed must not be limited")
		}
	})
}

func TestDefaultGetRateLimitingKey(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		nilRequest bool
		remoteAddr string
		wantKey    string
		wantErr    bool
	}{
		{name: "nil request", nilRequest: true, wantKey: ""},
		{name: "ipv4 with port", remoteAddr: "203.0.113.5:1234", wantKey: "203.0.113.5"},
		{name: "ipv6 with port", remoteAddr: "[2001:db8::1]:443", wantKey: "2001:db8::1"},
		{name: "missing port", remoteAddr: "203.0.113.5", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var request *http.Request
			if !testCase.nilRequest {
				request = &http.Request{RemoteAddr: testCase.remoteAddr}
			}

			key, err := DefaultGetRateLimitingKey(request)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if key != testCase.wantKey {
				t.Fatalf("got %q, want %q", key, testCase.wantKey)
			}
		})
	}
}

// limiterOf is the key's limiter, read under the lookup's lock.
func limiterOf(lookup *RateLimitingLookup, key string) *TimerRateLimiter {
	lookup.Mutex.Lock()
	defer lookup.Mutex.Unlock()

	return lookup.Map[key]
}

func TestRateLimitingLookup_Claim(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		lookup := &RateLimitingLookup{}
		for _, key := range []string{"a", "a", "b"} {
			if _, full := lookup.Claim(key, 2, 5); full {
				t.Fatalf("%s: limited within capacity", key)
			}
		}
		if _, full := lookup.Claim("a", 2, 5); !full {
			t.Fatal("a third claim of a is not limited")
		}
		if _, full := lookup.Claim("b", 2, 5); full {
			t.Fatal("b is limited by a's claims")
		}

		// The claims free a second before their window ends, and an idle limiter goes after twice it.
		time.Sleep(10 * time.Second)
		synctest.Wait()
		if a, b := limiterOf(lookup, "a"), limiterOf(lookup, "b"); a != nil || b != nil {
			t.Errorf("idle limiters kept: %v %v", a, b)
		}
	})
}

// An eviction finds the limiter it was set for or leaves the key alone: one that fired as its timer was
// being reset, or one of a limiter removed and made anew since, must not remove the newer limiter.
func TestRateLimitingLookup_EvictsOnlyItsOwnLimiter(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		lookup := &RateLimitingLookup{}
		lookup.Claim("k", 1, 5)
		first := limiterOf(lookup, "k")
		time.Sleep(4 * time.Second)
		synctest.Wait()

		lookup.evict("k", first)
		if limiterOf(lookup, "k") != nil {
			t.Fatal("an idle limiter was not evicted")
		}
		lookup.Claim("k", 1, 5)
		second := limiterOf(lookup, "k")

		lookup.evict("k", first)
		if limiterOf(lookup, "k") != second {
			t.Error("a stale eviction removed the newer limiter")
		}
		lookup.evict("k", second)
		if limiterOf(lookup, "k") != second {
			t.Error("a limiter holding a claim was evicted")
		}
	})
}

// Evictions run while claims are made and freed, as idle timers fire among requests: the limiter's
// count is read under its own lock.
func TestRateLimitingLookup_EvictionsAmongClaims(t *testing.T) {
	t.Parallel()

	lookup := &RateLimitingLookup{}
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			for range 200 {
				// A one-second window frees its claim at once, from the claim's own timer.
				lookup.Claim("k", 4, 1)
			}
		})
		group.Go(func() {
			for range 200 {
				if limiter := limiterOf(lookup, "k"); limiter != nil {
					lookup.evict("k", limiter)
				}
			}
		})
	}
	group.Wait()
}
