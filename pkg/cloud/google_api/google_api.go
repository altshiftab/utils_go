// Package google_api reads what a Google API says when it refuses a request,
// and offers the retry policy that follows from it.
//
// The envelope here is the one every Google API answers an error in, rather
// than any one service's, which is why it sits above them rather than in one of
// them. (google_ai/gemini's RetryAfterFromResponse reads the same envelope for
// the delay it advises, and would sit here as well.)
package google_api

import (
	"encoding/json/v2"
	"net/http"

	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config/retry_config/response_checker"
)

// The reasons a Google API refuses with 403 for having been asked too often,
// rather than for the caller not being allowed. They come back in a second, or
// tomorrow; a refusal for permission does not come back at all.
var rateLimitReasons = map[string]struct{}{
	"rateLimitExceeded":     {},
	"userRateLimitExceeded": {},
	"quotaExceeded":         {},
	"dailyLimitExceeded":    {},
}

// errorEnvelope is the shape a Google API answers an error in.
type errorEnvelope struct {
	Error *apiError `json:"error"`
}

type apiError struct {
	Errors []*apiErrorItem `json:"errors"`
	Status string          `json:"status"`
}

type apiErrorItem struct {
	Reason string `json:"reason"`
}

// IsRateLimited says whether the refusal was for the request having been made
// too often. Google reports that as 429, or -- and this is the case a status
// code alone cannot tell apart from a refusal for want of permission -- as 403
// with a reason in the body saying so.
func IsRateLimited(response *http.Response, responseBody []byte) bool {
	if response == nil {
		return false
	}

	switch response.StatusCode {
	case http.StatusTooManyRequests:
		return true
	case http.StatusForbidden:
		return hasRateLimitReason(responseBody)
	default:
		return false
	}
}

func hasRateLimitReason(responseBody []byte) bool {
	if len(responseBody) == 0 {
		return false
	}

	var envelope errorEnvelope
	if err := json.Unmarshal(responseBody, &envelope); err != nil || envelope.Error == nil {
		return false
	}

	if envelope.Error.Status == "RESOURCE_EXHAUSTED" {
		return true
	}

	for _, item := range envelope.Error.Errors {
		if item == nil {
			continue
		}
		if _, ok := rateLimitReasons[item.Reason]; ok {
			return true
		}
	}

	return false
}

// RetryResponseChecker is retry_config.DefaultResponseChecker plus the refusal
// only the body reveals. Wire it into a fetch's retry configuration when
// talking to a Google API; without it, a caller refused for asking too often
// gives up as though it had been refused outright.
var RetryResponseChecker = response_checker.New(
	func(response *http.Response, responseBody []byte, err error) bool {
		if response == nil {
			// Nothing came back at all -- a refused connection, a timeout --
			// which is the network's to have caused and may not happen again.
			return err != nil
		}

		return response.StatusCode >= http.StatusInternalServerError ||
			IsRateLimited(response, responseBody)
	},
)
