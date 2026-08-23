package domain_parts

import (
	"net/netip"
	"strings"

	altshiftNet "github.com/altshiftab/utils_go/pkg/net"
	"github.com/altshiftab/utils_go/pkg/net/publicsuffix"
)

type Parts struct {
	RegisteredDomain string `json:"registered_domain,omitempty"`
	Subdomain        string `json:"subdomain,omitempty"`
	TopLevelDomain   string `json:"top_level_domain,omitempty"`
}

func New(domainString string) *Parts {
	if domainString == "" {
		return nil
	}

	etld, icann := publicsuffix.PublicSuffix(domainString)
	if !icann && strings.IndexByte(etld, '.') == -1 {
		return nil
	}

	registeredDomain, err := publicsuffix.EffectiveTLDPlusOne(domainString)
	if err != nil {
		return nil
	}

	breakdown := Parts{
		TopLevelDomain:   etld,
		RegisteredDomain: registeredDomain,
	}

	if subdomain := strings.TrimSuffix(domainString, "."+registeredDomain); subdomain != domainString {
		breakdown.Subdomain = subdomain
	}

	return &breakdown
}

// NewAllowingLoopback is New, extended to the identifiers that name the loopback
// host rather than a registered one: the names IsLocalhost recognises, and the
// loopback addresses themselves -- 127.0.0.0/8 and ::1, however they are
// written. All of them answer with Localhost as the registered domain, that host
// being what they have in common and the only name it goes by. An address
// has no top-level domain and no subdomain to report, and reports neither.
//
// It is kept apart from New because most callers want New's answer. A mail
// domain or a schema-validated host that resolves to loopback is not something
// anyone registered, and describing it as though it were would let
// "user@localhost" pass for an address. Reach for this one only where a local
// host is a legitimate thing to be talking to: deciding whether an origin
// belongs to the same deployment as a service that is itself running locally,
// say. Somewhere the answer decides what may be *reached* rather than what may
// be talked to, it is the wrong question to ask -- resolving a loopback address
// there is how a request meant for the outside world ends up aimed back inside.
func NewAllowingLoopback(domainString string) *Parts {
	if parts := New(domainString); parts != nil {
		return parts
	}

	if domainString == "" {
		return nil
	}

	// An address is checked before the name, since no address is a name: a
	// hostname that parses as one is one, and what it resolves to is not a
	// lookup away.
	if address, err := netip.ParseAddr(domainString); err == nil {
		if !address.IsLoopback() {
			return nil
		}

		return &Parts{RegisteredDomain: altshiftNet.Localhost}
	}

	// The root label is what makes a name fully qualified rather than part of the
	// name, and RFC 6761 writes the reserved one with it.
	lowered := strings.ToLower(strings.TrimSuffix(domainString, "."))
	if !altshiftNet.IsLocalhost(lowered) {
		return nil
	}

	parts := Parts{TopLevelDomain: altshiftNet.Localhost, RegisteredDomain: altshiftNet.Localhost}
	if subdomain := strings.TrimSuffix(lowered, "."+altshiftNet.Localhost); subdomain != lowered {
		parts.Subdomain = subdomain
	}

	return &parts
}
