package response_writer

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	contentSecurityPolicy "github.com/altshiftab/utils_go/pkg/http/types/content_security_policy"
)

const contentSecurityPolicyHeaderName = "Content-Security-Policy"

var ErrInvalidInlineScriptHash = fmt.Errorf("invalid inline script hash")

// applyInlineScriptHashesCache caches merged policies keyed by the policy
// string and hash list, as the combinations are few and stable.
var applyInlineScriptHashesCache sync.Map

// applyInlineScriptHashes merges Content Security Policy hash sources (e.g.
// "sha256-<base64>") into the script-src directive of the provided policy. A
// missing script-src directive is seeded from default-src, which it stops
// inheriting from. A policy restricting neither is returned unchanged, as
// inline scripts are already permitted.
func applyInlineScriptHashes(policyString string, inlineScriptHashes []string) (string, error) {
	cacheKey := policyString + "\x00" + strings.Join(inlineScriptHashes, "\x00")
	if cached, ok := applyInlineScriptHashesCache.Load(cacheKey); ok {
		return cached.(string), nil
	}

	policy, err := contentSecurityPolicy.Parse([]byte(policyString))
	if err != nil {
		return "", altshiftErrors.New(
			fmt.Errorf("content security policy parse: %w", err),
			policyString,
		)
	}
	if policy == nil {
		return "", altshiftErrors.NewWithTrace(nil_error.New("content security policy"))
	}

	scriptSrc := policy.GetScriptSrc()
	if scriptSrc == nil {
		defaultSrc := policy.GetDefaultSrc()
		if defaultSrc == nil {
			applyInlineScriptHashesCache.Store(cacheKey, policyString)
			return policyString, nil
		}

		scriptSrc = &contentSecurityPolicy.ScriptSrcDirective{
			SourceDirective: contentSecurityPolicy.SourceDirective{
				Sources: slices.Clone(defaultSrc.Sources),
			},
		}
		policy.Directives = append(policy.Directives, scriptSrc)
	}

	presentSources := make(map[string]struct{})
	for _, source := range scriptSrc.Sources {
		if source == nil {
			continue
		}
		presentSources[source.String()] = struct{}{}
	}

	for _, inlineScriptHash := range inlineScriptHashes {
		hashAlgorithm, base64Value, found := strings.Cut(inlineScriptHash, "-")
		if !found || hashAlgorithm == "" || base64Value == "" {
			return "", altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %s", ErrInvalidInlineScriptHash, inlineScriptHash),
				inlineScriptHash,
			)
		}

		hashSource := &contentSecurityPolicy.HashSource{
			HashAlgorithm: hashAlgorithm,
			Base64Value:   base64Value,
		}
		if _, ok := presentSources[hashSource.String()]; ok {
			continue
		}
		scriptSrc.Sources = append(scriptSrc.Sources, hashSource)
	}

	mergedPolicyString := policy.String()
	applyInlineScriptHashesCache.Store(cacheKey, mergedPolicyString)
	return mergedPolicyString, nil
}
