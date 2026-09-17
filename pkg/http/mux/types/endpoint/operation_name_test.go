package endpoint

import (
	"net/http"
	"testing"
)

func TestOperationName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		method   string
		path     string
		expected string
	}{
		{name: "single segment", method: http.MethodGet, path: "/api/orders", expected: "getOrders"},
		{name: "two segments", method: http.MethodDelete, path: "/api/project", expected: "deleteProject"},
		{
			name:     "hyphenated segment",
			method:   http.MethodGet,
			path:     "/api/order/bucket-file",
			expected: "getOrderBucketFile",
		},
		{
			name:     "nested and hyphenated",
			method:   http.MethodPost,
			path:     "/api/order/sign-details",
			expected: "postOrderSignDetails",
		},
		{
			name:     "query method",
			method:   "QUERY",
			path:     "/api/orders",
			expected: "queryOrders",
		},
		{
			name:     "lowercase method is already lowered",
			method:   "query",
			path:     "/api/orders",
			expected: "queryOrders",
		},
		{
			name:     "path outside api keeps its segments",
			method:   http.MethodGet,
			path:     "/verification",
			expected: "getVerification",
		},
		{
			// A leading dot is a rune ToUpper leaves alone, so the segment after it keeps the case
			// it had. The name is the poorer for it, and is what the generated clients have always
			// called this operation; Hint.OperationId is the way to say something better.
			name:     "dots are dropped, and the segment holding one is not cased",
			method:   http.MethodGet,
			path:     "/.well-known/jwks.json",
			expected: "getwellKnownJwksjson",
		},
		{
			name:     "root path",
			method:   http.MethodGet,
			path:     "/",
			expected: "get",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if actual := OperationName(testCase.method, testCase.path); actual != testCase.expected {
				t.Errorf("%s: expected %q, got %q", testCase.name, testCase.expected, actual)
			}
		})
	}
}
