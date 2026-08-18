package utils

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	gmailMessage "github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message"
	gmailMessagePart "github.com/altshiftab/utils_go/pkg/cloud/gws/gmail/types/message/message_part"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
	altshiftNet "github.com/altshiftab/utils_go/pkg/net"
	"github.com/altshiftab/utils_go/pkg/net/types/domain_parts"
	"github.com/altshiftab/utils_go/pkg/net/types/flow_tuple"
	"github.com/altshiftab/utils_go/pkg/schema"
	altshiftTlsTypes "github.com/altshiftab/utils_go/pkg/tls/types"
	altshiftWhoisTypes "github.com/altshiftab/utils_go/pkg/whois/types"
)

const (
	timestampFormat = "2006-01-02T15:04:05.999999999Z"
)

func DefaultHeaderExtractorWithMasking(requestResponse any, maskNames []string, maskValue string) string {
	var header http.Header

	switch typedRequestResponse := requestResponse.(type) {
	case *http.Request:
		header = typedRequestResponse.Header
	case *http.Response:
		header = typedRequestResponse.Header
	default:
		return ""
	}

	var headerStrings []string

	for name, values := range header {
		shouldMask := slices.Contains(maskNames, strings.ToLower(name))
		for _, value := range values {
			if shouldMask {
				value = maskValue
			}
			headerStrings = append(headerStrings, fmt.Sprintf("%s: %s\r\n", name, value))
		}
	}

	return strings.Join(headerStrings, "")
}

func DefaultMaskedHeaderExtractor(requestResponse any) string {
	return DefaultHeaderExtractorWithMasking(
		requestResponse,
		[]string{"authorization", "cookie", "set-cookie"},
		"(MASKED)",
	)
}

func DefaultHeaderExtractor(requestResponse any) string {
	return DefaultHeaderExtractorWithMasking(requestResponse, nil, "")
}

func ParseHttp(
	request *http.Request,
	requestBodyData []byte,
	response *http.Response,
	responseBodyData []byte,
) (*schema.Base, error) {
	if request == nil && len(requestBodyData) == 0 && response == nil && len(responseBodyData) == 0 {
		return nil, nil
	}

	network := &schema.Network{Protocol: "http"}

	var source *schema.Target
	var destination *schema.Target
	var client *schema.Target
	var server *schema.Target

	var ecsUrl *schema.Url
	var userAgent *schema.UserAgent
	var httpVersion string

	var httpRequest *schema.HttpRequest
	if request != nil {
		requestUrl := request.URL
		originalUrl := requestUrl.String()

		hostSource := requestUrl.Host
		if hostSource == "" {
			hostSource = request.Host
		}
		hostUrl := &url.URL{Host: hostSource}
		trimmedHost := hostUrl.Hostname()

		host := trimmedHost
		if len(hostSource) > 0 && hostSource[0] == '[' {
			host = "[" + trimmedHost + "]"
		}

		domainParts := domain_parts.New(trimmedHost)

		requestHeader := request.Header

		var forwardedString string
		var xForwardedFor string
		if requestHeader != nil {
			forwardedString = requestHeader.Get("Forwarded")
			xForwardedFor = requestHeader.Get("X-Forwarded-For")
		}

		var port int
		if portString := requestUrl.Port(); portString != "" {
			port, _ = strconv.Atoi(portString)
		}

		if trimmedHost != "" || port != 0 {
			destination = &schema.Target{Address: trimmedHost, Port: port}
			if ip := net.ParseIP(trimmedHost); ip != nil {
				destination.Ip = trimmedHost
				if ipVersion := altshiftNet.GetIpVersion(&ip); ipVersion == 4 {
					network.Type = "ipv4"
				} else if ipVersion == 6 {
					network.Type = "ipv6"
				}
			} else {
				destination.Domain = trimmedHost
				if domainParts != nil {
					destination.RegisteredDomain = domainParts.RegisteredDomain
					destination.Subdomain = domainParts.Subdomain
					destination.TopLevelDomain = domainParts.TopLevelDomain
				}
			}
		}

		if destinationTcpAddr, ok := request.Context().Value(http.LocalAddrContextKey).(*net.TCPAddr); ok {
			if destination == nil {
				destination = &schema.Target{}
			}
			destination.Ip = destinationTcpAddr.IP.String()
			destination.Port = destinationTcpAddr.Port

			if ipVersion := altshiftNet.GetIpVersion(&destinationTcpAddr.IP); ipVersion == 4 {
				network.Type = "ipv4"
			} else if ipVersion == 6 {
				network.Type = "ipv6"
			}
		}

		var username string
		var password string
		if userInfo := requestUrl.User; userInfo != nil {
			username = userInfo.Username()
			password, _ = userInfo.Password()
		}

		// TODO: Maybe I can use `parseTarget()`?
		if remoteAddr := request.RemoteAddr; remoteAddr != "" {
			sourceIpAddress, sourcePort, err := altshiftNet.SplitAddress(remoteAddr)
			if err != nil {
				return nil, altshiftErrors.New(
					fmt.Errorf("split address: %w", err),
					remoteAddr,
				)
			}
			source = &schema.Target{Ip: sourceIpAddress, Port: sourcePort}
		}

		if userAgentOriginal := request.UserAgent(); userAgentOriginal != "" {
			userAgent = &schema.UserAgent{Original: userAgentOriginal}
		}

		ecsUrl = &schema.Url{
			Domain:   host,
			Fragment: requestUrl.Fragment,
			Original: originalUrl,
			Password: password,
			Path:     requestUrl.Path,
			Port:     port,
			Query:    requestUrl.RawQuery,
			Scheme:   requestUrl.Scheme,
			Username: username,
		}
		if domainParts != nil {
			ecsUrl.RegisteredDomain = domainParts.RegisteredDomain
			ecsUrl.Subdomain = domainParts.Subdomain
			ecsUrl.TopLevelDomain = domainParts.TopLevelDomain
		}

		var contentType string
		if requestHeader != nil {
			contentType = requestHeader.Get("Content-Type")
		}

		httpRequest = &schema.HttpRequest{
			ContentType: contentType,
			Method:      request.Method,
			Referrer:    request.Referer(),
		}

		httpVersionMajor := request.ProtoMajor
		httpVersionMinor := request.ProtoMinor

		if httpVersionMajor != 0 || httpVersionMinor != 0 {
			httpVersion = fmt.Sprintf("%d.%d", request.ProtoMajor, request.ProtoMinor)

			if strings.HasPrefix(httpVersion, "3.") {
				network.Transport = "udp"
				network.IanaNumber = "17"
			} else {
				network.Transport = "tcp"
				network.IanaNumber = "6"
			}

			if destination != nil && source != nil {
				destinationIp := net.ParseIP(destination.Ip)
				serverIp := net.ParseIP(source.Ip)
				destinationPort := destination.Port
				sourcePort := source.Port

				protocolNumber, _ := strconv.Atoi(network.IanaNumber)

				if destinationIp != nil && serverIp != nil && destinationPort != 0 && sourcePort != 0 && protocolNumber != 0 {
					flowTuple := flow_tuple.New(
						destinationIp,
						serverIp,
						uint16(destinationPort), //nolint:gosec // port fits uint16
						uint16(sourcePort),      //nolint:gosec // port fits uint16
						uint8(protocolNumber),   //nolint:gosec // IANA protocol number fits uint8
					)
					if flowTuple != nil {
						if communityId := flowTuple.Hash(); communityId != "" {
							network.CommunityId = append(network.CommunityId, communityId)
						}
					}
				}
			}
		}

		if forwardedString == "" && xForwardedFor == "" {
			client = source
			server = destination
		} else {
			// TODO: Currently relies on `X-Forwarded-For` rather than `Forwarded`; using the latter
			//	entails the inclusion of an external parsing library, which is not acceptable.

			var serverIpAddress string

			forwardedForSplit := strings.Split(xForwardedFor, ",")
			if len(forwardedForSplit) > 0 {
				forwardedForIpAddress := strings.TrimSpace(forwardedForSplit[0])

				if ip := net.ParseIP(forwardedForIpAddress); ip != nil {
					client = &schema.Target{Ip: forwardedForIpAddress, Address: forwardedForIpAddress}
					network.ForwardedIp = forwardedForIpAddress
				}

				if len(forwardedForSplit) > 1 {
					serverIpAddressElement := forwardedForSplit[len(forwardedForSplit)-1]
					if ip := net.ParseIP(serverIpAddressElement); ip != nil {
						serverIpAddress = serverIpAddressElement
					}
				}
			}

			if destination != nil && destination.Domain != "" {
				destinationCopy := *destination
				server = &destinationCopy
				server.Ip = serverIpAddress
				server.Port = 0
				server.Address = server.Domain
			}
		}
	}

	if len(requestBodyData) != 0 {
		if httpRequest == nil {
			httpRequest = &schema.HttpRequest{}
		}
		httpRequest.Body = &schema.Body{Bytes: len(requestBodyData), Content: string(requestBodyData)}
		httpRequest.MimeType = http.DetectContentType(requestBodyData)
	}

	var httpResponse *schema.HttpResponse
	if response != nil {
		httpResponse = &schema.HttpResponse{
			StatusCode:  response.StatusCode,
			ContentType: response.Header.Get("Content-Type"),
		}
	}

	if len(responseBodyData) != 0 {
		if httpResponse == nil {
			httpResponse = &schema.HttpResponse{}
		}
		httpResponse.Body = &schema.Body{Bytes: len(responseBodyData), Content: string(responseBodyData)}
		httpResponse.MimeType = http.DetectContentType(responseBodyData)
	}

	var ecsHttp *schema.Http
	if httpRequest != nil || httpResponse != nil {
		ecsHttp = &schema.Http{Request: httpRequest, Response: httpResponse, Version: httpVersion}
	}

	if source == nil && ecsHttp == nil && destination == nil && ecsUrl == nil && userAgent == nil && network == nil {
		return nil, nil
	}

	return &schema.Base{
		Client:      client,
		Destination: destination,
		Http:        ecsHttp,
		Server:      server,
		Source:      source,
		Url:         ecsUrl,
		UserAgent:   userAgent,
		Network:     network,
	}, nil
}

func MakeHttpMessage(base *schema.Base) string {
	if base == nil {
		return ""
	}

	sourceTargetIpAddress := "-"
	for _, target := range []*schema.Target{base.Client, base.Source} {
		if target == nil {
			continue
		}
		if target.Ip != "" {
			sourceTargetIpAddress = target.Ip
			break
		}
	}

	userName := "-"
	if user := base.User; user != nil {
		if user.Name != "" {
			userName = user.Name
		} else if user.Email != "" {
			userName = user.Email
		}
	}

	requestLine := "-"
	referrer := "-"
	userAgentOriginal := "-"

	if ecsHttp := base.Http; ecsHttp != nil {
		if httpRequest := ecsHttp.Request; httpRequest != nil {
			method := httpRequest.Method
			if method == "" {
				method = "-"
			}

			path := "-"
			if ecsUrl := base.Url; ecsUrl != nil {
				if ecsUrl.Original != "" {
					path = ecsUrl.Original
				} else if ecsUrl.Path != "" {
					path = ecsUrl.Path
					if ecsUrl.Query != "" {
						path += "?" + ecsUrl.Query
					}
				}
			}

			proto := "-"
			if ecsHttp.Version != "" {
				proto = "HTTP/" + ecsHttp.Version
			}

			requestLine = fmt.Sprintf("%s %s %s", method, path, proto)

			if httpRequest.Referrer != "" {
				referrer = httpRequest.Referrer
			}
		}
	}

	if userAgent := base.UserAgent; userAgent != nil {
		if userAgent.Original != "" {
			userAgentOriginal = userAgent.Original
		}
	}

	statusCodeString := "-"
	bodyBytesString := "-"
	if ecsHttp := base.Http; ecsHttp != nil {
		if httpResponse := ecsHttp.Response; httpResponse != nil {
			if httpResponse.StatusCode != 0 {
				statusCodeString = strconv.Itoa(httpResponse.StatusCode)
			}
			if body := httpResponse.Body; body != nil {
				if body.Bytes != 0 {
					bodyBytesString = strconv.Itoa(body.Bytes)
				}
			}
		}
	}

	return fmt.Sprintf(
		"%s - %s \"%s\" %s %s \"%s\" \"%s\"",
		sourceTargetIpAddress,
		userName,
		requestLine,
		statusCodeString,
		bodyBytesString,
		referrer,
		userAgentOriginal,
	)
}

func ParseHttpContext(httpContext *altshiftHttpTypes.HttpContext) (*schema.Base, error) {
	if httpContext == nil {
		return nil, nil
	}

	base, err := ParseHttp(httpContext.Request, httpContext.RequestBody, httpContext.Response, httpContext.ResponseBody)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("parse http: %w", err))
	}

	if base == nil {
		base = &schema.Base{}
	}

	if localAddr := httpContext.LocalAddr; localAddr != nil && base.Source == nil {
		ipAddress, port, err := altshiftNet.SplitAddress(localAddr.String())
		if err != nil {
			return nil, altshiftErrors.New(
				fmt.Errorf("split address (local addr): %w", err),
				localAddr.String(),
			)
		}
		base.Source = &schema.Target{Ip: ipAddress, Port: port}
	}

	if remoteAddr := httpContext.RemoteAddr; remoteAddr != nil {
		ipAddress, port, err := altshiftNet.SplitAddress(remoteAddr.String())
		if err != nil {
			return nil, altshiftErrors.New(
				fmt.Errorf("split address (remote addr): %w", err),
				remoteAddr.String(),
			)
		}

		if base.Destination == nil {
			base.Destination = &schema.Target{Ip: ipAddress, Port: port}
		} else if base.Destination.Ip == "" {
			base.Destination.Ip = ipAddress
		}
	}

	if base.Client == nil && base.Source != nil {
		base.Client = base.Source
	}

	if base.Server == nil && base.Destination != nil {
		base.Server = base.Destination
	} else if base.Server != nil && base.Server.Ip == "" && base.Destination != nil {
		base.Server.Ip = base.Destination.Ip
	}

	EnrichWithTlsContext(base, httpContext.TlsContext)

	base.User = httpContext.User
	base.Message = MakeHttpMessage(base)

	if base.Http != nil && base.Http.Request != nil {
		base.Http.Request.Reporting = httpContext.Reporting
	}

	return base, nil
}

func parseTarget(rawAddress string, rawIpAddress string, rawPort int) (*schema.Target, error) {
	var target *schema.Target

	if rawIpAddress != "" {
		ipAddressUrl := fmt.Sprintf("fake://%s", rawIpAddress)
		urlParsedClientIpAddress, err := url.Parse(ipAddressUrl)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("url parse (crafted ip address url): %w", err),
				ipAddressUrl,
			)
		}

		port := rawPort

		if portString := urlParsedClientIpAddress.Port(); portString != "" {
			port, err = strconv.Atoi(portString)
			if err != nil {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("strconv atoi (port string): %w", err),
					portString,
				)
			}
		}

		ipAddress := urlParsedClientIpAddress.Hostname()
		address := rawAddress
		if address != "" {
			address = ipAddress
		}

		target = &schema.Target{Address: address, Domain: rawAddress, Ip: ipAddress, Port: port}
	} else if rawAddress != "" {
		target = &schema.Target{
			Address: rawAddress,
			Domain:  rawAddress,
			Port:    rawPort,
		}
	}

	if target != nil {
		if domain := target.Domain; domain != "" {
			domainParts := domain_parts.New(domain)
			if domainParts != nil {
				target.RegisteredDomain = domainParts.RegisteredDomain
				target.Subdomain = domainParts.Subdomain
				target.TopLevelDomain = domainParts.TopLevelDomain
			}
		}
	}

	return target, nil
}

func ParseWhoisContext(whoisContext *altshiftWhoisTypes.WhoisContext) (*schema.Base, error) {
	if whoisContext == nil {
		return nil, nil
	}

	clientAddress := whoisContext.ClientAddress
	clientIpAddress := whoisContext.ClientIpAddress
	clientPort := whoisContext.ClientPort
	client, err := parseTarget(clientAddress, clientIpAddress, clientPort)
	if err != nil {
		return nil, altshiftErrors.New(
			fmt.Errorf("parse target (client data): %w", err),
			clientAddress, clientAddress, clientPort,
		)
	}
	var requestBody *schema.Body
	if requestData := whoisContext.RequestData; len(requestData) > 0 {
		requestBody = &schema.Body{Bytes: len(requestData), Content: string(requestData)}
	}

	serverAddress := whoisContext.ServerAddress
	serverIpAddress := whoisContext.ServerIpAddress
	serverPort := whoisContext.ServerPort
	server, err := parseTarget(serverAddress, serverIpAddress, serverPort)
	if err != nil {
		return nil, altshiftErrors.New(
			fmt.Errorf("parse target (server data): %w", err),
			serverAddress, serverIpAddress, serverPort,
		)
	}
	var responseBody *schema.Body
	if responseData := whoisContext.ResponseData; len(responseData) > 0 {
		responseBody = &schema.Body{Bytes: len(responseData), Content: string(responseData)}
	}

	var whois *schema.Whois
	if requestBody != nil || responseBody != nil {
		whois = &schema.Whois{}
		if requestBody != nil {
			whois.Request = &schema.WhoisRequest{Body: requestBody}
		}
		if responseBody != nil {
			whois.Response = &schema.WhoisResponse{Body: responseBody}
		}
	}

	return &schema.Base{
		Client:  client,
		Network: &schema.Network{Protocol: "whois", Transport: whoisContext.Transport},
		Server:  server,
		Whois:   whois,
	}, nil
}

func EventCreatedReplaceAttr(groups []string, attr slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return attr
	}

	switch attr.Key {
	case slog.TimeKey:
		return slog.Group("event", slog.Any("created", attr.Value))
	case slog.LevelKey:
		if value, ok := attr.Value.Any().(string); ok {
			return slog.Group("log", slog.String("level", strings.ToLower(value)))
		}
	case slog.MessageKey:
		attr.Key = "message"
	}

	return attr
}

func TimestampReplaceAttr(groups []string, attr slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return attr
	}

	switch attr.Key {
	case slog.TimeKey:
		attr.Key = "@timestamp"
	case slog.LevelKey:
		if level, ok := attr.Value.Any().(slog.Level); ok {
			return slog.Group("log", slog.String("level", strings.ToLower(level.String())))
		}
	case slog.MessageKey:
		attr.Key = "message"
	}

	return attr
}

func FlowTupleFromTargets(sourceTarget, destinationTarget *schema.Target, protocolNumber int) *flow_tuple.Tuple {
	if sourceTarget == nil || destinationTarget == nil || protocolNumber == 0 {
		return nil
	}

	sourceTargetIp := net.ParseIP(sourceTarget.Ip)
	destinationTargetIp := net.ParseIP(destinationTarget.Ip)
	sourceTargetPort := sourceTarget.Port
	destinationTargetPort := destinationTarget.Port

	if sourceTargetIp == nil || destinationTargetIp == nil || sourceTargetPort == 0 || destinationTargetPort == 0 {
		return nil
	}

	return flow_tuple.New(
		sourceTargetIp,
		destinationTargetIp,
		uint16(sourceTargetPort),      //nolint:gosec // port fits uint16
		uint16(destinationTargetPort), //nolint:gosec // port fits uint16
		uint8(protocolNumber),         //nolint:gosec // IANA protocol number fits uint8
	)
}

func CommunityIdFromTargets(sourceTarget, destinationTarget *schema.Target, protocolNumber int) string {
	flowTuple := FlowTupleFromTargets(sourceTarget, destinationTarget, protocolNumber)
	if flowTuple == nil {
		return ""
	}
	return flowTuple.Hash()
}

func EnrichWithTlsConnectionState(base *schema.Base, connectionState *tls.ConnectionState, clientInitiated bool) {
	if base == nil {
		return
	}

	if connectionState == nil {
		return
	}

	ecsTls := base.Tls
	if ecsTls == nil {
		ecsTls = &schema.Tls{}
		base.Tls = ecsTls
	}

	ecsTls.Cipher = tls.CipherSuiteName(connectionState.CipherSuite)
	ecsTls.Established = connectionState.HandshakeComplete
	ecsTls.NextProtocol = strings.ToLower(connectionState.NegotiatedProtocol)
	ecsTls.Resumed = connectionState.DidResume

	switch connectionState.Version {
	case tls.VersionSSL30: //nolint:staticcheck // constant referenced only to label SSLv3, not to enable it
		ecsTls.TlsProtocol = &schema.TlsProtocol{Name: "ssl", Version: "3"}
	case tls.VersionTLS10:
		ecsTls.TlsProtocol = &schema.TlsProtocol{Name: "tls", Version: "1.0"}
	case tls.VersionTLS11:
		ecsTls.TlsProtocol = &schema.TlsProtocol{Name: "tls", Version: "1.1"}
	case tls.VersionTLS12:
		ecsTls.TlsProtocol = &schema.TlsProtocol{Name: "tls", Version: "1.2"}
	case tls.VersionTLS13:
		ecsTls.TlsProtocol = &schema.TlsProtocol{Name: "tls", Version: "1.3"}
	}

	if serverName := connectionState.ServerName; serverName != "" {
		ecsTlsClient := ecsTls.Client
		if ecsTlsClient == nil {
			ecsTlsClient = &schema.TlsClient{}
			ecsTls.Client = ecsTlsClient
		}

		ecsTlsClient.ServerName = serverName
	}

	if peerCertificates := connectionState.PeerCertificates; len(peerCertificates) > 0 {
		if leaf := peerCertificates[0]; leaf != nil {
			// TODO: Add more fields.

			issuer := leaf.Issuer.String()
			subject := leaf.Subject.String()
			notAfter := leaf.NotAfter.UTC().Format(timestampFormat)
			notBefore := leaf.NotBefore.UTC().Format(timestampFormat)

			if !clientInitiated {
				ecsTlsClient := ecsTls.Client
				if ecsTlsClient == nil {
					ecsTlsClient = &schema.TlsClient{}
					ecsTls.Client = ecsTlsClient
				}

				ecsTlsClient.Issuer = issuer
				ecsTlsClient.Subject = subject
				ecsTlsClient.NotAfter = notAfter
				ecsTlsClient.NotBefore = notBefore
			} else {
				ecsTlsServer := ecsTls.Server
				if ecsTlsServer == nil {
					ecsTlsServer = &schema.TlsServer{}
					ecsTls.Server = ecsTlsServer
				}

				ecsTlsServer.Issuer = issuer
				ecsTlsServer.Subject = subject
				ecsTlsServer.NotAfter = notAfter
				ecsTlsServer.NotBefore = notBefore
			}
		}
	}
}

func EnrichWithTlsContext(base *schema.Base, tlsContext *altshiftTlsTypes.TlsContext) {
	if base == nil {
		return
	}

	if tlsContext == nil {
		return
	}

	connectionState := tlsContext.ConnectionState
	if connectionState == nil {
		return
	}

	EnrichWithTlsConnectionState(base, connectionState, tlsContext.ClientInitiated)
}

func ParseEmailAddress(value string) (*schema.EmailAddress, error) {
	if value == "" {
		return nil, nil
	}

	addr, err := mail.ParseAddress(value)
	if err != nil {
		return nil, fmt.Errorf("mail parse address: %w", err)
	}
	return &schema.EmailAddress{Address: addr.Address, Name: addr.Name}, nil
}

func getHeader(msg *gmailMessage.Message, name string) string {
	if msg == nil || msg.Payload == nil {
		return ""
	}

	for _, h := range msg.Payload.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}

	return ""
}

func parseEmailAddresses(value string) []*schema.EmailAddress {
	if value == "" {
		return nil
	}

	addrs, err := mail.ParseAddressList(value)
	if err != nil {
		addr, err := mail.ParseAddress(value)
		if err != nil {
			return nil
		}
		return []*schema.EmailAddress{{Address: addr.Address, Name: addr.Name}}
	}

	var result []*schema.EmailAddress
	for _, addr := range addrs {
		result = append(result, &schema.EmailAddress{Address: addr.Address, Name: addr.Name})
	}
	return result
}

func collectAttachments(part *gmailMessagePart.MessagePart) []*schema.EmailAttachment {
	if part == nil {
		return nil
	}

	var attachments []*schema.EmailAttachment

	if part.Filename != "" && part.Body != nil {
		attachment := &schema.EmailAttachment{
			File: &schema.EmailAttachmentFile{
				Name:     part.Filename,
				MimeType: part.MimeType,
				Size:     part.Body.Size,
			},
		}
		if ext := filepath.Ext(part.Filename); ext != "" {
			attachment.File.Extension = strings.TrimPrefix(ext, ".")
		}
		attachments = append(attachments, attachment)
	}

	for _, child := range part.Parts {
		attachments = append(attachments, collectAttachments(child)...)
	}

	return attachments
}

func EnrichWithGmailMessage(base *schema.Base, msg *gmailMessage.Message) {
	if base == nil {
		return
	}

	if msg == nil || msg.Payload == nil {
		return
	}

	email := base.Email
	if email == nil {
		email = &schema.Email{}
		base.Email = email
	}

	if messageId := getHeader(msg, "Message-ID"); messageId != "" {
		email.MessageId = messageId
	}

	if subject := getHeader(msg, "Subject"); subject != "" {
		email.Subject = subject
	}

	if contentType := msg.Payload.MimeType; contentType != "" {
		email.ContentType = contentType
	}

	if localId := msg.Id; localId != "" {
		email.LocalId = localId
	}

	if from := parseEmailAddresses(getHeader(msg, "From")); len(from) > 0 {
		email.From = from
		email.Sender = from[0]
	}

	if to := parseEmailAddresses(getHeader(msg, "To")); len(to) > 0 {
		email.To = to
	}

	if cc := parseEmailAddresses(getHeader(msg, "Cc")); len(cc) > 0 {
		email.Cc = cc
	}

	if bcc := parseEmailAddresses(getHeader(msg, "Bcc")); len(bcc) > 0 {
		email.Bcc = bcc
	}

	if replyTo := parseEmailAddresses(getHeader(msg, "Reply-To")); len(replyTo) > 0 {
		email.ReplyTo = replyTo
	}

	if date := getHeader(msg, "Date"); date != "" {
		if t, err := mail.ParseDate(date); err == nil {
			email.OriginationTimestamp = t.UTC().Format(timestampFormat)
		}
	}

	if msg.InternalDate != "" {
		if millis, err := strconv.ParseInt(msg.InternalDate, 10, 64); err == nil {
			email.DeliveryTimestamp = time.UnixMilli(millis).UTC().Format(timestampFormat)
		}
	}

	if deliveryTimestamp := email.DeliveryTimestamp; deliveryTimestamp != "" {
		base.Timestamp = email.DeliveryTimestamp
	}

	if attachments := collectAttachments(msg.Payload); len(attachments) > 0 {
		email.Attachments = attachments
	}
}
