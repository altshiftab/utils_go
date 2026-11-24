package http_context_extractor

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	altshiftHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

// recordFields returns every attribute of a record by its dotted path, rendered as a string.
func recordFields(record *slog.Record) map[string]string {
	fields := make(map[string]string)

	var walk func(prefix string, attr slog.Attr)
	walk = func(prefix string, attr slog.Attr) {
		key := attr.Key
		if prefix != "" {
			key = prefix + "." + key
		}

		value := attr.Value.Resolve()
		if value.Kind() == slog.KindGroup {
			for _, inner := range value.Group() {
				walk(key, inner)
			}
			return
		}
		fields[key] = value.String()
	}

	record.Attrs(func(attr slog.Attr) bool {
		walk("", attr)
		return true
	})

	return fields
}

// The headers a load balancer sets of the client are recorded only where the
// HTTP context says it is trusted to have set them, and never over what the
// connection says of its own ends.
func TestExtractor_Handle_ClientHeaders(t *testing.T) {
	t.Parallel()

	clientHeaders := map[string]string{
		"X-Client-Port":                 "1111",
		"X-Server-Port":                 "443",
		"X-Client-Geo-Country-Iso-Code": "SE",
		"X-Client-Geo-City-Name":        "Stockholm",
		"X-Tls-Ja3-Fingerprint":         "ja3-marker",
		"X-Tls-Ja4-Fingerprint":         "ja4-marker",
	}

	testCases := []struct {
		name          string
		trusted       bool
		clientAddress string
		expected      map[string]string
		absent        []string
	}{
		{
			name: "untrusted",
			expected: map[string]string{
				"client.ip":        "203.0.113.9",
				"client.port":      "54321",
				"source.port":      "54321",
				"server.port":      "8080",
				"destination.port": "8080",
			},
			absent: []string{"client.geo.country_iso_code", "client.geo.city_name", "tls.client.ja3", "tls.client.ja4"},
		},
		{
			name:    "trusted, the client being the connection's peer",
			trusted: true,
			expected: map[string]string{
				"client.ip":                   "203.0.113.9",
				"client.port":                 "1111",
				"client.geo.country_iso_code": "SE",
				"client.geo.city_name":        "Stockholm",
				"tls.client.ja3":              "ja3-marker",
				"tls.client.ja4":              "ja4-marker",
				"source.ip":                   "203.0.113.9",
				"source.port":                 "54321",
				"server.port":                 "443",
				"destination.port":            "8080",
			},
			absent: []string{"source.geo.country_iso_code"},
		},
		{
			name:          "trusted, the client resolved",
			trusted:       true,
			clientAddress: "192.0.2.3",
			expected: map[string]string{
				"client.ip":                   "192.0.2.3",
				"client.port":                 "1111",
				"client.geo.country_iso_code": "SE",
				"source.ip":                   "203.0.113.9",
				"source.port":                 "54321",
				"server.port":                 "443",
				"destination.port":            "8080",
			},
			absent: []string{"source.geo.country_iso_code"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequestWithContext(
				context.WithValue(t.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("10.0.0.2"), Port: 8080}),
				http.MethodGet,
				"http://service.example.com/",
				nil,
			)
			request.RemoteAddr = "203.0.113.9:54321"
			for name, value := range clientHeaders {
				request.Header.Set(name, value)
			}

			ctx := altshiftHttpContext.WithHttpContextValue(context.Background(), &altshiftHttpTypes.HttpContext{
				Request:            request,
				ClientAddress:      testCase.clientAddress,
				TrustClientHeaders: testCase.trusted,
			})

			record := &slog.Record{}
			if err := (&Extractor{}).Handle(ctx, record); err != nil {
				t.Fatalf("handle: %v", err)
			}

			fields := recordFields(record)
			for key, expected := range testCase.expected {
				if got, ok := fields[key]; !ok || got != expected {
					t.Errorf("%s = %q (present %t), want %q", key, got, ok, expected)
				}
			}
			for _, key := range testCase.absent {
				if got, ok := fields[key]; ok {
					t.Errorf("%s = %q, want it absent", key, got)
				}
			}
		})
	}
}
