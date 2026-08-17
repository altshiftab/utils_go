package service

import (
	"fmt"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftMux "github.com/altshiftab/utils_go/pkg/http/mux"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_writer"
	csp "github.com/altshiftab/utils_go/pkg/http/types/content_security_policy"
	cspUtils "github.com/altshiftab/utils_go/pkg/http/utils/content_security_policy"
)

const contentSecurityPolicyHeaderName = "Content-Security-Policy"

// ApiContentSecurityPolicy is what a service that serves no documents answers with. A policy is
// worth as much to it as to one that does -- what differs is which policy.
//
//   - default-src 'none' permits no subresource of any kind, there being no document to load one.
//   - base-uri and form-action do not fall back to default-src, so they are said in full.
//   - frame-ancestors 'none' keeps the response out of a frame.
//   - sandbox, with no value, applies every restriction there is: a response a browser is made to
//     render as a document -- by a content type it sniffed past, or by being navigated to directly
//     -- gets an opaque origin and runs no script.
const ApiContentSecurityPolicy = "default-src 'none'; base-uri 'none'; form-action 'none'; " +
	"frame-ancestors 'none'; sandbox"

// patchContentSecurityPolicy hands the policy a document is answered with to patch, and writes back
// what it made of it. What a policy is worth saying about -- what a viewer may style, what a script
// must go through, what the browser reports -- is worth saying about a document, and a document is
// what carries it.
func patchContentSecurityPolicy(
	mux *altshiftMux.Mux,
	patch func(*csp.ContentSecurityPolicy) error,
) error {
	if mux == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("mux"))
	}

	if patch == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("patch"))
	}

	headers := mux.DefaultDocumentHeaders
	if headers == nil {
		return altshiftErrors.NewWithTrace(nil_error.NewWithInstance("map", "default document headers"))
	}

	contentSecurityPolicyString := headers[contentSecurityPolicyHeaderName]
	if contentSecurityPolicyString == "" {
		contentSecurityPolicyString = response_writer.DefaultContentSecurityPolicyString
	}

	contentSecurityPolicy, err := csp.Parse([]byte(contentSecurityPolicyString))
	if err != nil {
		return altshiftErrors.New(
			fmt.Errorf("content security policy parse: %w", err),
			contentSecurityPolicyString,
		)
	}
	if contentSecurityPolicy == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("content security policy"))
	}

	if err := patch(contentSecurityPolicy); err != nil {
		return fmt.Errorf("patch: %w", err)
	}

	headers[contentSecurityPolicyHeaderName] = contentSecurityPolicy.String()

	return nil
}

// patchViewerStyleHashes permits the styles a browser's own viewer applies to a response it renders
// as a document -- Chrome's for XML, Edge's for PDF. Without them the viewer's styling is blocked
// by the policy, and what it renders comes out unstyled.
//
// This is not a matter for the services that serve documents alone: a service that serves none
// still answers a request it will not serve with a problem detail, which a browser asks for as XML
// and renders through the same viewer. It goes in the policy for documents either way, that being
// what such a response carries -- and what a response that is not a document has no use for.
//
// What the two viewers take differs. Chrome's styles the document tree through style elements,
// whose bodies a hash source matches as it is. Edge's styles through style attributes, which a hash
// source reaches only where 'unsafe-hashes' is permitted with it -- so that is permitted for Edge's
// viewer alone, rather than for every service on the chance that it serves a PDF.
func patchViewerStyleHashes(mux *altshiftMux.Mux, chromeXmlViewer bool, edgePdfViewer bool) error {
	if !chromeXmlViewer && !edgePdfViewer {
		return nil
	}

	err := patchContentSecurityPolicy(
		mux,
		func(contentSecurityPolicy *csp.ContentSecurityPolicy) error {
			// 'self' is permitted first, for the stylesheets a viewer links rather than inlines.
			cspUtils.PatchCspStyleSrcWithKeyword(contentSecurityPolicy, "self")

			if edgePdfViewer {
				cspUtils.PatchCspStyleSrcWithKeyword(contentSecurityPolicy, "unsafe-hashes")
			}

			if chromeXmlViewer {
				err := cspUtils.PatchCspStyleSrcWithHash(
					contentSecurityPolicy,
					cspUtils.ChromeXmlViewerStyleHashes...,
				)
				if err != nil {
					return fmt.Errorf("patch csp style src with hash (chrome xml viewer): %w", err)
				}
			}

			if edgePdfViewer {
				err := cspUtils.PatchCspStyleSrcWithHash(
					contentSecurityPolicy,
					cspUtils.EdgePdfViewerStyleHashes...,
				)
				if err != nil {
					return fmt.Errorf("patch csp style src with hash (edge pdf viewer): %w", err)
				}
			}

			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("patch content security policy: %w", err)
	}

	return nil
}

// patchTrustedTypes requires the named trusted types policies of the scripts the documents run.
func patchTrustedTypes(mux *altshiftMux.Mux, policies ...string) error {
	if len(policies) == 0 {
		return nil
	}

	err := patchContentSecurityPolicy(
		mux,
		func(contentSecurityPolicy *csp.ContentSecurityPolicy) error {
			cspUtils.PatchCspTrustedTypes(contentSecurityPolicy, policies...)
			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("patch content security policy: %w", err)
	}

	return nil
}

// patchApiContentSecurityPolicy answers every response with the policy for a service that serves no
// documents, rather than leaving the responses it does serve with no policy at all -- the policy
// for a document being carried by documents alone.
//
// The policy for documents is left as it is, and replaces this one on a response that is one: a
// service answering a request it will not serve answers with a problem detail, which a browser asks
// for as XML and renders, and what a rendered document is answered with is said there.
func patchApiContentSecurityPolicy(mux *altshiftMux.Mux) error {
	if mux == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("mux"))
	}

	defaultHeaders := mux.DefaultHeaders
	if defaultHeaders == nil {
		return altshiftErrors.NewWithTrace(nil_error.NewWithInstance("map", "default headers"))
	}

	defaultHeaders[contentSecurityPolicyHeaderName] = ApiContentSecurityPolicy

	return nil
}
