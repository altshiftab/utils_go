package content_security_policy

import (
	"net/url"
	"reflect"
	"testing"
)

func TestContentSecurityPolicy_String_Directives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		directive DirectiveI
		want      string
	}{
		{
			name: "default-src with keyword, scheme and host",
			directive: &DefaultSrcDirective{SourceDirective: SourceDirective{Sources: []SourceI{
				&KeywordSource{Keyword: "self"},
				&SchemeSource{Scheme: "https"},
				&HostSource{Host: "cdn.example.com"},
			}}},
			want: "default-src 'self' https: cdn.example.com",
		},
		{
			name: "script-src with keywords, schemes, nonce and hash",
			directive: &ScriptSrcDirective{SourceDirective: SourceDirective{Sources: []SourceI{
				&KeywordSource{Keyword: "unsafe-inline"},
				&KeywordSource{Keyword: "unsafe-eval"},
				&KeywordSource{Keyword: "strict-dynamic"},
				&SchemeSource{Scheme: "https"},
				&SchemeSource{Scheme: "http"},
				&NonceSource{Base64Value: "dGVzdA=="},
				&HashSource{HashAlgorithm: "sha256", Base64Value: "AbCd012+/_-=="},
			}}},
			want: "script-src 'unsafe-inline' 'unsafe-eval' 'strict-dynamic' https: http: 'nonce-dGVzdA==' 'sha256-AbCd012+/_-=='",
		},
		{
			name: "style-src with report-sample and host",
			directive: &StyleSrcDirective{SourceDirective: SourceDirective{Sources: []SourceI{
				&KeywordSource{Keyword: "report-sample"},
				&HostSource{Scheme: "https", Host: "styles.example.com"},
			}}},
			want: "style-src 'report-sample' https://styles.example.com",
		},
		{
			name: "img-src with host and data scheme",
			directive: &ImgSrcDirective{SourceDirective: SourceDirective{Sources: []SourceI{
				&HostSource{Scheme: "https", Host: "example.com", PortString: "443", Path: "/path"},
				&SchemeSource{Scheme: "data"},
			}}},
			want: "img-src https://example.com:443/path data:",
		},
		{
			name: "connect-src with wildcard port",
			directive: &ConnectSrcDirective{SourceDirective: SourceDirective{Sources: []SourceI{
				&HostSource{Host: "*.example.com", PortString: "*"},
			}}},
			want: "connect-src *.example.com:*",
		},
		{
			name: "frame-ancestors with self and https parent",
			directive: &FrameAncestorsDirective{SourceDirective: SourceDirective{Sources: []SourceI{
				&KeywordSource{Keyword: "self"},
				&HostSource{Scheme: "https", Host: "parent.example.com"},
			}}},
			want: "frame-ancestors 'self' https://parent.example.com",
		},
		{
			name:      "sandbox with tokens",
			directive: &SandboxDirective{Tokens: []string{"allow-same-origin", "allow-scripts"}},
			want:      "sandbox allow-same-origin allow-scripts",
		},
		{
			name:      "sandbox without tokens",
			directive: &SandboxDirective{},
			want:      "sandbox",
		},
		{
			name:      "report-uri with multiple endpoints",
			directive: &ReportUriDirective{UriReferences: []string{"/csp", "/csp2", "https://report.example.com/endpoint"}},
			want:      "report-uri /csp /csp2 https://report.example.com/endpoint",
		},
		{
			name:      "report-to with token",
			directive: &ReportToDirective{Token: "csp-endpoint"}, //nolint:gosec // G101: report-to token label, not a credential
			want:      "report-to csp-endpoint",
		},
		{
			name:      "require-sri-for script",
			directive: &RequireSriForDirective{ResourceTypes: []string{"script"}},
			want:      "require-sri-for script",
		},
		{
			name: "trusted-types with expressions",
			directive: &TrustedTypesDirective{Expressions: []TrustedTypeExpression{
				{Kind: "policy-name", Value: "default"},
				{Kind: "policy-name", Value: "policy1"},
				{Kind: "keyword", Value: "allow-duplicates"},
				{Kind: "keyword", Value: "none"},
			}},
			want: "trusted-types default policy1 'allow-duplicates' 'none'",
		},
		{
			name:      "require-trusted-types-for script",
			directive: &RequireTrustedTypesForDirective{SinkGroups: []string{"script"}},
			want:      "require-trusted-types-for 'script'",
		},
		{
			name:      "upgrade-insecure-requests",
			directive: &UpgradeInsecureRequestsDirective{},
			want:      "upgrade-insecure-requests",
		},
		{
			name:      "webrtc allow",
			directive: &WebrtcDirective{Value: "allow"},
			want:      "webrtc 'allow'",
		},
		{
			name: "base-uri self",
			directive: &BaseUriDirective{SourceDirective: SourceDirective{Sources: []SourceI{
				&KeywordSource{Keyword: "self"},
			}}},
			want: "base-uri 'self'",
		},
		{
			name: "object-src none",
			directive: &ObjectSrcDirective{SourceDirective: SourceDirective{Sources: []SourceI{
				&NoneSource{},
			}}},
			want: "object-src 'none'",
		},
		{
			name: "worker-src data and blob",
			directive: &WorkerSrcDirective{SourceDirective: SourceDirective{Sources: []SourceI{
				&SchemeSource{Scheme: "data"},
				&SchemeSource{Scheme: "blob"},
			}}},
			want: "worker-src data: blob:",
		},
		{
			name: "child-src https host",
			directive: &ChildSrcDirective{SourceDirective: SourceDirective{Sources: []SourceI{
				&HostSource{Scheme: "https", Host: "child.example.com"},
			}}},
			want: "child-src https://child.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			csp := &ContentSecurityPolicy{Directives: []DirectiveI{tt.directive}}
			if got := csp.String(); got != tt.want {
				t.Errorf("String() got = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContentSecurityPolicy_GetDirective(t *testing.T) {
	t.Parallel()

	type fields struct {
		Directives            []DirectiveI
		OtherDirectives       []DirectiveI
		IneffectiveDirectives []DirectiveI
		Raw                   string
	}
	type args struct {
		name string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   DirectiveI
		want1  bool
	}{
		{
			name: "found in Directives",
			fields: fields{
				Directives: []DirectiveI{
					&DefaultSrcDirective{SourceDirective: SourceDirective{}},
				},
			},
			args:  args{name: "default-src"},
			want:  &DefaultSrcDirective{SourceDirective: SourceDirective{}},
			want1: true,
		},
		{
			name: "found in OtherDirectives",
			fields: fields{
				OtherDirectives: []DirectiveI{
					&ParsedDirective{Name: "foo", Value: "bar"},
				},
			},
			args:  args{name: "foo"},
			want:  &ParsedDirective{Name: "foo", Value: "bar"},
			want1: true,
		},
		{
			name:   "not found",
			fields: fields{},
			args:   args{name: "nope"},
			want:   nil,
			want1:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			csp := &ContentSecurityPolicy{
				Directives:            tt.fields.Directives,
				OtherDirectives:       tt.fields.OtherDirectives,
				IneffectiveDirectives: tt.fields.IneffectiveDirectives,
				Raw:                   tt.fields.Raw,
			}
			got, got1 := csp.GetDirective(tt.args.name)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetDirective() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("GetDirective() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestContentSecurityPolicy_String_SpecificPolicy(t *testing.T) {
	t.Parallel()

	expected := "default-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'; require-trusted-types-for 'script'; trusted-types lit-html"

	csp := &ContentSecurityPolicy{
		Directives: []DirectiveI{
			&DefaultSrcDirective{SourceDirective: SourceDirective{Sources: []SourceI{&KeywordSource{Keyword: "self"}}}},
			&FrameAncestorsDirective{SourceDirective: SourceDirective{Sources: []SourceI{&NoneSource{}}}},
			&BaseUriDirective{SourceDirective: SourceDirective{Sources: []SourceI{&NoneSource{}}}},
			&FormActionDirective{SourceDirective: SourceDirective{Sources: []SourceI{&NoneSource{}}}},
			&RequireTrustedTypesForDirective{SinkGroups: []string{"script"}},
			&TrustedTypesDirective{
				Expressions: []TrustedTypeExpression{
					{Kind: "policy-name", Value: "lit-html"},
				},
			},
		},
	}

	if got := csp.String(); got != expected {
		to := got
		want := expected
		t.Errorf("String() got = %q, want %q", to, want)
	}
}

func TestHostSourceFromUrl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		url  string
		want *HostSource
		// rendered is the source as written; parsed says whether the CSP grammar reads it back, which
		// it does not for an IPv6 literal.
		rendered string
		parsed   bool
	}{
		{
			name: "no port", url: "https://login.vvvp.se",
			want:     &HostSource{Scheme: "https", Host: "login.vvvp.se"},
			rendered: "https://login.vvvp.se", parsed: true,
		},
		{
			name: "the scheme's default port", url: "https://login.vvvp.se:443",
			want:     &HostSource{Scheme: "https", Host: "login.vvvp.se", PortString: "443"},
			rendered: "https://login.vvvp.se:443", parsed: true,
		},
		{
			name: "another port", url: "http://localhost:8081",
			want:     &HostSource{Scheme: "http", Host: "localhost", PortString: "8081"},
			rendered: "http://localhost:8081", parsed: true,
		},
		{
			name: "a port and a path", url: "https://api.example.test:8443/v1/",
			want:     &HostSource{Scheme: "https", Host: "api.example.test", PortString: "8443", Path: "/v1/"},
			rendered: "https://api.example.test:8443/v1/", parsed: true,
		},
		{
			name: "an empty port", url: "https://example.test:/x",
			want:     &HostSource{Scheme: "https", Host: "example.test", Path: "/x"},
			rendered: "https://example.test/x", parsed: true,
		},
		{
			name: "an IPv4 address and a port", url: "http://127.0.0.1:8080",
			want:     &HostSource{Scheme: "http", Host: "127.0.0.1", PortString: "8080"},
			rendered: "http://127.0.0.1:8080", parsed: true,
		},
		{
			name: "an IPv6 literal and a port", url: "http://[::1]:8081",
			want:     &HostSource{Scheme: "http", Host: "[::1]", PortString: "8081"},
			rendered: "http://[::1]:8081",
		},
		{
			name: "an IPv6 literal", url: "https://[2001:db8::1]",
			want:     &HostSource{Scheme: "https", Host: "[2001:db8::1]"},
			rendered: "https://[2001:db8::1]",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			parsedUrl, err := url.Parse(testCase.url)
			if err != nil {
				t.Fatal(err)
			}
			got := HostSourceFromUrl(parsedUrl)
			if got == nil || !reflect.DeepEqual(*got, *testCase.want) {
				t.Fatalf("HostSourceFromUrl(%q) = %+v, want %+v", testCase.url, got, testCase.want)
			}
			if rendered := got.String(); rendered != testCase.rendered {
				t.Errorf("String() = %q, want %q", rendered, testCase.rendered)
			}
			if !testCase.parsed {
				return
			}

			// What it writes is what the parser reads back: the same host, the same port.
			policy, err := Parse([]byte("connect-src " + testCase.rendered))
			if err != nil {
				t.Fatalf("parse %q: %v", testCase.rendered, err)
			}
			directive, ok := policy.GetDirective("connect-src")
			if !ok {
				t.Fatal("no connect-src")
			}
			sourceDirective, ok := directive.(interface{ GetSources() []SourceI })
			if !ok || len(sourceDirective.GetSources()) != 1 {
				t.Fatalf("sources %+v", directive)
			}
			read, ok := sourceDirective.GetSources()[0].(*HostSource)
			if !ok || read.Scheme != got.Scheme || read.Host != got.Host || read.PortString != got.PortString || read.Path != got.Path {
				t.Errorf("read back %+v, made %+v", read, got)
			}
		})
	}

	if HostSourceFromUrl(nil) != nil {
		t.Error("a host source of no URL")
	}
}
