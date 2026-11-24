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

// Each client header is recorded where it is on one line and not at all where it
// is on two, a load balancer that sets one replacing what the client sent leaving
// one. X-Client-Geo-Location is not recorded at all, the load balancer not setting
// it.
func TestExtractor_Handle_ClientHeaderLines(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		header string
		value  string
		field  string
		lines  int
		// expected is the field's value, or the empty string for its absence.
		expected string
	}{
		{name: "client port, one line", header: "X-Client-Port", value: "1111", field: "client.port", lines: 1, expected: "1111"},
		{name: "client port, two lines", header: "X-Client-Port", value: "1111", field: "client.port", lines: 2},
		{name: "server port, one line", header: "X-Server-Port", value: "443", field: "server.port", lines: 1, expected: "443"},
		{name: "server port, two lines", header: "X-Server-Port", value: "443", field: "server.port", lines: 2},
		{name: "country, one line", header: "X-Client-Geo-Country-Iso-Code", value: "SE", field: "client.geo.country_iso_code", lines: 1, expected: "SE"},
		{name: "country, two lines", header: "X-Client-Geo-Country-Iso-Code", value: "SE", field: "client.geo.country_iso_code", lines: 2},
		{name: "region, one line", header: "X-Client-Geo-Region-Iso-Code", value: "SE-AB", field: "client.geo.region_iso_code", lines: 1, expected: "SE-AB"},
		{name: "region, two lines", header: "X-Client-Geo-Region-Iso-Code", value: "SE-AB", field: "client.geo.region_iso_code", lines: 2},
		{name: "city, one line", header: "X-Client-Geo-City-Name", value: "Stockholm", field: "client.geo.city_name", lines: 1, expected: "Stockholm"},
		{name: "city, two lines", header: "X-Client-Geo-City-Name", value: "Stockholm", field: "client.geo.city_name", lines: 2},
		{name: "ja3, one line", header: "X-Tls-Ja3-Fingerprint", value: "ja3-marker", field: "tls.client.ja3", lines: 1, expected: "ja3-marker"},
		{name: "ja3, two lines", header: "X-Tls-Ja3-Fingerprint", value: "ja3-marker", field: "tls.client.ja3", lines: 2},
		{name: "ja4, one line", header: "X-Tls-Ja4-Fingerprint", value: "ja4-marker", field: "tls.client.ja4", lines: 1, expected: "ja4-marker"},
		{name: "ja4, two lines", header: "X-Tls-Ja4-Fingerprint", value: "ja4-marker", field: "tls.client.ja4", lines: 2},
		{name: "location, one line", header: "X-Client-Geo-Location", value: "59.33,18.07", field: "client.geo.location.lat", lines: 1},
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
			request.Header.Set("X-Client-Ip-Address", "192.0.2.3")
			for range testCase.lines {
				request.Header.Add(testCase.header, testCase.value)
			}

			ctx := altshiftHttpContext.WithHttpContextValue(context.Background(), &altshiftHttpTypes.HttpContext{
				Request:            request,
				ClientAddress:      "192.0.2.3",
				TrustClientHeaders: true,
			})

			record := &slog.Record{}
			if err := (&Extractor{}).Handle(ctx, record); err != nil {
				t.Fatalf("handle: %v", err)
			}

			fields := recordFields(record)
			if fields["client.ip"] != "192.0.2.3" {
				t.Fatalf("client.ip = %q, want %q", fields["client.ip"], "192.0.2.3")
			}

			got, ok := fields[testCase.field]
			if testCase.expected == "" {
				if ok {
					t.Errorf("%s = %q, want it absent", testCase.field, got)
				}
				return
			}
			if !ok || got != testCase.expected {
				t.Errorf("%s = %q (present %t), want %q", testCase.field, got, ok, testCase.expected)
			}
		})
	}
}
