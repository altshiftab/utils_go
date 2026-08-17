package content_security_policy

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/altshiftab/utils_go/pkg/utils"
)

const (
	DirectiveNameBaseUri                 = "base-uri"
	DirectiveNameChildSrc                = "child-src"
	DirectiveNameConnectSrc              = "connect-src"
	DirectiveNameDefaultSrc              = "default-src"
	DirectiveNameFontSrc                 = "font-src"
	DirectiveNameFormAction              = "form-action"
	DirectiveNameFrameAncestors          = "frame-ancestors"
	DirectiveNameFrameSrc                = "frame-src"
	DirectiveNameImgSrc                  = "img-src"
	DirectiveNameManifestSrc             = "manifest-src"
	DirectiveNameMediaSrc                = "media-src"
	DirectiveNameObjectSrc               = "object-src"
	DirectiveNameReportTo                = "report-to"
	DirectiveNameReportUri               = "report-uri"
	DirectiveNameRequireSriFor           = "require-sri-for"
	DirectiveNameRequireTrustedTypesFor  = "require-trusted-types-for"
	DirectiveNameSandbox                 = "sandbox"
	DirectiveNameScriptSrc               = "script-src"
	DirectiveNameScriptSrcAttr           = "script-src-attr"
	DirectiveNameScriptSrcElem           = "script-src-elem"
	DirectiveNameStyleSrc                = "style-src"
	DirectiveNameStyleSrcAttr            = "style-src-attr"
	DirectiveNameStyleSrcElem            = "style-src-elem"
	DirectiveNameTrustedTypes            = "trusted-types"
	DirectiveNameUpgradeInsecureRequests = "upgrade-insecure-requests"
	DirectiveNameWebrtc                  = "webrtc"
	DirectiveNameWorkerSrc               = "worker-src"
)

type SourceI interface {
	GetRaw() string
	String() string
}

type ParsedSource struct {
	Raw string `json:"raw,omitempty"`
}

func (parsedSource *ParsedSource) GetRaw() string {
	return parsedSource.Raw
}

func (parsedSource *ParsedSource) String() string {
	return parsedSource.Raw
}

type NoneSource struct {
	ParsedSource
}

func (noneSource *NoneSource) String() string {
	return "'none'"
}

type SchemeSource struct {
	ParsedSource
	Scheme string `json:"scheme,omitempty"`
}

func (schemeSource *SchemeSource) String() string {
	if scheme := schemeSource.Scheme; scheme != "" {
		return scheme + ":"
	}
	return ""
}

type HostSource struct {
	ParsedSource
	Scheme     string `json:"scheme,omitempty"`
	Host       string `json:"host,omitempty"`
	PortString string `json:"port_string,omitempty"`
	Path       string `json:"path,omitempty"`
}

func HostSourceFromUrl(url *url.URL) *HostSource {
	if url == nil {
		return nil
	}

	return &HostSource{
		Scheme:     url.Scheme,
		Host:       url.Host,
		PortString: url.Port(),
		Path:       url.Path,
	}
}

func (hostSource *HostSource) String() string {
	scheme := hostSource.Scheme
	host := hostSource.Host
	portString := hostSource.PortString
	path := hostSource.Path

	if scheme == "" && host == "" && portString == "" && path == "" {
		return ""
	}

	var builder strings.Builder

	if scheme != "" {
		builder.WriteString(scheme)
		builder.WriteString("://")
	}

	builder.WriteString(host)
	if portString != "" {
		builder.WriteString(":")
		builder.WriteString(portString)
	}
	if path != "" {
		builder.WriteString(path)
	}

	return builder.String()
}

type KeywordSource struct {
	ParsedSource
	Keyword string `json:"keyword,omitempty"`
}

func (keywordSource *KeywordSource) String() string {
	if keyword := keywordSource.Keyword; keyword != "" {
		return fmt.Sprintf("'%s'", keyword)
	}
	return ""
}

type NonceSource struct {
	ParsedSource
	Base64Value string `json:"base64_value,omitempty"`
}

func (nonceSource *NonceSource) String() string {
	if nonceSource.Base64Value == "" {
		return ""
	}
	return "'nonce-" + nonceSource.Base64Value + "'"
}

type HashSource struct {
	ParsedSource
	HashAlgorithm string `json:"hash_algorithm,omitempty"`
	Base64Value   string `json:"base64_value,omitempty"`
}

func (hashSource *HashSource) String() string {
	hashAlgorithm := hashSource.HashAlgorithm
	base64Value := hashSource.Base64Value

	if hashAlgorithm == "" || base64Value == "" {
		return ""
	}

	return fmt.Sprintf("'%s-%s'", hashAlgorithm, base64Value)
}

type DirectiveI interface {
	GetName() string
	GetRawValue() string
	String() string
}

type ParsedDirective struct {
	Name    string `json:"name,omitempty"`
	Value   string `json:"value,omitempty"`
	RawName string `json:"raw_name,omitempty"`
}

func (directive *ParsedDirective) GetName() string {
	return directive.Name
}

func (directive *ParsedDirective) GetRawValue() string {
	return directive.Value
}

func (directive *ParsedDirective) String() string {
	if directive.Name == "" {
		return ""
	}

	if directive.Value == "" {
		return directive.Name
	}

	return fmt.Sprintf("%s %s", directive.Name, directive.Value)
}

type SourceDirectiveI interface {
	GetSources() []SourceI
}

type SourceDirective struct {
	ParsedDirective
	Sources []SourceI `json:"sources,omitempty"`
}

func (sourceDirective *SourceDirective) SourcesString() string {
	var sourceStrings []string

	for _, source := range sourceDirective.Sources {
		if source == nil {
			continue
		}
		if sourceString := source.String(); sourceString != "" {
			sourceStrings = append(sourceStrings, sourceString)
		}
	}

	return strings.Join(sourceStrings, " ")
}

func (sourceDirective *SourceDirective) GetSources() []SourceI {
	return sourceDirective.Sources
}

func (sourceDirective *SourceDirective) SetSources(sources []SourceI) {
	sourceDirective.Sources = sources
}

type BaseUriDirective struct {
	SourceDirective
}

func (*BaseUriDirective) GetName() string {
	return DirectiveNameBaseUri
}

func (baseUriDirective *BaseUriDirective) String() string {
	if sourcesString := baseUriDirective.SourcesString(); sourcesString != "" {
		return baseUriDirective.GetName() + " " + sourcesString
	}
	return ""
}

type ChildSrcDirective struct {
	SourceDirective
}

func (*ChildSrcDirective) GetName() string {
	return DirectiveNameChildSrc
}

func (childSrcDirective *ChildSrcDirective) String() string {
	if sourcesString := childSrcDirective.SourcesString(); sourcesString != "" {
		return childSrcDirective.GetName() + " " + sourcesString
	}
	return ""
}

type ConnectSrcDirective struct {
	SourceDirective
}

func (*ConnectSrcDirective) GetName() string {
	return DirectiveNameConnectSrc
}

func (connectSrcDirective *ConnectSrcDirective) String() string {
	if sourcesString := connectSrcDirective.SourcesString(); sourcesString != "" {
		return connectSrcDirective.GetName() + " " + sourcesString
	}
	return ""
}

type DefaultSrcDirective struct {
	SourceDirective
}

func (*DefaultSrcDirective) GetName() string {
	return DirectiveNameDefaultSrc
}

func (defaultSrcDirective *DefaultSrcDirective) String() string {
	if sourcesString := defaultSrcDirective.SourcesString(); sourcesString != "" {
		return defaultSrcDirective.GetName() + " " + sourcesString
	}
	return ""
}

type FontSrcDirective struct {
	SourceDirective
}

func (*FontSrcDirective) GetName() string {
	return DirectiveNameFontSrc
}

func (fontSrcDirective *FontSrcDirective) String() string {
	if sourcesString := fontSrcDirective.SourcesString(); sourcesString != "" {
		return fontSrcDirective.GetName() + " " + sourcesString
	}
	return ""
}

type FormActionDirective struct {
	SourceDirective
}

func (*FormActionDirective) GetName() string {
	return DirectiveNameFormAction
}

func (formActionDirective *FormActionDirective) String() string {
	if sourcesString := formActionDirective.SourcesString(); sourcesString != "" {
		return formActionDirective.GetName() + " " + sourcesString
	}
	return ""
}

type FrameSrcDirective struct {
	SourceDirective
}

func (*FrameSrcDirective) GetName() string {
	return DirectiveNameFrameSrc
}

func (frameSrcDirective *FrameSrcDirective) String() string {
	if sourcesString := frameSrcDirective.SourcesString(); sourcesString != "" {
		return frameSrcDirective.GetName() + " " + sourcesString
	}
	return ""
}

type ImgSrcDirective struct {
	SourceDirective
}

func (*ImgSrcDirective) GetName() string {
	return DirectiveNameImgSrc
}

func (imgSrcDirective *ImgSrcDirective) String() string {
	if sourcesString := imgSrcDirective.SourcesString(); sourcesString != "" {
		return imgSrcDirective.GetName() + " " + sourcesString
	}
	return ""
}

type ManifestSrcDirective struct {
	SourceDirective
}

func (*ManifestSrcDirective) GetName() string {
	return DirectiveNameManifestSrc
}

func (manifestSrcDirective *ManifestSrcDirective) String() string {
	if sourcesString := manifestSrcDirective.SourcesString(); sourcesString != "" {
		return manifestSrcDirective.GetName() + " " + sourcesString
	}
	return ""
}

type MediaSrcDirective struct {
	SourceDirective
}

func (*MediaSrcDirective) GetName() string {
	return DirectiveNameMediaSrc
}

func (mediaSrcDirective *MediaSrcDirective) String() string {
	if sourcesString := mediaSrcDirective.SourcesString(); sourcesString != "" {
		return mediaSrcDirective.GetName() + " " + sourcesString
	}
	return ""
}

type ObjectSrcDirective struct {
	SourceDirective
}

func (*ObjectSrcDirective) GetName() string {
	return DirectiveNameObjectSrc
}

func (objectSrcDirective *ObjectSrcDirective) String() string {
	if sourcesString := objectSrcDirective.SourcesString(); sourcesString != "" {
		return objectSrcDirective.GetName() + " " + sourcesString
	}
	return ""
}

type ScriptSrcAttrDirective struct {
	SourceDirective
}

func (*ScriptSrcAttrDirective) GetName() string {
	return DirectiveNameScriptSrcAttr
}

func (scriptSrcAttrDirective *ScriptSrcAttrDirective) String() string {
	if sourcesString := scriptSrcAttrDirective.SourcesString(); sourcesString != "" {
		return scriptSrcAttrDirective.GetName() + " " + sourcesString
	}
	return ""
}

type ScriptSrcDirective struct {
	SourceDirective
}

func (*ScriptSrcDirective) GetName() string {
	return DirectiveNameScriptSrc
}

func (scriptSrcDirective *ScriptSrcDirective) String() string {
	if sourcesString := scriptSrcDirective.SourcesString(); sourcesString != "" {
		return scriptSrcDirective.GetName() + " " + sourcesString
	}
	return ""
}

type ScriptSrcElemDirective struct {
	SourceDirective
}

func (*ScriptSrcElemDirective) GetName() string {
	return DirectiveNameScriptSrcElem
}

func (scriptSrcElemDirective *ScriptSrcElemDirective) String() string {
	if sourcesString := scriptSrcElemDirective.SourcesString(); sourcesString != "" {
		return scriptSrcElemDirective.GetName() + " " + sourcesString
	}
	return ""
}

type StyleSrcAttrDirective struct {
	SourceDirective
}

func (*StyleSrcAttrDirective) GetName() string {
	return DirectiveNameStyleSrcAttr
}

func (styleSrcAttrDirective *StyleSrcAttrDirective) String() string {
	if sourcesString := styleSrcAttrDirective.SourcesString(); sourcesString != "" {
		return styleSrcAttrDirective.GetName() + " " + sourcesString
	}
	return ""
}

type StyleSrcDirective struct {
	SourceDirective
}

func (*StyleSrcDirective) GetName() string {
	return DirectiveNameStyleSrc
}

func (styleSrcDirective *StyleSrcDirective) String() string {
	if sourcesString := styleSrcDirective.SourcesString(); sourcesString != "" {
		return styleSrcDirective.GetName() + " " + sourcesString
	}
	return ""
}

type StyleSrcElemDirective struct {
	SourceDirective
}

func (*StyleSrcElemDirective) GetName() string {
	return DirectiveNameStyleSrcElem
}

func (styleSrcElemDirective *StyleSrcElemDirective) String() string {
	if sourcesString := styleSrcElemDirective.SourcesString(); sourcesString != "" {
		return styleSrcElemDirective.GetName() + " " + sourcesString
	}
	return ""
}

type WorkerSrcDirective struct {
	SourceDirective
}

func (*WorkerSrcDirective) GetName() string {
	return DirectiveNameWorkerSrc
}

func (workerSrcDirective *WorkerSrcDirective) String() string {
	if sourcesString := workerSrcDirective.SourcesString(); sourcesString != "" {
		return workerSrcDirective.GetName() + " " + sourcesString
	}
	return ""
}

type SandboxDirective struct {
	ParsedDirective
	Tokens []string `json:"tokens,omitempty"`
}

func (*SandboxDirective) GetName() string {
	return DirectiveNameSandbox
}

func (sandboxDirective *SandboxDirective) String() string {
	name := sandboxDirective.GetName()

	if len(sandboxDirective.Tokens) == 0 {
		return name
	}

	return fmt.Sprintf("%s %s", name, strings.Join(sandboxDirective.Tokens, " "))
}

type WebrtcDirective struct {
	ParsedDirective
	Value string `json:"value,omitempty"`
}

func (*WebrtcDirective) GetName() string {
	return DirectiveNameWebrtc
}

func (webrtcDirective *WebrtcDirective) String() string {
	value := webrtcDirective.Value
	if value == "" {
		return ""
	}

	return webrtcDirective.GetName() + " " + fmt.Sprintf("'%s'", value)
}

type ReportUriDirective struct {
	ParsedDirective
	UriReferences []string `json:"uri_references,omitempty"`
}

func (*ReportUriDirective) GetName() string {
	return DirectiveNameReportUri
}

func (reportUriDirective *ReportUriDirective) String() string {
	if len(reportUriDirective.UriReferences) == 0 {
		return ""
	}

	return fmt.Sprintf(
		"%s %s",
		reportUriDirective.GetName(),
		strings.Join(reportUriDirective.UriReferences, " "),
	)
}

type ReportToDirective struct {
	ParsedDirective
	Token string `json:"token,omitempty"`
}

func (*ReportToDirective) GetName() string {
	return DirectiveNameReportTo
}

func (reportToDirective *ReportToDirective) String() string {
	token := reportToDirective.Token
	if token == "" {
		return ""
	}

	return reportToDirective.GetName() + " " + token
}

type FrameAncestorsDirective struct {
	SourceDirective
}

func (*FrameAncestorsDirective) GetName() string {
	return DirectiveNameFrameAncestors
}

func (frameAncestorsDirective *FrameAncestorsDirective) String() string {
	if sourcesString := frameAncestorsDirective.SourcesString(); sourcesString != "" {
		return frameAncestorsDirective.GetName() + " " + sourcesString
	}
	return ""
}

type UpgradeInsecureRequestsDirective struct {
	ParsedDirective
}

func (*UpgradeInsecureRequestsDirective) GetName() string {
	return DirectiveNameUpgradeInsecureRequests
}

func (upgradeInsecureRequestDirective *UpgradeInsecureRequestsDirective) String() string {
	return upgradeInsecureRequestDirective.GetName()
}

type RequireSriForDirective struct {
	ParsedDirective
	ResourceTypes []string `json:"resource_types,omitempty"`
}

func (*RequireSriForDirective) GetName() string {
	return DirectiveNameRequireSriFor
}

func (requireSriForDirective *RequireSriForDirective) String() string {
	resourcesTypes := requireSriForDirective.ResourceTypes
	if len(resourcesTypes) == 0 {
		return ""
	}

	return fmt.Sprintf("%s %s", requireSriForDirective.GetName(), strings.Join(resourcesTypes, " "))
}

type TrustedTypeExpression struct {
	Kind  string `json:"kind,omitempty"`
	Value string `json:"value,omitempty"`
}

type TrustedTypesDirective struct {
	ParsedDirective
	Expressions []TrustedTypeExpression `json:"expressions,omitempty"`
}

func (*TrustedTypesDirective) GetName() string {
	return DirectiveNameTrustedTypes
}

func (trustedTypesDirective *TrustedTypesDirective) String() string {
	expressions := trustedTypesDirective.Expressions
	if len(expressions) == 0 {
		return ""
	}

	var expressionStrings []string
	for _, expression := range expressions {
		kind := expression.Kind
		value := expression.Value
		if kind == "" || value == "" {
			continue
		}

		if kind == "keyword" {
			value = fmt.Sprintf("'%s'", value)
		}

		expressionStrings = append(expressionStrings, value)
	}

	return fmt.Sprintf("%s %s", trustedTypesDirective.GetName(), strings.Join(expressionStrings, " "))
}

type RequireTrustedTypesForDirective struct {
	ParsedDirective
	SinkGroups []string `json:"sink_groups,omitempty"`
}

func (*RequireTrustedTypesForDirective) GetName() string {
	return DirectiveNameRequireTrustedTypesFor
}

func (requireTrustedTypesForDirective *RequireTrustedTypesForDirective) String() string {
	sinkGroups := requireTrustedTypesForDirective.SinkGroups
	if len(sinkGroups) == 0 {
		return ""
	}

	var sinkGroupStrings []string
	for _, sinkGroup := range sinkGroups {
		if sinkGroup == "" {
			continue
		}
		sinkGroupStrings = append(sinkGroupStrings, fmt.Sprintf("'%s'", sinkGroup))
	}

	return fmt.Sprintf("%s %s", requireTrustedTypesForDirective.GetName(), strings.Join(sinkGroupStrings, " "))
}

type ContentSecurityPolicy struct {
	Directives            []DirectiveI `json:"directives"`
	OtherDirectives       []DirectiveI `json:"other_directives"`
	IneffectiveDirectives []DirectiveI `json:"ineffective_directives"`
	Raw                   string       `json:"raw,omitempty"`
}

func (csp *ContentSecurityPolicy) GetDirective(name string) (DirectiveI, bool) {
	for _, directive := range csp.Directives {
		if directive.GetName() == name {
			return directive, true
		}
	}

	for _, directive := range csp.OtherDirectives {
		if directive.GetName() == name {
			return directive, true
		}
	}

	return nil, false
}

func (csp *ContentSecurityPolicy) GetDefaultSrc() *DefaultSrcDirective {
	directive, found := csp.GetDirective(DirectiveNameDefaultSrc)
	if !found {
		return nil
	}
	return directive.(*DefaultSrcDirective)
}

func (csp *ContentSecurityPolicy) GetBaseUri() *BaseUriDirective {
	directive, found := csp.GetDirective(DirectiveNameBaseUri)
	if !found {
		return nil
	}
	return directive.(*BaseUriDirective)
}

func (csp *ContentSecurityPolicy) GetChildSrc() *ChildSrcDirective {
	directive, found := csp.GetDirective(DirectiveNameChildSrc)
	if !found {
		return nil
	}
	return directive.(*ChildSrcDirective)
}

func (csp *ContentSecurityPolicy) GetConnectSrc() *ConnectSrcDirective {
	directive, found := csp.GetDirective(DirectiveNameConnectSrc)
	if !found {
		return nil
	}
	return directive.(*ConnectSrcDirective)
}

func (csp *ContentSecurityPolicy) GetFontSrc() *FontSrcDirective {
	directive, found := csp.GetDirective(DirectiveNameFontSrc)
	if !found {
		return nil
	}
	return directive.(*FontSrcDirective)
}

func (csp *ContentSecurityPolicy) GetFormAction() *FormActionDirective {
	directive, found := csp.GetDirective(DirectiveNameFormAction)
	if !found {
		return nil
	}
	return directive.(*FormActionDirective)
}

func (csp *ContentSecurityPolicy) GetFrameSrc() *FrameSrcDirective {
	directive, found := csp.GetDirective(DirectiveNameFrameSrc)
	if !found {
		return nil
	}
	return directive.(*FrameSrcDirective)
}

func (csp *ContentSecurityPolicy) GetImgSrc() *ImgSrcDirective {
	directive, found := csp.GetDirective(DirectiveNameImgSrc)
	if !found {
		return nil
	}
	return directive.(*ImgSrcDirective)
}

func (csp *ContentSecurityPolicy) GetManifestSrc() *ManifestSrcDirective {
	directive, found := csp.GetDirective(DirectiveNameManifestSrc)
	if !found {
		return nil
	}
	return directive.(*ManifestSrcDirective)
}

func (csp *ContentSecurityPolicy) GetMediaSrc() *MediaSrcDirective {
	directive, found := csp.GetDirective(DirectiveNameMediaSrc)
	if !found {
		return nil
	}
	return directive.(*MediaSrcDirective)
}

func (csp *ContentSecurityPolicy) GetObjectSrc() *ObjectSrcDirective {
	directive, found := csp.GetDirective(DirectiveNameObjectSrc)
	if !found {
		return nil
	}
	return directive.(*ObjectSrcDirective)
}

func (csp *ContentSecurityPolicy) GetScriptSrcAttr() *ScriptSrcAttrDirective {
	directive, found := csp.GetDirective(DirectiveNameScriptSrcAttr)
	if !found {
		return nil
	}
	return directive.(*ScriptSrcAttrDirective)
}

func (csp *ContentSecurityPolicy) GetScriptSrc() *ScriptSrcDirective {
	directive, found := csp.GetDirective(DirectiveNameScriptSrc)
	if !found {
		return nil
	}
	return directive.(*ScriptSrcDirective)
}

func (csp *ContentSecurityPolicy) GetScriptSrcElem() *ScriptSrcElemDirective {
	directive, found := csp.GetDirective(DirectiveNameScriptSrcElem)
	if !found {
		return nil
	}
	return directive.(*ScriptSrcElemDirective)
}

func (csp *ContentSecurityPolicy) GetStyleSrcAttr() *StyleSrcAttrDirective {
	directive, found := csp.GetDirective(DirectiveNameStyleSrcAttr)
	if !found {
		return nil
	}
	return directive.(*StyleSrcAttrDirective)
}

func (csp *ContentSecurityPolicy) GetStyleSrc() *StyleSrcDirective {
	directive, found := csp.GetDirective(DirectiveNameStyleSrc)
	if !found {
		return nil
	}
	return directive.(*StyleSrcDirective)
}

func (csp *ContentSecurityPolicy) GetStyleSrcElem() *StyleSrcElemDirective {
	directive, found := csp.GetDirective(DirectiveNameStyleSrcElem)
	if !found {
		return nil
	}
	return directive.(*StyleSrcElemDirective)
}

func (csp *ContentSecurityPolicy) GetWorkerSrc() *WorkerSrcDirective {
	directive, found := csp.GetDirective(DirectiveNameWorkerSrc)
	if !found {
		return nil
	}
	return directive.(*WorkerSrcDirective)
}

func (csp *ContentSecurityPolicy) GetSandbox() *SandboxDirective {
	directive, found := csp.GetDirective(DirectiveNameSandbox)
	if !found {
		return nil
	}
	return directive.(*SandboxDirective)
}

func (csp *ContentSecurityPolicy) GetWebrtc() *WebrtcDirective {
	directive, found := csp.GetDirective(DirectiveNameWebrtc)
	if !found {
		return nil
	}
	return directive.(*WebrtcDirective)
}

func (csp *ContentSecurityPolicy) GetReportUri() *ReportUriDirective {
	directive, found := csp.GetDirective(DirectiveNameReportUri)
	if !found {
		return nil
	}
	return directive.(*ReportUriDirective)
}

func (csp *ContentSecurityPolicy) GetReportTo() *ReportToDirective {
	directive, found := csp.GetDirective(DirectiveNameReportTo)
	if !found {
		return nil
	}
	return directive.(*ReportToDirective)
}

func (csp *ContentSecurityPolicy) GetFrameAncestors() *FrameAncestorsDirective {
	directive, found := csp.GetDirective(DirectiveNameFrameAncestors)
	if !found {
		return nil
	}
	return directive.(*FrameAncestorsDirective)
}

func (csp *ContentSecurityPolicy) GetUpgradeInsecureRequests() *UpgradeInsecureRequestsDirective {
	directive, found := csp.GetDirective(DirectiveNameUpgradeInsecureRequests)
	if !found {
		return nil
	}
	return directive.(*UpgradeInsecureRequestsDirective)
}

func (csp *ContentSecurityPolicy) GetRequireSriFor() *RequireSriForDirective {
	directive, found := csp.GetDirective(DirectiveNameRequireSriFor)
	if !found {
		return nil
	}
	return directive.(*RequireSriForDirective)
}

func (csp *ContentSecurityPolicy) GetTrustedTypes() *TrustedTypesDirective {
	directive, found := csp.GetDirective(DirectiveNameTrustedTypes)
	if !found {
		return nil
	}
	return directive.(*TrustedTypesDirective)
}

func (csp *ContentSecurityPolicy) GetRequireTrustedTypesFor() *RequireTrustedTypesForDirective {
	directive, found := csp.GetDirective(DirectiveNameRequireTrustedTypesFor)
	if !found {
		return nil
	}
	return directive.(*RequireTrustedTypesForDirective)
}

func (csp *ContentSecurityPolicy) String() string {
	directives := csp.Directives
	otherDirectives := csp.OtherDirectives

	if len(directives) == 0 && len(otherDirectives) == 0 {
		return ""
	}

	var policies []string
	for _, directive := range slices.Concat(directives, otherDirectives) {
		if utils.IsNil(directive) {
			continue
		}

		if policyString := directive.String(); policyString != "" {
			policies = append(policies, policyString)
		}
	}

	if len(policies) == 0 {
		return ""
	}

	return strings.Join(policies, "; ")
}
