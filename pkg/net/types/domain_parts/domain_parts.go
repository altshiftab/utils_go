package domain_parts

import (
	"strings"

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
