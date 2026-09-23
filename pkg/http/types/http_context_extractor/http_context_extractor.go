package http_context_extractor

import (
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	"github.com/altshiftab/utils_go/pkg/http/types/authorization"
	csp "github.com/altshiftab/utils_go/pkg/http/types/content_security_policy"
	"github.com/altshiftab/utils_go/pkg/http/types/http_context_extractor/http_context_extractor_config"
	"github.com/altshiftab/utils_go/pkg/http/types/integrity_policy"
	"github.com/altshiftab/utils_go/pkg/http/types/reporting_api"
	"github.com/altshiftab/utils_go/pkg/iso3166"
	altshiftJson "github.com/altshiftab/utils_go/pkg/json"
	"github.com/altshiftab/utils_go/pkg/json/jose/jws"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/claims/session_claims"
	altshiftLog "github.com/altshiftab/utils_go/pkg/log"
	"github.com/altshiftab/utils_go/pkg/net/types/domain_parts"
	"github.com/altshiftab/utils_go/pkg/schema"
	schemaUtils "github.com/altshiftab/utils_go/pkg/schema/utils"
	"github.com/altshiftab/utils_go/pkg/utils"
)

const maskedValue = "(MASKED)"

func maskJws(serialization string) string {
	if parts, err := jws.Split(serialization); err == nil {
		parts[2] = maskedValue
		return strings.Join(parts[:], jws.Delimiter)
	}
	return maskedValue
}

func maskSetCookieHeader(setCookieHeader string) string {
	header := http.Header{}
	header.Add("Set-Cookie", setCookieHeader)
	resp := &http.Response{Header: header}

	cookies := resp.Cookies()
	if len(cookies) == 0 {
		return setCookieHeader
	}

	// Only the value is replaced; the cookie is reconstructed with the attributes it arrived with.
	// Those are the ones the response actually set, and a log that says otherwise says the wrong
	// thing -- so nothing is added here, whatever a cookie ought to carry.
	cookie := cookies[0] //nolint:gosec // Masking a cookie that was set, not setting one.
	cookie.Value = maskJws(cookie.Value)

	return cookie.String()
}

func maskCookieHeader(cookieHeader string) string {
	header := http.Header{}
	header.Add("Cookie", cookieHeader)
	req := &http.Request{Header: header}

	cookies := req.Cookies()
	masked := make([]string, len(cookies))
	for i, c := range cookies {
		masked[i] = c.Name + "=" + maskJws(c.Value)
	}
	return strings.Join(masked, "; ")
}

func maskBasicAuth(value string) string {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return maskedValue
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return maskedValue
	}

	return base64.StdEncoding.EncodeToString([]byte(parts[0]+":")) + maskedValue
}

func extractNormalizedHeaders(host string, header http.Header, maskedHeaders map[string]struct{}) string {
	var headerStrings []string

	if host != "" {
		headerStrings = append(headerStrings, fmt.Sprintf("Host: %s\r\n", host))
	}

	for name, values := range header {
		for _, value := range values {
			if _, ok := maskedHeaders[name]; ok {
				value = maskedValue
			} else if name == "Set-Cookie" {
				value = maskSetCookieHeader(value)
			} else if name == "Authorization" {
				parsedValue, err := authorization.Parse([]byte(value))
				if err == nil && parsedValue != nil {
					for k := range parsedValue.Params {
						parsedValue.Params[k] = maskedValue
					}

					if strings.ToLower(parsedValue.Scheme) == "basic" {
						parsedValue.Token68 = maskBasicAuth(parsedValue.Token68)
					} else {
						parsedValue.Token68 = maskJws(parsedValue.Token68)
					}

					value = parsedValue.String()
				} else {
					value = maskedValue
				}
			} else if name == "Cookie" {
				value = maskCookieHeader(value)
			} else if name == "X-Goog-Iap-Jwt-Assertion" {
				value = maskJws(value)
			}

			headerStrings = append(headerStrings, fmt.Sprintf("%s: %s\r\n", name, value))
		}
	}

	return strings.Join(headerStrings, "")
}

func extractJwtStrings(header http.Header) []string {
	var candidates []string

	if authHeader := header.Get("Authorization"); authHeader != "" {
		if scheme, token, found := strings.Cut(authHeader, " "); found && strings.EqualFold(scheme, "bearer") && token != "" {
			candidates = append(candidates, token)
		}
	}

	for _, cookie := range (&http.Request{Header: header}).Cookies() {
		if cookie.Value != "" {
			candidates = append(candidates, cookie.Value)
		}
	}

	return candidates
}

func decodeJwtPayload(token string) (map[string]any, error) {
	parts, err := jws.Split(token)
	if err != nil {
		return nil, err
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}

	return claims, nil
}

func userFromSessionClaims(claims *session_claims.Claims) *schema.User {
	sub := claims.Subject
	if sub == "" {
		return nil
	}

	subjectId, subjectEmailAddress, found := strings.Cut(sub, ":")
	if !found {
		return nil
	}

	user := &schema.User{
		Id:         subjectId,
		Email:      subjectEmailAddress,
		Unverified: true,
	}

	if azp := claims.AuthorizedParty; azp != "" {
		if tenantId, tenantName, found := strings.Cut(azp, ":"); found {
			if tenantId != "" || tenantName != "" {
				user.Group = &schema.Group{
					Id:   tenantId,
					Name: tenantName,
				}
			}
		}
	}

	user.Roles = claims.Roles

	return user
}

func userFromSubClaim(sub string) *schema.User {
	user := &schema.User{Unverified: true}

	if strings.Contains(sub, "@") {
		user.Email = sub
	} else {
		user.Name = sub
	}

	return user
}

func userFromBasicAuth(header http.Header) *schema.User {
	authHeader := header.Get("Authorization")
	if authHeader == "" {
		return nil
	}

	scheme, credentials, found := strings.Cut(authHeader, " ")
	if !found || !strings.EqualFold(scheme, "basic") || credentials == "" {
		return nil
	}

	decoded, err := base64.StdEncoding.DecodeString(credentials)
	if err != nil {
		return nil
	}

	username, _, found := strings.Cut(string(decoded), ":")
	if !found || username == "" {
		return nil
	}

	user := &schema.User{Unverified: true}
	if strings.Contains(username, "@") {
		user.Email = username
	} else {
		user.Name = username
	}

	return user
}

func extractUnverifiedUser(header http.Header) *schema.User {
	for _, token := range extractJwtStrings(header) {
		claims, err := decodeJwtPayload(token)
		if err != nil {
			continue
		}

		if sessionClaims, err := session_claims.New(claims); err == nil && sessionClaims != nil {
			if user := userFromSessionClaims(sessionClaims); user != nil {
				return user
			}
		}

		if sub, ok := claims["sub"].(string); ok && sub != "" {
			return userFromSubClaim(sub)
		}
	}

	if user := userFromBasicAuth(header); user != nil {
		return user
	}

	return nil
}

// pathMatches reports whether the incoming request path matches the pattern.
// A pattern with a trailing "/*" matches any incoming path under that segment
// prefix (e.g. "/api/customers/*" matches "/api/customers/123" but not
// "/api/customers"). An empty pattern matches anything. Otherwise the
// comparison is an exact equality check.
func pathMatches(pattern, incoming string) bool {
	if pattern == "" {
		return true
	}
	if strings.HasSuffix(pattern, "/*") {
		// Keep the trailing "/" in the prefix so we don't accidentally match
		// sibling paths like "/api/customers-v2/...".
		return strings.HasPrefix(incoming, pattern[:len(pattern)-1])
	}
	return pattern == incoming
}

// urlMatchesPattern says whether a pattern applies to a URL and, where it does, which of the
// parameters it declares that URL carries.
//
// The third result asks for the whole query to be replaced instead. It is returned for a query that
// cannot be read: a declared parameter may well be in there, and a query nothing can parse is one
// nothing can vouch for, so the safe reading of it is that it holds a secret.
func urlMatchesPattern(pattern *schema.Url, incoming *schema.Url) (bool, []string, bool) {
	if pattern == nil || incoming == nil {
		return false, nil, false
	}

	if pattern.Domain != "" && pattern.Domain != incoming.Domain {
		return false, nil, false
	}
	if !pathMatches(pattern.Path, incoming.Path) {
		return false, nil, false
	}
	if pattern.RegisteredDomain != "" && pattern.RegisteredDomain != incoming.RegisteredDomain {
		return false, nil, false
	}
	if pattern.Subdomain != "" && pattern.Subdomain != incoming.Subdomain {
		return false, nil, false
	}
	if pattern.TopLevelDomain != "" && pattern.TopLevelDomain != incoming.TopLevelDomain {
		return false, nil, false
	}

	if pattern.Query == "" {
		return true, nil, false
	}

	// The pattern is the service's own declaration rather than anything a client sent. One that
	// cannot be read names no parameters, and so masks nothing.
	patternQuery, err := url.ParseQuery(pattern.Query)
	if err != nil {
		return false, nil, false
	}

	// ParseQuery hands back the pairs it did read alongside the error, and a browser will send a
	// query it rejects -- a stray percent sign in an unrelated parameter is enough, and the page
	// still opens, because URL.Query() drops the pair it cannot read and keeps the rest. The pairs
	// that did read are masked by name; the ones that did not are covered by replacing the whole
	// query, since bailing here would leave the credential in the log in full.
	incomingQuery, incomingErr := url.ParseQuery(incoming.Query)

	var paramsToMask []string
	for paramName := range patternQuery {
		if incomingQuery.Has(paramName) {
			paramsToMask = append(paramsToMask, paramName)
		}
	}

	return true, paramsToMask, incomingErr != nil
}

func (e *Extractor) maskUrl(urlStruct *schema.Url) {
	if urlStruct == nil || len(e.MaskedUrlParams) == 0 {
		return
	}

	paramsToMask := make(map[string]struct{})
	maskWholeQuery := false
	for _, maskedUrlParam := range e.MaskedUrlParams {
		if matches, params, whole := urlMatchesPattern(maskedUrlParam, urlStruct); matches {
			if whole {
				maskWholeQuery = true
			}
			for _, param := range params {
				paramsToMask[param] = struct{}{}
			}
		}
	}

	// A query that could not be read, on a path something was declared secret for. Which parameter
	// holds what is unknowable, so the whole of it goes.
	if maskWholeQuery {
		replaceQuery := func(urlStr string) string {
			if urlStr == "" {
				return urlStr
			}

			// Parse does not read the query -- only Query() and ParseQuery do -- so a query
			// neither of those accepts still parses here, and is replaced wholesale.
			parsedUrl, err := url.Parse(urlStr)
			if err != nil || parsedUrl.RawQuery == "" {
				return urlStr
			}

			parsedUrl.RawQuery = maskedValue

			return parsedUrl.String()
		}

		urlStruct.Full = replaceQuery(urlStruct.Full)
		urlStruct.Original = replaceQuery(urlStruct.Original)
		if urlStruct.Query != "" {
			urlStruct.Query = maskedValue
		}

		return
	}

	if len(paramsToMask) == 0 {
		return
	}

	maskQueryParams := func(urlStr string) string {
		if urlStr == "" {
			return urlStr
		}

		parsedUrl, err := url.Parse(urlStr)
		if err != nil {
			return urlStr
		}

		queryValues := parsedUrl.Query()
		modified := false
		for param := range paramsToMask {
			if queryValues.Has(param) {
				queryValues.Set(param, maskedValue)
				modified = true
			}
		}

		if modified {
			parsedUrl.RawQuery = queryValues.Encode()
			return parsedUrl.String()
		}

		return urlStr
	}

	urlStruct.Full = maskQueryParams(urlStruct.Full)
	urlStruct.Original = maskQueryParams(urlStruct.Original)

	if urlStruct.Query != "" {
		parsedQuery, err := url.ParseQuery(urlStruct.Query)
		if err == nil {
			modified := false
			for param := range paramsToMask {
				if parsedQuery.Has(param) {
					parsedQuery.Set(param, maskedValue)
					modified = true
				}
			}
			if modified {
				urlStruct.Query = parsedQuery.Encode()
			}
		}
	}
}

// parseUrlString derives the schema URL a pattern is matched against from a URL written as a
// string, the way ParseHttp derives it from a request. A reported URL arrives as a string, and a
// pattern scoped by Domain or RegisteredDomain has to match it exactly as it matches a request.
func parseUrlString(raw string) *schema.Url {
	parsedUrl, err := url.Parse(raw)
	if err != nil {
		return nil
	}

	hostname := parsedUrl.Hostname()

	var port int
	if portString := parsedUrl.Port(); portString != "" {
		port, _ = strconv.Atoi(portString)
	}

	schemaUrl := &schema.Url{
		Domain:   hostname,
		Fragment: parsedUrl.Fragment,
		Full:     raw,
		Original: raw,
		Path:     parsedUrl.Path,
		Port:     port,
		Query:    parsedUrl.RawQuery,
		Scheme:   parsedUrl.Scheme,
	}

	if domainParts := domain_parts.New(hostname); domainParts != nil {
		schemaUrl.RegisteredDomain = domainParts.RegisteredDomain
		schemaUrl.Subdomain = domainParts.Subdomain
		schemaUrl.TopLevelDomain = domainParts.TopLevelDomain
	}

	return schemaUrl
}

// MaskUrlString applies the configured URL parameter masks to a URL written as a string, returning
// it with the declared parameters replaced.
//
// A URL carrying no query cannot carry a masked parameter, which disposes of the empty string and
// of the sentinels a violation report uses in place of a URL -- "inline", "eval", "wasm-eval",
// "trusted-types-policy" -- without naming them.
func (e *Extractor) MaskUrlString(raw string) string {
	if raw == "" || len(e.MaskedUrlParams) == 0 || !strings.Contains(raw, "?") {
		return raw
	}

	schemaUrl := parseUrlString(raw)
	if schemaUrl == nil {
		return raw
	}

	e.maskUrl(schemaUrl)

	return schemaUrl.Full
}

// maskUrlStringPointer is the same for an optional field, which a report uses for the members a
// browser may leave out. A nil stays nil: the field was absent, and saying otherwise would report
// something the browser did not send.
func (e *Extractor) maskUrlStringPointer(raw *string) *string {
	if raw == nil {
		return nil
	}

	masked := e.MaskUrlString(*raw)

	return &masked
}

// urlInTextPattern finds the URLs written into a free-text message -- the one a violation report is
// logged under names the URL that was blocked.
var urlInTextPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

// maskUrlsInText masks every URL appearing in a line of free text. A slog record's message is a
// plain field a handler can rewrite, unlike its attributes, so this is the only reach the extractor
// has into what a message says.
func (e *Extractor) maskUrlsInText(text string) string {
	if text == "" || len(e.MaskedUrlParams) == 0 || !strings.Contains(text, "?") {
		return text
	}

	return urlInTextPattern.ReplaceAllStringFunc(text, e.MaskUrlString)
}

type Extractor struct {
	ReplaceableMessages    map[string]struct{}
	MaskedUrlParams        []*schema.Url
	MaskedHeaders          []*http_context_extractor_config.MaskedHeader
	MaskedRequestBodyUrls  []*schema.Url
	MaskedResponseBodyUrls []*schema.Url
}

func (e *Extractor) headersToMaskForUrl(u *schema.Url) map[string]struct{} {
	if len(e.MaskedHeaders) == 0 {
		return nil
	}

	var result map[string]struct{}
	for _, entry := range e.MaskedHeaders {
		if entry == nil || len(entry.Headers) == 0 {
			continue
		}
		if entry.Url != nil {
			if u == nil {
				continue
			}
			if matches, _, _ := urlMatchesPattern(entry.Url, u); !matches {
				continue
			}
		}
		for _, h := range entry.Headers {
			if result == nil {
				result = make(map[string]struct{})
			}
			result[http.CanonicalHeaderKey(h)] = struct{}{}
		}
	}
	return result
}

// maskCspViolationBody returns a copy of a violation body with every URL it names masked.
func (e *Extractor) maskCspViolationBody(body *csp.CSPViolationReportBody) *csp.CSPViolationReportBody {
	if body == nil {
		return nil
	}

	masked := *body
	masked.DocumentURL = e.MaskUrlString(body.DocumentURL)
	masked.BlockedURL = e.MaskUrlString(body.BlockedURL)
	masked.Referrer = e.maskUrlStringPointer(body.Referrer)
	masked.SourceFile = e.maskUrlStringPointer(body.SourceFile)

	return &masked
}

// maskIntegrityViolationBody is the same for an integrity violation.
func (e *Extractor) maskIntegrityViolationBody(
	body *integrity_policy.IntegrityViolationReportBody,
) *integrity_policy.IntegrityViolationReportBody {
	if body == nil {
		return nil
	}

	masked := *body
	masked.DocumentUrl = e.MaskUrlString(body.DocumentUrl)
	masked.BlockedUrl = e.MaskUrlString(body.BlockedUrl)

	return &masked
}

// maskReports copies a batch of reports, masking the document URL each one names and whatever URLs
// its body names, the latter through the given function.
func maskReports[T any](
	e *Extractor,
	reports []*reporting_api.Report[T],
	maskBody func(T) T,
) []*reporting_api.Report[T] {
	if len(reports) == 0 {
		return reports
	}

	masked := make([]*reporting_api.Report[T], len(reports))
	for index, report := range reports {
		if report == nil {
			continue
		}

		maskedReport := *report
		maskedReport.URL = e.MaskUrlString(report.URL)
		maskedReport.Body = maskBody(report.Body)
		masked[index] = &maskedReport
	}

	return masked
}

// maskReporting returns a copy of what a browser reported, with every URL in it masked.
//
// A copy, never the original: ParseHttpContext takes the reporting straight off the HTTP context by
// pointer, and that context belongs to the request, which is logged again by the access line the
// mux writes when it is done. Masking in place would reach back into request state.
func (e *Extractor) maskReporting(reporting *schema.HttpReporting) *schema.HttpReporting {
	if reporting == nil || len(e.MaskedUrlParams) == 0 {
		return reporting
	}

	masked := &schema.HttpReporting{
		CspViolations:       maskReports(e, reporting.CspViolations, e.maskCspViolationBody),
		IntegrityViolations: maskReports(e, reporting.IntegrityViolations, e.maskIntegrityViolationBody),
	}

	// The envelope the deprecated report-uri directive posts, which names the same URLs under the
	// hyphenated spelling the previous specification gave them.
	if envelope := reporting.CspReport; envelope != nil {
		maskedEnvelope := *envelope
		if report := envelope.CspReport; report != nil {
			maskedReport := *report
			maskedReport.DocumentURI = e.MaskUrlString(report.DocumentURI)
			maskedReport.BlockedUri = e.MaskUrlString(report.BlockedUri)
			maskedReport.Referrer = e.maskUrlStringPointer(report.Referrer)
			maskedReport.SourceFile = e.maskUrlStringPointer(report.SourceFile)
			maskedEnvelope.CspReport = &maskedReport
		}
		masked.CspReport = &maskedEnvelope
	}

	return masked
}

func (e *Extractor) Handle(ctx context.Context, record *slog.Record) error {
	if record == nil {
		return nil
	}

	if requestId, ok := ctx.Value(altshiftHttpContext.RequestIdContextKey).(string); ok {
		record.Add(slog.Group("http", slog.Group("request", slog.String("id", requestId))))
	}

	if httpContext, ok := ctx.Value(altshiftHttpContext.HttpContextContextKey).(*altshiftHttpTypes.HttpContext); ok && httpContext != nil {
		if httpContext.User == nil {
			if request := httpContext.Request; request != nil {
				if requestHeader := request.Header; requestHeader != nil {
					httpContext.User = extractUnverifiedUser(requestHeader)
				}
			}
		}

		base, err := schemaUtils.ParseHttpContext(httpContext)
		if err != nil {
			return altshiftErrors.New(fmt.Errorf("parse http context: %w", err), httpContext)
		}

		if base != nil {
			maskedHeaders := e.headersToMaskForUrl(base.Url)

			// Mask URL query parameters if configured. The declared parameters are masked
			// wherever a URL carrying them appears in the entry, not only in the URL of the
			// request itself: a page whose address is a credential hands that address to
			// everything it goes on to request, as the Referer, and to everything it reports.
			if len(e.MaskedUrlParams) > 0 {
				if baseUrl := base.Url; baseUrl != nil {
					e.maskUrl(baseUrl)
				}

				if ecsHttp := base.Http; ecsHttp != nil {
					if request := ecsHttp.Request; request != nil {
						if referrer := request.Referrer; referrer != "" {
							if masked := e.MaskUrlString(referrer); masked != referrer {
								request.Referrer = masked

								// The header line is rendered separately, from the request's own
								// headers, and would otherwise still carry what was just masked.
								if maskedHeaders == nil {
									maskedHeaders = make(map[string]struct{})
								}
								maskedHeaders[http.CanonicalHeaderKey("Referer")] = struct{}{}
							}
						}

						request.Reporting = e.maskReporting(request.Reporting)
					}
				}

				// Re-rendered last, so it reflects every masked field -- ParseHttpContext builds
				// the message from the unmasked URL and the unmasked referrer alike.
				base.Message = schemaUtils.MakeHttpMessage(base)
			}

			if baseUrl := base.Url; baseUrl != nil && len(e.MaskedRequestBodyUrls) > 0 {
				if ecsHttp := base.Http; ecsHttp != nil {
					if request := ecsHttp.Request; request != nil {
						if body := request.Body; body != nil && body.Content != "" {
							for _, pattern := range e.MaskedRequestBodyUrls {
								if matches, _, _ := urlMatchesPattern(pattern, baseUrl); matches {
									body.Content = maskedValue
									break
								}
							}
						}
					}
				}
			}

			if baseUrl := base.Url; baseUrl != nil && len(e.MaskedResponseBodyUrls) > 0 {
				if ecsHttp := base.Http; ecsHttp != nil {
					if response := ecsHttp.Response; response != nil {
						if body := response.Body; body != nil && body.Content != "" {
							for _, pattern := range e.MaskedResponseBodyUrls {
								if matches, _, _ := urlMatchesPattern(pattern, baseUrl); matches {
									body.Content = maskedValue
									break
								}
							}
						}
					}
				}
			}

			if request := httpContext.Request; request != nil {
				requestHost := request.Host
				requestHeader := request.Header

				if len(requestHeader) > 0 || requestHost != "" {
					if base.Http == nil {
						base.Http = &schema.Http{}
					}

					if base.Http.Request == nil {
						base.Http.Request = &schema.HttpRequest{}
					}

					base.Http.Request.HttpHeaders = &schema.HttpHeaders{
						Normalized: extractNormalizedHeaders(requestHost, requestHeader, maskedHeaders),
					}

					var (
						clientCityName       = requestHeader.Get("X-Client-Geo-City-Name")
						clientCountryIsoCode = requestHeader.Get("X-Client-Geo-Country-Iso-Code")
						clientCityLongLat    = requestHeader.Get("X-Client-Geo-Location")
						clientRegionIsoCode  = requestHeader.Get("X-Client-Geo-Region-Iso-Code")
						clientPort           = requestHeader.Get("X-Client-Port")
						clientTlsJa3         = requestHeader.Get("X-Tls-Ja3-Fingerprint")
						clientTlsJa4         = requestHeader.Get("X-Tls-Ja4-Fingerprint")
						serverPort           = requestHeader.Get("X-Server-Port")
					)

					if clientPort != "" {
						if clientPortInt, err := strconv.Atoi(clientPort); err == nil && clientPortInt > 0 {
							ecsClient := base.Client
							if ecsClient == nil {
								base.Client = &schema.Target{}
								ecsClient = base.Client
							}
							ecsClient.Port = clientPortInt
						}
					}

					if utils.AnyNonZero(clientCityName, clientCountryIsoCode, clientCityLongLat, clientRegionIsoCode) {
						ecsClient := base.Client
						if ecsClient == nil {
							base.Client = &schema.Target{}
							ecsClient = base.Client
						}

						ecsClientGeo := ecsClient.Geo
						if ecsClientGeo == nil {
							ecsClient.Geo = &schema.Geo{}
							ecsClientGeo = ecsClient.Geo
						}

						if clientCityName != "" {
							ecsClientGeo.CityName = clientCityName
						}

						if clientCountryIsoCode != "" {
							ecsClientGeo.CountryIsoCode = clientCountryIsoCode

							if countryName := iso3166.CountryName(clientCountryIsoCode); countryName != "" {
								ecsClientGeo.CountryName = countryName
							}
						}

						if clientRegionIsoCode != "" {
							ecsClientGeo.RegionIsoCode = clientRegionIsoCode
						}
						if clientCityLongLat != "" {
							if lat, lon, ok := strings.Cut(clientCityLongLat, ","); ok {
								ecsClientGeo.Location = map[string]string{"lat": lat, "lon": lon}
							}
						}
					}

					if utils.AnyNonZero(clientTlsJa3, clientTlsJa4) {
						ecsTls := base.Tls
						if ecsTls == nil {
							base.Tls = &schema.Tls{}
							ecsTls = base.Tls
						}

						ecsTlsClient := ecsTls.Client
						if ecsTlsClient == nil {
							ecsTls.Client = &schema.TlsClient{}
							ecsTlsClient = ecsTls.Client
						}

						if clientTlsJa3 != "" {
							ecsTlsClient.Ja3 = clientTlsJa3
						}

						if clientTlsJa4 != "" {
							ecsTlsClient.Ja4 = clientTlsJa4
						}
					}

					if serverPort != "" {
						if serverPortInt, err := strconv.Atoi(serverPort); err == nil && serverPortInt > 0 {
							ecsServer := base.Server
							if ecsServer == nil {
								base.Server = &schema.Target{}
								ecsServer = base.Server
							}
							ecsServer.Port = serverPortInt
						}
					}

					// TODO: Add community id for client + server; protocol can be inferred based on HTTP version header.
				}
			}

			if response := httpContext.Response; response != nil {
				if responseHeader := response.Header; responseHeader != nil {
					if base.Http == nil {
						base.Http = &schema.Http{}
					}

					if base.Http.Response == nil {
						base.Http.Response = &schema.HttpResponse{}
					}

					base.Http.Response.HttpHeaders = &schema.HttpHeaders{
						Normalized: extractNormalizedHeaders("", responseHeader, maskedHeaders),
					}
				}
			}

			ecsNetwork := base.Network
			if ecsNetwork == nil {
				base.Network = &schema.Network{}
				ecsNetwork = base.Network
			}

			if utils.IsNil(httpContext.LocalAddr) {
				ecsNetwork.Direction = "ingress"
			} else {
				ecsNetwork.Direction = "egress"
			}

			if e.ReplaceableMessages != nil {
				if _, ok := e.ReplaceableMessages[record.Message]; ok {
					record.Message = base.Message
				}
			}

			// A message the caller wrote is not built from the URL and so escapes the masking
			// above -- the one a violation report is logged under names the URL that was blocked.
			// The message is a plain field; a record's attributes cannot be rewritten this way,
			// which is why this reaches only what a message says.
			record.Message = e.maskUrlsInText(record.Message)

			base.Message = ""

			baseMap, err := altshiftJson.ObjectToMap(base)
			if err != nil {
				return altshiftErrors.New(fmt.Errorf("object to map: %w", err), base)
			}

			record.Add(altshiftLog.AttrsFromMap(baseMap)...)
		}
	}

	return nil
}

func New(options ...http_context_extractor_config.Option) *Extractor {
	config := http_context_extractor_config.New(options...)

	var messagesMap map[string]struct{}
	if len(config.ReplaceableMessages) > 0 {
		messagesMap = make(map[string]struct{}, len(config.ReplaceableMessages))
		for _, msg := range config.ReplaceableMessages {
			messagesMap[msg] = struct{}{}
		}
	}

	return &Extractor{
		ReplaceableMessages:    messagesMap,
		MaskedUrlParams:        config.MaskedUrlParams,
		MaskedHeaders:          config.MaskedHeaders,
		MaskedRequestBodyUrls:  config.MaskedRequestBodyUrls,
		MaskedResponseBodyUrls: config.MaskedResponseBodyUrls,
	}
}
