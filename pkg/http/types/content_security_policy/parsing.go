package content_security_policy

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	"github.com/altshiftab/utils_go/pkg/abnf"
	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

//go:embed grammar.abnf
var grammar []byte

var sourceListDirectiveNames = map[string]struct{}{
	DirectiveNameBaseUri:       {},
	DirectiveNameChildSrc:      {},
	DirectiveNameConnectSrc:    {},
	DirectiveNameDefaultSrc:    {},
	DirectiveNameFontSrc:       {},
	DirectiveNameFormAction:    {},
	DirectiveNameFrameSrc:      {},
	DirectiveNameImgSrc:        {},
	DirectiveNameManifestSrc:   {},
	DirectiveNameMediaSrc:      {},
	DirectiveNameObjectSrc:     {},
	DirectiveNameScriptSrc:     {},
	DirectiveNameScriptSrcAttr: {},
	DirectiveNameScriptSrcElem: {},
	DirectiveNameStyleSrc:      {},
	DirectiveNameStyleSrcAttr:  {},
	DirectiveNameStyleSrcElem:  {},
	DirectiveNameWorkerSrc:     {},
}

var Grammar *abnf.Grammar

var (
	ErrUnexpectedSourceRuleName = errors.New("unexpected source rule name")
)

// TODO: Update to use proper errors

func makeSourcesFromPaths(
	data []byte,
	paths []*abnf.Path,
	ruleName string,
) ([]SourceI, error) {
	sourceExpressionPaths := abnfUtils.SearchPath(paths[0], []string{ruleName}, 2, false)
	if len(sourceExpressionPaths) == 0 {
		return nil, nil
	}

	var sources []SourceI

	for _, sourceExpressionPath := range sourceExpressionPaths {
		concreteSourcePath := sourceExpressionPath.Subpaths[0]

		parsedSource := ParsedSource{Raw: string(abnfUtils.ExtractPathValue(data, concreteSourcePath))}

		matchRuleName := concreteSourcePath.MatchRule
		switch matchRuleName {
		case "scheme-source":
			sources = append(
				sources,
				&SchemeSource{
					ParsedSource: parsedSource,
					Scheme:       string(abnfUtils.ExtractPathValue(data, concreteSourcePath.Subpaths[0])),
				},
			)
		case "host-source":
			hostSource := &HostSource{ParsedSource: parsedSource}

			schemePartPath := abnfUtils.SearchPathSingleName(
				concreteSourcePath,
				"scheme-part",
				1,
				false,
			)
			if schemePartPath != nil {
				hostSource.Scheme = string(abnfUtils.ExtractPathValue(data, schemePartPath))
			}

			hostPartPath := abnfUtils.SearchPathSingleName(
				concreteSourcePath,
				"host-part",
				1,
				false,
			)
			if hostPartPath == nil {
				return nil, altshiftErrors.NewWithTrace(nil_error.New("host part path"), concreteSourcePath)
			}

			hostSource.Host = string(abnfUtils.ExtractPathValue(data, hostPartPath))

			portPartPath := abnfUtils.SearchPathSingleName(
				concreteSourcePath,
				"port-part",
				1,
				false,
			)
			if portPartPath != nil {
				hostSource.PortString = string(abnfUtils.ExtractPathValue(data, portPartPath))
			}

			pathPartPath := abnfUtils.SearchPathSingleName(
				concreteSourcePath,
				"path-part",
				1,
				false,
			)
			if pathPartPath != nil {
				hostSource.Path = string(abnfUtils.ExtractPathValue(data, pathPartPath))
			}

			sources = append(sources, hostSource)
		case "ancestor-keyword-source":
			fallthrough
		case "keyword-source":
			sources = append(
				sources,
				&KeywordSource{
					ParsedSource: parsedSource,
					Keyword: strings.Trim(
						string(abnfUtils.ExtractPathValue(data, concreteSourcePath.Subpaths[0])),
						"'",
					),
				},
			)
		case "nonce-source":
			sources = append(
				sources,
				&NonceSource{
					ParsedSource: parsedSource,
					Base64Value:  string(abnfUtils.ExtractPathValue(data, concreteSourcePath.Subpaths[1])),
				},
			)
		case "hash-source":
			sources = append(
				sources,
				&HashSource{
					ParsedSource:  parsedSource,
					HashAlgorithm: string(abnfUtils.ExtractPathValue(data, concreteSourcePath.Subpaths[1])),
					Base64Value:   string(abnfUtils.ExtractPathValue(data, concreteSourcePath.Subpaths[3])),
				},
			)
		default:
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %s", ErrUnexpectedSourceRuleName, matchRuleName),
				matchRuleName,
			)
		}
	}

	return sources, nil
}

func Parse(data []byte) (*ContentSecurityPolicy, error) {
	paths, err := abnfUtils.GetParsedDataPaths(Grammar, data, "serialized-policy")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError, data)
	}

	directiveNameSet := make(map[string]struct{})

	contentSecurityPolicy := &ContentSecurityPolicy{}
	contentSecurityPolicy.Raw = string(data)

	interestingPaths := abnfUtils.SearchPath(paths[0], []string{"serialized-directive"}, 2, false)
	for _, interestingPath := range interestingPaths {
		directiveNamePath := abnfUtils.SearchPathSingleName(
			interestingPath,
			"directive-name",
			1,
			false,
		)
		if directiveNamePath == nil {
			return nil, altshiftErrors.NewWithTrace(nil_error.New("directive name path"), interestingPath)
		}
		directiveName := string(abnfUtils.ExtractPathValue(data, directiveNamePath))

		var directiveValue []byte
		directiveValuePath := abnfUtils.SearchPathSingleName(
			interestingPath,
			"directive-value",
			1,
			false,
		)
		if directiveValuePath != nil {
			directiveValue = abnfUtils.ExtractPathValue(data, directiveValuePath)
		}

		isOtherDirective := false
		isIneffectiveDirective := false

		lowercaseDirectiveName := strings.ToLower(directiveName)
		if _, ok := directiveNameSet[lowercaseDirectiveName]; ok {
			isIneffectiveDirective = true
		}
		directiveNameSet[lowercaseDirectiveName] = struct{}{}

		var directive DirectiveI
		var sources []SourceI

		if _, ok := sourceListDirectiveNames[lowercaseDirectiveName]; ok {
			if bytes.Equal(directiveValue, []byte("'none'")) {
				sources = []SourceI{
					&NoneSource{
						ParsedSource: ParsedSource{Raw: string(abnfUtils.ExtractPathValue(data, directiveValuePath))},
					},
				}
			} else {
				serializedSourceListPaths, err := abnf.Parse(directiveValue, Grammar, "serialized-source-list")
				if err != nil {
					return nil, altshiftErrors.New(
						fmt.Errorf("goabnf parse (serialized source list): %w", err),
						directiveValue,
					)
				}
				if len(serializedSourceListPaths) == 0 {
					return nil, altshiftErrors.New(
						fmt.Errorf("%w (serialized-source-list)", altshiftErrors.ErrSyntaxError),
						directiveValue,
					)
				}

				sources, err = makeSourcesFromPaths(
					directiveValue,
					serializedSourceListPaths,
					"source-expression",
				)
				if err != nil {
					return nil, altshiftErrors.New(
						fmt.Errorf("make sources from paths (source expression): %w", err),
						directiveValue,
						serializedSourceListPaths,
					)
				}
				if len(sources) == 0 {
					return nil, altshiftErrors.New(
						fmt.Errorf("%w (source-expression)", altshiftErrors.ErrSyntaxError),
						directiveValue,
						serializedSourceListPaths,
					)
				}
			}
		}

		parsedDirective := ParsedDirective{
			Name:    lowercaseDirectiveName,
			Value:   string(directiveValue),
			RawName: directiveName,
		}
		sourceDirective := SourceDirective{ParsedDirective: parsedDirective, Sources: sources}

		switch lowercaseDirectiveName {
		case DirectiveNameBaseUri:
			directive = &BaseUriDirective{SourceDirective: sourceDirective}
		case DirectiveNameChildSrc:
			directive = &ChildSrcDirective{SourceDirective: sourceDirective}
		case DirectiveNameConnectSrc:
			directive = &ConnectSrcDirective{SourceDirective: sourceDirective}
		case DirectiveNameDefaultSrc:
			directive = &DefaultSrcDirective{SourceDirective: sourceDirective}
		case DirectiveNameFontSrc:
			directive = &FontSrcDirective{SourceDirective: sourceDirective}
		case DirectiveNameFormAction:
			directive = &FormActionDirective{SourceDirective: sourceDirective}
		case DirectiveNameFrameSrc:
			directive = &FrameSrcDirective{SourceDirective: sourceDirective}
		case DirectiveNameImgSrc:
			directive = &ImgSrcDirective{SourceDirective: sourceDirective}
		case DirectiveNameManifestSrc:
			directive = &ManifestSrcDirective{SourceDirective: sourceDirective}
		case DirectiveNameMediaSrc:
			directive = &MediaSrcDirective{SourceDirective: sourceDirective}
		case DirectiveNameObjectSrc:
			directive = &ObjectSrcDirective{SourceDirective: sourceDirective}
		case DirectiveNameRequireSriFor:
			resourceTypes := bytes.Split(directiveValue, []byte(" "))
			var trimmedResourceTypes []string
			for _, resourceType := range resourceTypes {
				trimmedResourceType := bytes.ToLower(bytes.TrimSpace(resourceType))
				if len(trimmedResourceType) == 0 {
					continue
				}
				trimmedResourceTypes = append(trimmedResourceTypes, string(trimmedResourceType))
			}

			directive = &RequireSriForDirective{
				ParsedDirective: parsedDirective,
				ResourceTypes:   trimmedResourceTypes,
			}
		case DirectiveNameScriptSrc:
			directive = &ScriptSrcDirective{SourceDirective: sourceDirective}
		case DirectiveNameScriptSrcAttr:
			directive = &ScriptSrcAttrDirective{SourceDirective: sourceDirective}
		case DirectiveNameScriptSrcElem:
			directive = &ScriptSrcElemDirective{SourceDirective: sourceDirective}
		case DirectiveNameStyleSrc:
			directive = &StyleSrcDirective{SourceDirective: sourceDirective}
		case DirectiveNameStyleSrcAttr:
			directive = &StyleSrcAttrDirective{SourceDirective: sourceDirective}
		case DirectiveNameStyleSrcElem:
			directive = &StyleSrcElemDirective{SourceDirective: sourceDirective}
		case DirectiveNameUpgradeInsecureRequests:
			directive = &UpgradeInsecureRequestsDirective{ParsedDirective: parsedDirective}
		case DirectiveNameWorkerSrc:
			directive = &WorkerSrcDirective{SourceDirective: sourceDirective}
		case DirectiveNameSandbox:
			sandboxDirective := &SandboxDirective{ParsedDirective: parsedDirective}

			sandboxDirectiveValuePaths, err := abnf.Parse(
				directiveValue,
				Grammar,
				"sandbox-directive-value-root",
			)
			if err != nil {
				return nil, altshiftErrors.New(
					fmt.Errorf("goabnf parse (sandbox directive value root): %w", err),
					directiveValue,
				)
			}
			if len(sandboxDirectiveValuePaths) == 0 {
				return nil, altshiftErrors.New(
					fmt.Errorf("%w (sandbox directive value root)", altshiftErrors.ErrSyntaxError),
					directiveValue,
				)
			}

			tokenPaths := abnfUtils.SearchPath(sandboxDirectiveValuePaths[0], []string{"token"}, 2, false)
			for _, tokenPath := range tokenPaths {
				sandboxDirective.Tokens = append(
					sandboxDirective.Tokens,
					string(abnfUtils.ExtractPathValue(directiveValue, tokenPath)),
				)
			}
			directive = sandboxDirective
		case DirectiveNameWebrtc:
			// Per CSP3 the value is the quoted keyword 'allow' or 'block'; directive-value retains the quotes.
			rawValue := parsedDirective.Value
			if rawValue != "'allow'" && rawValue != "'block'" {
				return nil, altshiftErrors.New(
					fmt.Errorf("%w (webrtc directive)", altshiftErrors.ErrSyntaxError),
					rawValue,
				)
			}
			webrtcDirective := &WebrtcDirective{
				ParsedDirective: parsedDirective,
				Value:           strings.Trim(rawValue, "'"),
			}
			directive = webrtcDirective
		case DirectiveNameReportUri:
			reportUriDirective := &ReportUriDirective{ParsedDirective: parsedDirective}

			reportUriDirectivePaths, err := abnf.Parse(directiveValue, Grammar, "report-uri-directive-value-root")
			if err != nil {
				return nil, altshiftErrors.New(
					fmt.Errorf("goabnf parse (report uri directive value root): %w", err),
					directiveValue,
				)
			}
			if len(reportUriDirectivePaths) == 0 {
				return nil, altshiftErrors.New(
					fmt.Errorf("%w (report uri directive value root)", altshiftErrors.ErrSyntaxError),
					directiveValue,
				)
			}

			reportUriDirectivePath := reportUriDirectivePaths[0]
			uriReferencePaths := abnfUtils.SearchPath(
				reportUriDirectivePath,
				[]string{"uri-reference"},
				1,
				false,
			)
			if len(uriReferencePaths) == 0 {
				return nil, altshiftErrors.New(
					fmt.Errorf("%w (uri-reference)", altshiftErrors.ErrSyntaxError),
					directiveValue,
				)
			}

			for _, uriReferencePath := range uriReferencePaths {
				reportUriDirective.UriReferences = append(
					reportUriDirective.UriReferences,
					string(abnfUtils.ExtractPathValue(directiveValue, uriReferencePath)),
				)
			}
			directive = reportUriDirective
		case DirectiveNameFrameAncestors:
			frameAncestorsDirective := &FrameAncestorsDirective{SourceDirective: sourceDirective}
			if bytes.Equal(directiveValue, []byte("'none'")) {
				sources = []SourceI{
					&NoneSource{
						ParsedSource: ParsedSource{Raw: string(abnfUtils.ExtractPathValue(data, directiveValuePath))},
					},
				}
			} else {
				ancestorSourceListPaths, err := abnf.Parse(directiveValue, Grammar, "ancestor-source-list-root")
				if err != nil {
					return nil, altshiftErrors.New(
						fmt.Errorf("goabnf parse (ancestor soruce list root): %w", err),
						directiveValue,
					)
				}
				if len(ancestorSourceListPaths) == 0 {
					return nil, altshiftErrors.New(
						fmt.Errorf("%w (ancestor source list root)", altshiftErrors.ErrSyntaxError),
						directiveValue,
					)
				}

				sources, err = makeSourcesFromPaths(directiveValue, ancestorSourceListPaths, "ancestor-source")
				if err != nil {
					return nil, altshiftErrors.New(
						fmt.Errorf("make sources from paths (ancestor source): %w", err),
						directiveValue,
					)
				}
				if sources == nil {
					return nil, altshiftErrors.New(
						fmt.Errorf("%w (ancestor source)", altshiftErrors.ErrSyntaxError),
						directiveValue,
						ancestorSourceListPaths,
					)
				}
			}

			frameAncestorsDirective.Sources = sources
			directive = frameAncestorsDirective
		case DirectiveNameReportTo:
			reportToDirective := &ReportToDirective{ParsedDirective: parsedDirective, Token: parsedDirective.Value}
			directive = reportToDirective
		case DirectiveNameRequireTrustedTypesFor:
			requireTrustedTypesForDirective := &RequireTrustedTypesForDirective{
				ParsedDirective: parsedDirective,
			}

			requireTrustedTypesForDirectiveValuePaths, err := abnf.Parse(directiveValue, Grammar, "require-trusted-types-for-directive-value-root")
			if err != nil {
				return nil, altshiftErrors.New(
					fmt.Errorf("goabnf parse (require trusted types for directive value root): %w", err),
					directiveValue,
				)
			}
			if len(requireTrustedTypesForDirectiveValuePaths) == 0 {
				return nil, altshiftErrors.New(
					fmt.Errorf("%w (require trusted types for directive value root)", altshiftErrors.ErrSyntaxError),
				)
			}

			sinkGroupPaths := abnfUtils.SearchPath(requireTrustedTypesForDirectiveValuePaths[0], []string{"trusted-types-sink-group"}, 2, false)
			for _, path := range sinkGroupPaths {
				requireTrustedTypesForDirective.SinkGroups = append(
					requireTrustedTypesForDirective.SinkGroups,
					strings.Trim(string(abnfUtils.ExtractPathValue(directiveValue, path)), "'"),
				)
			}
			directive = requireTrustedTypesForDirective
		case DirectiveNameTrustedTypes:
			trustedTypesDirective := &TrustedTypesDirective{ParsedDirective: parsedDirective}

			trustedTypesDirectiveValuePaths, err := abnf.Parse(directiveValue, Grammar, "trusted-types-directive-value-root")
			if err != nil {
				return nil, altshiftErrors.New(
					fmt.Errorf("goabnf parse (trusted types directive value root): %w", err),
					directiveValue,
				)
			}
			if len(trustedTypesDirectiveValuePaths) == 0 {
				return nil, altshiftErrors.New(
					fmt.Errorf("%w (trusted types directive value root)", altshiftErrors.ErrSyntaxError),
				)
			}

			ttExpressionPaths := abnfUtils.SearchPath(trustedTypesDirectiveValuePaths[0], []string{"tt-expression"}, 2, false)
			for _, ttExpressionPath := range ttExpressionPaths {
				concreteTTPath := ttExpressionPath.Subpaths[0]
				switch concreteTTPath.MatchRule {
				case "tt-policy-name":
					trustedTypesDirective.Expressions = append(
						trustedTypesDirective.Expressions,
						TrustedTypeExpression{
							Kind:  "policy-name",
							Value: string(abnfUtils.ExtractPathValue(directiveValue, concreteTTPath)),
						},
					)
				case "tt-keyword":
					val := string(abnfUtils.ExtractPathValue(directiveValue, concreteTTPath))
					trustedTypesDirective.Expressions = append(
						trustedTypesDirective.Expressions,
						TrustedTypeExpression{
							Kind:  "keyword",
							Value: strings.Trim(val, "'"),
						},
					)
				case "tt-wildcard":
					trustedTypesDirective.Expressions = append(
						trustedTypesDirective.Expressions,
						TrustedTypeExpression{
							Kind:  "wildcard",
							Value: string(abnfUtils.ExtractPathValue(directiveValue, concreteTTPath)),
						},
					)
				}
			}

			directive = trustedTypesDirective

		default:
			directive = &parsedDirective
			isOtherDirective = true
		}

		if isIneffectiveDirective {
			contentSecurityPolicy.IneffectiveDirectives = append(contentSecurityPolicy.IneffectiveDirectives, directive)
		} else if isOtherDirective {
			contentSecurityPolicy.OtherDirectives = append(contentSecurityPolicy.OtherDirectives, directive)
		} else {
			contentSecurityPolicy.Directives = append(contentSecurityPolicy.Directives, directive)
		}
	}

	return contentSecurityPolicy, nil
}

func init() {
	var err error
	Grammar, err = abnf.ParseABNF(grammar)
	if err != nil {
		panic(fmt.Sprintf("colud not parse content security policy grammar: %v", err))
	}
}

// ParseList parses the value of a Content-Security-Policy header field,
// which CSP Level 3 Section 3.1 defines as `1#serialized-policy`: a
// comma-delimited list of policies, every one of which a user agent must
// parse and enforce. A value holding a single policy is a list of one.
//
// The "#" of that production is not the one RFC 9110 Section 5.6.1 defines:
// CSP Section 2.1 replaces its OWS with optional-ascii-whitespace and gives
// only the sender expansion, which is what serialized-policy-list writes
// out.
func ParseList(data []byte) ([]*ContentSecurityPolicy, error) {
	paths, err := abnfUtils.GetParsedDataPaths(Grammar, data, "serialized-policy-list")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError, data)
	}

	policyPaths := abnfUtils.SearchPath(paths[0], []string{"serialized-policy"}, -1, false)
	if len(policyPaths) == 0 {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("serialized policy paths"), paths[0])
	}

	// The first policy is a child of the list and the rest sit within its
	// repetition, so the search reaches them at differing depths; order them
	// as they were written.
	slices.SortStableFunc(policyPaths, func(a *abnf.Path, b *abnf.Path) int { return a.Start - b.Start })

	policies := make([]*ContentSecurityPolicy, 0, len(policyPaths))
	for _, policyPath := range policyPaths {
		policy, err := Parse(abnfUtils.ExtractPathValue(data, policyPath))
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("parse: %w", err), policyPath)
		}
		policies = append(policies, policy)
	}

	return policies, nil
}
