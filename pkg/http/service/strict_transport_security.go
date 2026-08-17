package service

import (
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	motmedelMux "github.com/altshiftab/utils_go/pkg/http/mux"
)

const strictTransportSecurityHeaderName = "Strict-Transport-Security"

// strictTransportSecurityValue asks a browser to reach the host over HTTPS only for a year, and its
// subdomains with it. A year is the maximum age the preload list requires of a host that wants to
// be on it; includeSubDomains reaches every name under the service's own, which on a registered
// domain is every name in it -- a subdomain that cannot serve HTTPS becomes unreachable, so a
// service on such a domain answers for what it takes with it.
const strictTransportSecurityValue = "max-age=31536000; includeSubDomains"

func patchStrictTransportSecurity(mux *motmedelMux.Mux) error {
	if mux == nil {
		return motmedelErrors.NewWithTrace(nil_error.New("mux"))
	}

	defaultHeaders := mux.DefaultHeaders
	if defaultHeaders == nil {
		return motmedelErrors.NewWithTrace(nil_error.NewWithInstance("map", "default headers"))
	}

	defaultHeaders[strictTransportSecurityHeaderName] = strictTransportSecurityValue

	return nil
}
