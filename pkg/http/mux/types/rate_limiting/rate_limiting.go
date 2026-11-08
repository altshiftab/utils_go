package rate_limiting

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftNet "github.com/altshiftab/utils_go/pkg/net"
)

// TODO: This does not need to be in `mux`?

type RateLimiter struct {
	insertionIndex   int
	Bucket           []*time.Time
	mutex            sync.Mutex
	NumOccupied      int
	NumSecondsExpiry int
}

func (rateLimiter *RateLimiter) Claim() (*time.Time, bool) {
	rateLimiter.mutex.Lock()
	defer rateLimiter.mutex.Unlock()

	if rateLimiter.NumOccupied == len(rateLimiter.Bucket) {
		return rateLimiter.Bucket[rateLimiter.insertionIndex], true
	}

	expirationTime := time.Now().Add(time.Duration(rateLimiter.NumSecondsExpiry) * time.Second)

	currentInsertionIndex := rateLimiter.insertionIndex
	rateLimiter.Bucket[currentInsertionIndex] = &expirationTime
	rateLimiter.insertionIndex = (currentInsertionIndex + 1) % len(rateLimiter.Bucket)
	rateLimiter.NumOccupied += 1

	// NOTE: Arbitrarily decreasing the wait time by one second.
	time.AfterFunc(time.Until(expirationTime)-time.Second, func() {
		rateLimiter.mutex.Lock()
		defer rateLimiter.mutex.Unlock()

		rateLimiter.Bucket[currentInsertionIndex] = nil
		rateLimiter.NumOccupied -= 1
	})

	return &expirationTime, false
}

// occupied is how many of the limiter's slots are claimed.
func (rateLimiter *RateLimiter) occupied() int {
	rateLimiter.mutex.Lock()
	defer rateLimiter.mutex.Unlock()

	return rateLimiter.NumOccupied
}

func DefaultGetRateLimitingKey(request *http.Request) (string, error) {
	if request == nil {
		return "", nil
	}

	remoteAddr := request.RemoteAddr
	ipAddress, _, err := altshiftNet.SplitAddress(remoteAddr)
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("split address: %w", err), remoteAddr)
	}

	return ipAddress, nil
}

// TimerRateLimiter is a key's limiter, and the timer that removes it from its lookup once it has
// been idle for twice its window. The timer is the lookup's to read and write, under its lock.
type TimerRateLimiter struct {
	RateLimiter
	Timer *time.Timer
}

// RateLimitingLookup is the limiters by key. Map, and every limiter's Timer, are read and written
// under Mutex alone; Claim is how a request is counted.
type RateLimitingLookup struct {
	Map   map[string]*TimerRateLimiter
	Mutex sync.Mutex
}

// Claim counts a request of the key against the key's limiter, made with the capacity and the window
// on the key's first request, and says, as RateLimiter.Claim does, when the oldest claim of a full
// limiter expires.
//
// The limiter is found or made, its idle timer reset and the claim made under the lookup's lock, so
// that two requests of a new key make one limiter between them, and a limiter is never removed between
// being found and being claimed from: a claim made on a limiter no longer in the lookup would count
// against nothing.
func (lookup *RateLimitingLookup) Claim(key string, numRequests int, numSecondsExpiration int) (*time.Time, bool) {
	lookup.Mutex.Lock()
	defer lookup.Mutex.Unlock()

	if lookup.Map == nil {
		lookup.Map = make(map[string]*TimerRateLimiter)
	}

	timerRateLimiter, ok := lookup.Map[key]
	if !ok {
		timerRateLimiter = &TimerRateLimiter{
			RateLimiter: RateLimiter{
				Bucket:           make([]*time.Time, numRequests),
				NumSecondsExpiry: numSecondsExpiration,
			},
		}
		lookup.Map[key] = timerRateLimiter
	}

	if timerRateLimiter.Timer != nil {
		timerRateLimiter.Timer.Stop()
	}
	timerRateLimiter.Timer = time.AfterFunc(
		time.Duration(2*timerRateLimiter.NumSecondsExpiry)*time.Second,
		func() { lookup.evict(key, timerRateLimiter) },
	)

	return timerRateLimiter.Claim()
}

// evict removes the key's limiter if it is still this one and holds no claim. A timer that fired as it
// was being stopped, or one of a limiter removed and made anew since, finds another limiter under the
// key, and leaves it.
func (lookup *RateLimitingLookup) evict(key string, timerRateLimiter *TimerRateLimiter) {
	lookup.Mutex.Lock()
	defer lookup.Mutex.Unlock()

	if lookup.Map[key] == timerRateLimiter && timerRateLimiter.occupied() == 0 {
		delete(lookup.Map, key)
	}
}

type RateLimitingConfiguration struct {
	NumRequests          int
	NumSecondsExpiration int
	GetKey               func(*http.Request) (string, error)
	Lookup               RateLimitingLookup
}
