package log

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/url"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"time"

	context2 "github.com/altshiftab/utils_go/pkg/context"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftStrings "github.com/altshiftab/utils_go/pkg/strings"
)

type ContextExtractor interface {
	Handle(context.Context, *slog.Record) error
}

type ContextExtractorFunction func(context.Context, *slog.Record) error

func (cef ContextExtractorFunction) Handle(ctx context.Context, record *slog.Record) error {
	return cef(ctx, record)
}

type ContextHandler struct {
	Next       slog.Handler
	Extractors []ContextExtractor
}

func (contextHandler *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	contextHandler.Next = contextHandler.Next.WithAttrs(attrs)
	return contextHandler
}

func (contextHandler *ContextHandler) WithGroup(name string) slog.Handler {
	contextHandler.Next = contextHandler.Next.WithGroup(name)
	return contextHandler
}

func (contextHandler *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return contextHandler.Next.Enabled(ctx, level)
}

func (contextHandler *ContextHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, extractor := range contextHandler.Extractors {
		if extractor != nil {
			if err := extractor.Handle(ctx, &record); err != nil {
				return fmt.Errorf("extractor handle: %w", err)
			}
		}
	}

	return contextHandler.Next.Handle(ctx, record)
}

type ErrorContextExtractor struct {
	SkipCause         bool
	SkipInput         bool
	SkipStackTrace    bool
	SkipOutput        bool
	ContextExtractors []ContextExtractor
}

func (extractor *ErrorContextExtractor) MakeErrorAttrs(err error) []any {
	if err == nil {
		return nil
	}

	errorMessage := err.Error()
	errType := reflect.TypeOf(err).String()

	var attrs []any

	switch err.(type) { //nolint:errorlint // Deliberate: attrs describe this exact error; wrapped causes are handled level-by-level via CollectWrappedErrors.
	case *altshiftErrors.Error:
		break
	case *altshiftErrors.ExtendedError:
		break
	default:
		switch errType {
		case "*errors.errorString", "*fmt.wrapError":
			break
		default:
			attrs = append(attrs, slog.String("type", errType))
		}
	}

	if inputError, ok := err.(altshiftErrors.InputErrorI); ok && !extractor.SkipInput { //nolint:errorlint // Deliberate: attrs describe this exact error, not its wrapped causes.
		if input := inputError.GetInput(); input != nil {
			var inputSlice []any
			var typeName string
			switch typedInput := input.(type) {
			case []any:
				inputSlice = typedInput
			default:
				inputSlice = []any{typedInput}
				if t := reflect.TypeOf(input); t != nil {
					typeName = t.String()
				}
			}

			var textualRepresentations []string

			for _, inputElement := range inputSlice {
				textualRepresentation, err := altshiftStrings.MakeTextualRepresentation(inputElement)
				if err != nil {
					slog.Error(
						fmt.Sprintf(
							"An error occurred when making a textual representation of error input: %v",
							fmt.Errorf("make textual representation: %w", err),
						),
					)
					continue
				}

				textualRepresentations = append(textualRepresentations, textualRepresentation)
			}

			if len(textualRepresentations) != 0 {
				var logValue any = textualRepresentations
				if len(textualRepresentations) == 1 {
					logValue = textualRepresentations[0]
				}

				logArgs := []any{slog.Any("value", logValue)}
				if typeName != "" {
					logArgs = append(logArgs, slog.String("type", typeName))
				}

				attrs = append(attrs, slog.Group("input", logArgs...))
			}
		}
	}

	if !extractor.SkipCause {
		wrappedErrors := altshiftErrors.CollectWrappedErrors(err)
		var lastWrappedErrorAttrs []any

		for i := len(wrappedErrors) - 1; i >= 0; i-- {
			wrappedError := wrappedErrors[i]
			if wrappedError == nil {
				continue
			}

			switch reflect.TypeOf(wrappedError).String() {
			case "*errors.joinError", "*fmt.wrapError":
				continue
			}

			wrappedErrorAttrs := extractor.MakeErrorAttrs(wrappedError)

			if lastWrappedErrorAttrs != nil {
				wrappedErrorAttrs = append(
					wrappedErrorAttrs,
					slog.Group("cause", lastWrappedErrorAttrs...),
				)
			}

			lastWrappedErrorAttrs = wrappedErrorAttrs
		}

		if lastWrappedErrorAttrs != nil {
			if errType == "*errors.joinError" {
				return lastWrappedErrorAttrs
			}
			attrs = append(attrs, slog.Group("cause", lastWrappedErrorAttrs...))
		}
	}

	if codeError, ok := err.(altshiftErrors.CodeErrorI); ok { //nolint:errorlint // Deliberate: attrs describe this exact error, not its wrapped causes.
		if code := codeError.GetCode(); code != "" {
			attrs = append(attrs, slog.String("code", code))
		}
	}

	if idError, ok := err.(altshiftErrors.IdErrorI); ok { //nolint:errorlint // Deliberate: attrs describe this exact error, not its wrapped causes.
		if id := idError.GetId(); id != "" {
			attrs = append(attrs, slog.String("id", id))
		}
	}

	if stackTraceError, ok := err.(altshiftErrors.StackTraceErrorI); ok && !extractor.SkipStackTrace { //nolint:errorlint // Deliberate: attrs describe this exact error, not its wrapped causes.
		if stackTrace := stackTraceError.GetStackTrace(); stackTrace != "" {
			attrs = append(attrs, slog.String("stack_trace", stackTrace))
		}
	}

	if execExitError, ok := err.(*exec.ExitError); ok { //nolint:errorlint // Deliberate: attrs describe this exact error, not its wrapped causes.
		exitCode := execExitError.ExitCode()
		if exitCode != 0 {
			attrs = append(attrs, slog.String("code", strconv.Itoa(exitCode)))
		}

		if stderr := execExitError.Stderr; len(stderr) != 0 && !extractor.SkipOutput {
			attrs = append(
				attrs,
				slog.Group(
					"output",
					slog.String("stderr", string(stderr)),
					slog.String("type", "stderr"),
				),
			)
		}
	}

	if errorMessage != "" {
		attrs = append(attrs, slog.String("message", errorMessage))
	}

	return attrs
}

func (extractor *ErrorContextExtractor) Handle(ctx context.Context, record *slog.Record) error {
	if record == nil {
		return nil
	}

	if logErr, ok := ctx.Value(context2.ErrorContextKey).(error); ok {
		record.Add(slog.Group("error", extractor.MakeErrorAttrs(logErr)...))

		if contextErr, ok := errors.AsType[altshiftErrors.ContextErrorI](logErr); ok {
			if contextErrCtxPtr := contextErr.GetContext(); contextErrCtxPtr != nil {
				contextErrCtx := *contextErrCtxPtr

				var metadataRecord slog.Record

				for _, contextExtractor := range extractor.ContextExtractors {
					if err := contextExtractor.Handle(contextErrCtx, &metadataRecord); err != nil { //nolint:contextcheck // Deliberate: the context attached to the error is the one to extract metadata from.
						return fmt.Errorf("context extractor handle: %w", err)
					}

					var attrs []any
					metadataRecord.Attrs(
						func(attr slog.Attr) bool {
							attrs = append(attrs, attr)
							return true
						},
					)

					if len(attrs) > 0 {
						record.Add(slog.Group("error", slog.Group("context", attrs...)))
						break
					}
				}
			}
		}

		if opErr, ok := errors.AsType[*net.OpError](logErr); ok {
			var contextAttrs []any

			if source := opErr.Source; source != nil {
				if clientAttrs := makeNetAddrAttrs(source); len(clientAttrs) > 0 {
					contextAttrs = append(contextAttrs, slog.Group("client", clientAttrs...))
				}
			}

			if addr := opErr.Addr; addr != nil {
				if serverAttrs := makeNetAddrAttrs(addr); len(serverAttrs) > 0 {
					contextAttrs = append(contextAttrs, slog.Group("server", serverAttrs...))
				}
			}

			if networkAttrs := makeNetworkAttrs(opErr); len(networkAttrs) > 0 {
				contextAttrs = append(contextAttrs, slog.Group("network", networkAttrs...))
			}

			if len(contextAttrs) > 0 {
				record.Add(slog.Group("error", slog.Group("context", contextAttrs...)))
			}
		}

		if dnsErr, ok := errors.AsType[*net.DNSError](logErr); ok {
			var contextAttrs []any

			if name := dnsErr.Name; name != "" {
				contextAttrs = append(contextAttrs, slog.Group("dns", slog.Group("question", slog.String("name", name))))
			}

			if server := dnsErr.Server; server != "" {
				contextAttrs = append(contextAttrs, slog.Group("server", slog.String("address", server)))
			}

			if len(contextAttrs) > 0 {
				record.Add(slog.Group("error", slog.Group("context", contextAttrs...)))
			}
		}

		if urlErr, ok := errors.AsType[*url.Error](logErr); ok {
			var contextAttrs []any

			if urlStr := urlErr.URL; urlStr != "" {
				contextAttrs = append(contextAttrs, slog.Group("url", slog.String("original", urlStr)))
			}

			if op := urlErr.Op; op != "" {
				contextAttrs = append(contextAttrs, slog.Group("http", slog.Group("request", slog.String("method", strings.ToUpper(op)))))
			}

			if len(contextAttrs) > 0 {
				record.Add(slog.Group("error", slog.Group("context", contextAttrs...)))
			}
		}

		if pathErr, ok := errors.AsType[*fs.PathError](logErr); ok {
			if path := pathErr.Path; path != "" {
				record.Add(slog.Group("error", slog.Group("context", slog.Group("file", slog.String("path", path)))))
			}
		}

		if certVerErr, ok := errors.AsType[*tls.CertificateVerificationError](logErr); ok {
			if certs := certVerErr.UnverifiedCertificates; len(certs) > 0 {
				if serverAttrs := makeTlsCertAttrs(certs[0]); len(serverAttrs) > 0 {
					record.Add(slog.Group("error", slog.Group("context", slog.Group("tls", slog.Group("server", serverAttrs...)))))
				}
			}
		}

		if hostErr, ok := errors.AsType[*x509.HostnameError](logErr); ok {
			var tlsAttrs []any

			if host := hostErr.Host; host != "" {
				tlsAttrs = append(tlsAttrs, slog.Group("client", slog.String("server_name", host)))
			}

			if serverAttrs := makeTlsCertAttrs(hostErr.Certificate); len(serverAttrs) > 0 {
				tlsAttrs = append(tlsAttrs, slog.Group("server", serverAttrs...))
			}

			if len(tlsAttrs) > 0 {
				record.Add(slog.Group("error", slog.Group("context", slog.Group("tls", tlsAttrs...))))
			}
		}
	}

	return nil
}

func makeNetAddrAttrs(addr net.Addr) []any {
	switch typedAddr := addr.(type) {
	case *net.TCPAddr:
		return []any{
			slog.String("ip", typedAddr.IP.String()),
			slog.Int("port", typedAddr.Port),
		}
	case *net.UDPAddr:
		return []any{
			slog.String("ip", typedAddr.IP.String()),
			slog.Int("port", typedAddr.Port),
		}
	default:
		return []any{slog.String("address", addr.String())}
	}
}

func makeTlsCertAttrs(cert *x509.Certificate) []any {
	if cert == nil {
		return nil
	}

	var attrs []any

	if subject := cert.Subject.String(); subject != "" {
		attrs = append(attrs, slog.String("subject", subject))
	}

	if issuer := cert.Issuer.String(); issuer != "" {
		attrs = append(attrs, slog.String("issuer", issuer))
	}

	if notAfter := cert.NotAfter; !notAfter.IsZero() {
		attrs = append(attrs, slog.String("not_after", notAfter.UTC().Format(time.RFC3339Nano)))
	}

	if notBefore := cert.NotBefore; !notBefore.IsZero() {
		attrs = append(attrs, slog.String("not_before", notBefore.UTC().Format(time.RFC3339Nano)))
	}

	return attrs
}

func makeNetworkAttrs(opErr *net.OpError) []any {
	netStr := opErr.Net

	var transport string
	var ianaNumber string

	if strings.HasPrefix(netStr, "tcp") {
		transport = "tcp"
		ianaNumber = "6"
	} else if strings.HasPrefix(netStr, "udp") {
		transport = "udp"
		ianaNumber = "17"
	}

	var networkType string
	if strings.HasSuffix(netStr, "4") {
		networkType = "ipv4"
	} else if strings.HasSuffix(netStr, "6") {
		networkType = "ipv6"
	} else {
		for _, addr := range []net.Addr{opErr.Source, opErr.Addr} {
			if addr == nil {
				continue
			}
			var ip net.IP
			switch typedAddr := addr.(type) {
			case *net.TCPAddr:
				ip = typedAddr.IP
			case *net.UDPAddr:
				ip = typedAddr.IP
			}
			if ip != nil {
				if ip.To4() != nil {
					networkType = "ipv4"
				} else {
					networkType = "ipv6"
				}
				break
			}
		}
	}

	var attrs []any
	if transport != "" {
		attrs = append(attrs, slog.String("transport", transport))
	}
	if ianaNumber != "" {
		attrs = append(attrs, slog.String("iana_number", ianaNumber))
	}
	if networkType != "" {
		attrs = append(attrs, slog.String("type", networkType))
	}

	return attrs
}

func AttrsFromMap(m map[string]any) []any {
	var attrs []any
	for key, value := range m {
		if stringAnyMap, ok := value.(map[string]any); ok {
			attrs = append(attrs, slog.Group(key, AttrsFromMap(stringAnyMap)...))
		} else {
			attrs = append(attrs, slog.Any(key, value))
		}
	}
	return attrs
}
