package types

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/schema"
	altshiftTlsTypes "github.com/altshiftab/utils_go/pkg/tls/types"
)

type HttpContext struct {
	Request      *http.Request
	RequestBody  []byte
	Response     *http.Response
	ResponseBody []byte
	Reporting    *schema.HttpReporting
	TlsContext   *altshiftTlsTypes.TlsContext
	User         *schema.User
	Extra        []*HttpContext
	LocalAddr    net.Addr
	RemoteAddr   net.Addr
}

func getFullType(typeValue string, subtypeValue string, normalize bool) string {
	if typeValue == "" {
		typeValue = "*"
	}
	if subtypeValue == "" {
		subtypeValue = "*"
	}

	fullType := fmt.Sprintf("%s/%s", typeValue, subtypeValue)
	if normalize {
		return strings.ToLower(fullType)
	}

	return fullType
}

func getParameterMap(parameters [][2]string, normalize bool) map[string]string {
	if len(parameters) == 0 {
		return nil
	}

	parameterMap := make(map[string]string)

	for _, parameter := range parameters {
		key := parameter[0]
		if normalize {
			key = strings.ToLower(key)
		}
		value := parameter[1]

		if _, ok := parameterMap[key]; !ok {
			parameterMap[key] = value
		}
	}

	return parameterMap
}

func getStructuredSyntaxName(subtype string, normalize bool) string {
	if subtype == "" {
		return ""
	}

	separator := "+"

	lastSeparatorIndex := strings.LastIndex(subtype, separator)
	if lastSeparatorIndex == -1 {
		return ""
	}

	structuredSyntaxName := subtype[lastSeparatorIndex+len(separator):]
	if normalize {
		structuredSyntaxName = strings.ToLower(structuredSyntaxName)
	}

	return structuredSyntaxName
}

type MediaRange struct {
	Type       string
	Subtype    string
	Parameters [][2]string
	Weight     float32
}

func (mediaRange *MediaRange) GetFullType(normalize bool) string {
	return getFullType(mediaRange.Type, mediaRange.Subtype, normalize)
}

func (mediaRange *MediaRange) GetParameterMap(normalize bool) map[string]string {
	parameters := mediaRange.Parameters
	if len(parameters) == 0 {
		return nil
	}

	return getParameterMap(parameters, normalize)
}

func (mediaRange *MediaRange) GetStructuredSyntaxName(normalize bool) string {
	return getStructuredSyntaxName(mediaRange.Subtype, normalize)
}

type ServerMediaRange struct {
	Type    string
	Subtype string
}

func (serverMediaRange *ServerMediaRange) GetFullType(normalize bool) string {
	return getFullType(serverMediaRange.Type, serverMediaRange.Subtype, normalize)
}

func (serverMediaRange *ServerMediaRange) GetStructuredSyntaxName(normalize bool) string {
	return getStructuredSyntaxName(serverMediaRange.Subtype, normalize)
}

type Accept struct {
	MediaRanges []*MediaRange
	Raw         string
}

func (accept *Accept) GetPriorityOrderedEncodings() []*MediaRange {
	mediaRanges := make([]*MediaRange, len(accept.MediaRanges))
	copy(mediaRanges, accept.MediaRanges)

	sort.SliceStable(mediaRanges, func(i, j int) bool {
		return mediaRanges[i].Weight > mediaRanges[j].Weight
	})

	return mediaRanges
}

type Authorization struct {
	Scheme  string
	Token68 string
	Params  map[string]string
}

func isHttpTokenRune(c byte) bool {
	if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}
	return false
}

func isHttpToken(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := range len(s) {
		if !isHttpTokenRune(s[i]) {
			return false
		}
	}
	return true
}

func quoteHttpString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := range len(s) {
		c := s[i]
		if c == '"' || c == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	b.WriteByte('"')
	return b.String()
}

func (authorization *Authorization) String() string {
	if authorization.Scheme == "" {
		return ""
	}

	if authorization.Token68 != "" {
		return authorization.Scheme + " " + authorization.Token68
	}

	if len(authorization.Params) == 0 {
		return authorization.Scheme
	}

	keys := make([]string, 0, len(authorization.Params))
	for k := range authorization.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	params := make([]string, 0, len(keys))
	for _, k := range keys {
		v := authorization.Params[k]
		if !isHttpToken(v) {
			v = quoteHttpString(v)
		}
		params = append(params, k+"="+v)
	}

	return authorization.Scheme + " " + strings.Join(params, ", ")
}

type MediaType struct {
	Type       string
	Subtype    string
	Parameters [][2]string
}

func (mediaType *MediaType) GetFullType(normalize bool) string {
	return getFullType(mediaType.Type, mediaType.Subtype, normalize)
}

func (mediaType *MediaType) GetStructuredSyntaxName(normalize bool) string {
	return getStructuredSyntaxName(mediaType.Subtype, normalize)
}

func (mediaType *MediaType) GetParametersMap(normalize bool) map[string]string {
	if len(mediaType.Parameters) == 0 {
		return nil
	}

	return getParameterMap(mediaType.Parameters, normalize)
}

type ContentType struct {
	MediaType
}

type Encoding struct {
	Coding       string
	QualityValue float32
}

type AcceptEncoding struct {
	Encodings []*Encoding
	Raw       string
}

func (acceptEncoding *AcceptEncoding) GetPriorityOrderedEncodings() []*Encoding {
	encodings := make([]*Encoding, len(acceptEncoding.Encodings))
	copy(encodings, acceptEncoding.Encodings)

	sort.SliceStable(encodings, func(i, j int) bool {
		return encodings[i].QualityValue > encodings[j].QualityValue
	})

	return encodings
}

type LanguageTag struct {
	PrimarySubtag string
	Subtag        string
}

type LanguageQ struct {
	Tag *LanguageTag
	Q   float32
}
type AcceptLanguage struct {
	LanguageQs []*LanguageQ
	Raw        string
}

func (acceptLanguage *AcceptLanguage) GetPriorityOrderedLanguages() []*LanguageQ {
	languages := make([]*LanguageQ, len(acceptLanguage.LanguageQs))
	copy(languages, acceptLanguage.LanguageQs)

	sort.SliceStable(languages, func(i, j int) bool {
		return languages[i].Q > languages[j].Q
	})

	return languages
}

type StrictTransportSecurityPolicy struct {
	MaxAge            int
	IncludeSubdomains bool
	Preload           bool
	Raw               string
}

type RetryAfter struct {
	// The time can be either a timestamp or a duration.
	WaitTime any
	Raw      string
}

type ContentDisposition struct {
	DispositionType     string
	Filename            string
	FilenameAsterisk    string
	ExtensionParameters map[string]string
}

type ContentNegotiation struct {
	Accept         *Accept
	AcceptEncoding *AcceptEncoding
	AcceptLanguage *AcceptLanguage

	NegotiatedAccept         string
	NegotiatedAcceptEncoding string
}

type RobotsTxt struct {
	Groups []*RobotsTxtGroup
}

func (robotsTxt *RobotsTxt) String() string {
	var nonEmptyGroupStrings []string

	for _, group := range robotsTxt.Groups {
		if group == nil {
			continue
		}

		if groupString := group.String(); groupString != "" {
			nonEmptyGroupStrings = append(nonEmptyGroupStrings, groupString)
		}
	}

	return strings.Join(nonEmptyGroupStrings, "\n\n")
}

func makeLine(label string, value string, allowEmpty bool) string {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" && !allowEmpty {
		return ""
	}

	return fmt.Sprintf("%s: %s", label, trimmedValue)
}

func makePart(values []string, label string, allowEmpty bool) string {
	var parts []string
	for _, value := range values {
		if line := makeLine(label, value, allowEmpty); line != "" {
			parts = append(parts, line)
		}
	}

	return strings.Join(parts, "\n")
}

type RobotsTxtGroup struct {
	UserAgents   []string
	Disallowed   []string
	Allowed      []string
	OtherRecords [][2]string
}

func (robotsTxtGroup *RobotsTxtGroup) String() string {
	if len(robotsTxtGroup.UserAgents) == 0 {
		return ""
	}

	userAgentPart := makePart(robotsTxtGroup.UserAgents, "User-Agent", false)
	if userAgentPart == "" {
		return ""
	}

	parts := []string{userAgentPart}

	if disallowedPart := makePart(robotsTxtGroup.Disallowed, "Disallow", true); disallowedPart != "" {
		parts = append(parts, disallowedPart)
	}

	if allowedPart := makePart(robotsTxtGroup.Allowed, "Allow", false); allowedPart != "" {
		parts = append(parts, allowedPart)
	}

	for _, otherRecord := range robotsTxtGroup.OtherRecords {
		if line := makeLine(otherRecord[0], otherRecord[1], false); line != "" {
			parts = append(parts, line)
		}
	}

	return strings.Join(parts, "\n")
}

// SecurityTxt is what a host says about how a vulnerability in it is reported, served at
// /.well-known/security.txt (RFC 9116).
type SecurityTxt struct {
	// Contacts are where a vulnerability is reported, most preferred first, each as a URI:
	// "mailto:security@example.com", "https://example.com/vulnerability", "tel:+46-...". RFC 9116
	// requires at least one; a security.txt without one says nothing and renders as empty.
	Contacts []string
	// Expires is when the information stops being considered valid. RFC 9116 requires it, and a
	// reporter is to disregard the file once it has passed.
	Expires time.Time
	// Encryption are keys a report may be encrypted with, each as a URI. RFC 9116 forbids naming a
	// key by its fingerprint here; it is to be fetched from where this points.
	Encryption []string
	// Acknowledgments are where those who have reported are credited.
	Acknowledgments []string
	// PreferredLanguages are the languages a report is preferably written in, as RFC 5646 tags. The
	// order carries no preference, RFC 9116 notwithstanding the field's name.
	PreferredLanguages []string
	// Canonical are the URIs this security.txt is expected to be found at, so that a copy found
	// elsewhere can be told from the genuine one.
	Canonical []string
	// Policy are where the host's vulnerability disclosure policy is stated.
	Policy []string
	// Hiring are where the host advertises security-related work.
	Hiring []string
	// Csaf are where the host's CSAF provider metadata is served.
	Csaf []string
}

// String renders the security.txt. It renders as empty unless a contact is named, there being
// nothing to say to a reporter without one.
func (securityTxt *SecurityTxt) String() string {
	if securityTxt == nil {
		return ""
	}

	var lines []string

	appendLines := func(label string, values []string) {
		for _, value := range values {
			if line := makeLine(label, value, false); line != "" {
				lines = append(lines, line)
			}
		}
	}

	appendLines("Contact", securityTxt.Contacts)
	if len(lines) == 0 {
		return ""
	}

	if expires := securityTxt.Expires; !expires.IsZero() {
		lines = append(lines, makeLine("Expires", expires.UTC().Format(time.RFC3339), false))
	}

	appendLines("Encryption", securityTxt.Encryption)
	appendLines("Acknowledgments", securityTxt.Acknowledgments)

	// Preferred-Languages is a single line listing the languages, unlike the repeatable fields.
	if languages := securityTxt.PreferredLanguages; len(languages) != 0 {
		if line := makeLine("Preferred-Languages", strings.Join(languages, ", "), false); line != "" {
			lines = append(lines, line)
		}
	}

	appendLines("Canonical", securityTxt.Canonical)
	appendLines("Policy", securityTxt.Policy)
	appendLines("Hiring", securityTxt.Hiring)
	appendLines("CSAF", securityTxt.Csaf)

	return strings.Join(lines, "\n") + "\n"
}

type CorsConfiguration struct {
	Origin        string
	Methods       []string
	Headers       []string
	Credentials   bool
	MaxAge        int
	ExposeHeaders []string
}

// ForwardedElement represents a single forwarded element containing multiple parameters.
// Standard parameters defined in RFC 7239 are:
//   - For: identifies the node making the request to the proxy
//   - By: identifies the interface where the request came in to the proxy
//   - Host: the original value of the Host request header
//   - Proto: indicates the protocol used to make the request (http or https)
type ForwardedElement struct {
	For   string
	By    string
	Host  string
	Proto string
	// Extensions contain any non-standard parameters
	Extensions map[string]string
}

// Forwarded represents the parsed Forwarded HTTP header as defined in RFC 7239.
// The header can contain multiple elements, each potentially originating from
// different proxies in the request chain.
type Forwarded struct {
	Elements []*ForwardedElement
}

type ETag struct {
	Weak bool
	Tag  string
}

func (etag *ETag) String() string {
	if etag == nil {
		return ""
	}

	var b strings.Builder
	b.Grow(len(etag.Tag) + 4)
	if etag.Weak {
		b.WriteString("W/")
	}
	b.WriteByte('"')
	b.WriteString(etag.Tag)
	b.WriteByte('"')
	return b.String()
}

type CacheControlDirective struct {
	Name  string
	Value string
}

type CacheControl struct {
	Directives []*CacheControlDirective
	Raw        string
}

func (cacheControl *CacheControl) findDirective(name string) *CacheControlDirective {
	for _, directive := range cacheControl.Directives {
		if directive.Name == name {
			return directive
		}
	}
	return nil
}

func (cacheControl *CacheControl) hasDirective(name string) bool {
	return cacheControl.findDirective(name) != nil
}

var ErrDirectiveNotPresent = errors.New("directive not present")

func (cacheControl *CacheControl) deltaSeconds(name string) (int, error) {
	directive := cacheControl.findDirective(name)
	if directive == nil {
		return 0, altshiftErrors.NewWithTrace(ErrDirectiveNotPresent)
	}

	value, err := strconv.Atoi(directive.Value)
	if err != nil {
		return 0, altshiftErrors.NewWithTrace(fmt.Errorf("strconv atoi: %w", err), directive.Value)
	}

	return value, nil
}

func splitFieldNames(value string) []string {
	parts := strings.Split(value, ",")
	var result []string
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// Request and response directives.

func (cacheControl *CacheControl) MaxAge() (int, error) {
	return cacheControl.deltaSeconds("max-age")
}

func (cacheControl *CacheControl) NoCache() bool {
	return cacheControl.hasDirective("no-cache")
}

func (cacheControl *CacheControl) NoCacheFieldNames() []string {
	directive := cacheControl.findDirective("no-cache")
	if directive == nil || directive.Value == "" {
		return nil
	}

	return splitFieldNames(directive.Value)
}

func (cacheControl *CacheControl) NoStore() bool {
	return cacheControl.hasDirective("no-store")
}

func (cacheControl *CacheControl) NoTransform() bool {
	return cacheControl.hasDirective("no-transform")
}

// Request-only directives.

func (cacheControl *CacheControl) MaxStale() (int, bool, error) {
	directive := cacheControl.findDirective("max-stale")
	if directive == nil {
		return 0, false, fmt.Errorf("max-stale: %w", ErrDirectiveNotPresent)
	}
	if directive.Value == "" {
		return 0, false, nil
	}

	v, err := strconv.Atoi(directive.Value)
	if err != nil {
		return 0, true, fmt.Errorf("invalid delta-seconds for max-stale: %w", err)
	}

	return v, true, nil
}

func (cacheControl *CacheControl) MinFresh() (int, error) {
	return cacheControl.deltaSeconds("min-fresh")
}

func (cacheControl *CacheControl) OnlyIfCached() bool {
	return cacheControl.hasDirective("only-if-cached")
}

// Response-only directives.

func (cacheControl *CacheControl) MustRevalidate() bool {
	return cacheControl.hasDirective("must-revalidate")
}

func (cacheControl *CacheControl) MustUnderstand() bool {
	return cacheControl.hasDirective("must-understand")
}

func (cacheControl *CacheControl) Private() bool {
	return cacheControl.hasDirective("private")
}

func (cacheControl *CacheControl) PrivateFieldNames() []string {
	directive := cacheControl.findDirective("private")
	if directive == nil || directive.Value == "" {
		return nil
	}
	return splitFieldNames(directive.Value)
}

func (cacheControl *CacheControl) ProxyRevalidate() bool {
	return cacheControl.hasDirective("proxy-revalidate")
}

func (cacheControl *CacheControl) Public() bool {
	return cacheControl.hasDirective("public")
}

func (cacheControl *CacheControl) SMaxAge() (int, error) {
	return cacheControl.deltaSeconds("s-maxage")
}
