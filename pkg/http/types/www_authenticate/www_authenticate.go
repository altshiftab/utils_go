// Package www_authenticate renders the challenges a 401 carries in WWW-Authenticate (RFC 9110
// §11.6.1), and the Bearer ones RFC 6750 §3 defines for a request with a token that was refused.
package www_authenticate

import "strings"

const HeaderName = "WWW-Authenticate"

// The RFC 6750 §3.1 error codes. A request that carried no token at all gets none.
const (
	ErrorInvalidRequest    = "invalid_request"
	ErrorInvalidToken      = "invalid_token"
	ErrorInsufficientScope = "insufficient_scope"
)

// Challenger is implemented by a token extractor that knows the scheme its token arrives under, so
// that a refusal further on can say which challenge to answer with. An empty result means none.
type Challenger interface {
	Challenge(errorCode string, errorDescription string) string
}

// Bearer renders a Bearer challenge. An empty errorCode renders the bare scheme, which is the
// answer to a request with no token (RFC 6750 §3.1); a description is included only with a code.
func Bearer(errorCode string, errorDescription string) string {
	if errorCode == "" {
		return "Bearer"
	}

	var builder strings.Builder
	builder.WriteString(`Bearer error="`)
	builder.WriteString(restrict(errorCode))
	builder.WriteString(`"`)
	if description := restrict(errorDescription); description != "" {
		builder.WriteString(`, error_description="`)
		builder.WriteString(description)
		builder.WriteString(`"`)
	}

	return builder.String()
}

// restrict keeps the characters RFC 6750 allows in error and error_description, printable ASCII
// less the quote and the backslash, which cannot be escaped there.
func restrict(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return -1
		}
		return r
	}, value)
}
