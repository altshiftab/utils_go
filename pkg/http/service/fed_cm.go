package service

import (
	"fmt"
	"net/url"
	"strings"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	motmedelMux "github.com/altshiftab/utils_go/pkg/http/mux"
	csp "github.com/altshiftab/utils_go/pkg/http/types/content_security_policy"
	cspUtils "github.com/altshiftab/utils_go/pkg/http/utils/content_security_policy"
)

const permissionsPolicyHeaderName = "Permissions-Policy"

// patchFedCm lets the documents ask the identity providers who the user is, through the browser's
// federated credential management: the providers are permitted as connect-src, which is what the
// browser fetches their configuration and accounts over, and identity-credentials-get is permitted
// for each of them, without which the browser refuses the call whatever the policy says.
//
// The providers are whichever the service federates to; nothing here is particular to any of them.
func patchFedCm(mux *motmedelMux.Mux, providerUrls ...*url.URL) error {
	if len(providerUrls) == 0 {
		return nil
	}

	if mux == nil {
		return motmedelErrors.NewWithTrace(nil_error.New("mux"))
	}

	defaultDocumentHeaders := mux.DefaultDocumentHeaders
	if defaultDocumentHeaders == nil {
		return motmedelErrors.NewWithTrace(nil_error.NewWithInstance("map", "default document headers"))
	}

	var permissionsPolicyEntries []string
	for _, providerUrl := range providerUrls {
		if providerUrl == nil {
			continue
		}

		permissionsPolicyEntries = append(
			permissionsPolicyEntries,
			fmt.Sprintf("identity-credentials-get=(self %q)", providerUrl.String()),
		)
	}

	if len(permissionsPolicyEntries) == 0 {
		return nil
	}

	err := patchContentSecurityPolicy(
		mux,
		func(contentSecurityPolicy *csp.ContentSecurityPolicy) error {
			cspUtils.PatchCspConnectSrcWithHostSrc(contentSecurityPolicy, providerUrls...)
			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("patch content security policy: %w", err)
	}

	permissionsPolicy := defaultDocumentHeaders[permissionsPolicyHeaderName]
	if permissionsPolicy != "" {
		permissionsPolicy += ", "
	}
	permissionsPolicy += strings.Join(permissionsPolicyEntries, ", ")

	defaultDocumentHeaders[permissionsPolicyHeaderName] = permissionsPolicy

	return nil
}
