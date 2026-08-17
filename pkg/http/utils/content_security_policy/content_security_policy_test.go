package content_security_policy

import (
	"net/url"
	"slices"
	"testing"

	csp "github.com/altshiftab/utils_go/pkg/http/types/content_security_policy"
)

func sourceStrings(sources []csp.SourceI) []string {
	var result []string
	for _, s := range sources {
		result = append(result, s.String())
	}
	return result
}

func mustParseURL(rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return u
}

// --- buildHostSources ---

func TestBuildHostSources_NilUrls(t *testing.T) {
	t.Parallel()

	result := buildHostSources(nil, nil)
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %d sources", len(result))
	}
}

func TestBuildHostSources_ValidUrls(t *testing.T) {
	t.Parallel()

	u := mustParseURL("https://example.com")
	result := buildHostSources(u)
	if len(result) != 1 {
		t.Fatalf("expected 1 source, got %d", len(result))
	}
	if result[0].String() != "https://example.com" {
		t.Fatalf("expected 'https://example.com', got %q", result[0].String())
	}
}

func TestBuildHostSources_MixedNilAndValid(t *testing.T) {
	t.Parallel()

	u := mustParseURL("https://cdn.example.com")
	result := buildHostSources(nil, u, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 source, got %d", len(result))
	}
}

func TestBuildHostSources_NoArgs(t *testing.T) {
	t.Parallel()

	result := buildHostSources()
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %d sources", len(result))
	}
}

// --- PatchCspConnectSrcWithHostSrc ---

func TestPatchCspConnectSrcWithHostSrc_NilCsp(t *testing.T) {
	t.Parallel()

	PatchCspConnectSrcWithHostSrc(nil, mustParseURL("https://example.com"))
	// Should not panic.
}

func TestPatchCspConnectSrcWithHostSrc_NoUrls(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspConnectSrcWithHostSrc(policy)
	if len(policy.Directives) != 0 {
		t.Fatalf("expected no directives, got %d", len(policy.Directives))
	}
}

func TestPatchCspConnectSrcWithHostSrc_AllNilUrls(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspConnectSrcWithHostSrc(policy, nil, nil)
	if len(policy.Directives) != 0 {
		t.Fatalf("expected no directives, got %d", len(policy.Directives))
	}
}

func TestPatchCspConnectSrcWithHostSrc_AddsNewDirective(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspConnectSrcWithHostSrc(policy, mustParseURL("https://api.example.com"))

	directive := policy.GetConnectSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if directive == nil {
		t.Fatal("expected connect-src directive to be added")
	}

	sources := sourceStrings(directive.Sources)
	if !slices.Contains(sources, "'self'") {
		t.Error("expected 'self' source")
	}
	if !slices.Contains(sources, "https://api.example.com") {
		t.Errorf("expected 'https://api.example.com' source, got %v", sources)
	}
}

func TestPatchCspConnectSrcWithHostSrc_AppendsToExisting(t *testing.T) {
	t.Parallel()

	existingDirective := &csp.ConnectSrcDirective{
		SourceDirective: csp.SourceDirective{
			Sources: []csp.SourceI{
				&csp.KeywordSource{Keyword: "self"},
				&csp.HostSource{Scheme: "https", Host: "existing.example.com"},
			},
		},
	}
	policy := &csp.ContentSecurityPolicy{
		Directives: []csp.DirectiveI{existingDirective},
	}

	PatchCspConnectSrcWithHostSrc(policy, mustParseURL("https://new.example.com"))

	directive := policy.GetConnectSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	sources := sourceStrings(directive.Sources)
	if len(sources) != 3 {
		t.Fatalf("expected 3 sources, got %d: %v", len(sources), sources)
	}
	if !slices.Contains(sources, "https://new.example.com") {
		t.Errorf("expected 'https://new.example.com', got %v", sources)
	}
}

func TestPatchCspConnectSrcWithHostSrc_NoDuplicates(t *testing.T) {
	t.Parallel()

	existingDirective := &csp.ConnectSrcDirective{
		SourceDirective: csp.SourceDirective{
			Sources: []csp.SourceI{
				&csp.KeywordSource{Keyword: "self"},
				&csp.HostSource{Scheme: "https", Host: "api.example.com"},
			},
		},
	}
	policy := &csp.ContentSecurityPolicy{
		Directives: []csp.DirectiveI{existingDirective},
	}

	PatchCspConnectSrcWithHostSrc(policy, mustParseURL("https://api.example.com"))

	directive := policy.GetConnectSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if len(directive.Sources) != 2 {
		t.Fatalf("expected 2 sources (no duplicate), got %d: %v", len(directive.Sources), sourceStrings(directive.Sources))
	}
}

func TestPatchCspConnectSrcWithHostSrc_MultipleUrls(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspConnectSrcWithHostSrc(policy,
		mustParseURL("https://api1.example.com"),
		mustParseURL("https://api2.example.com"),
	)

	directive := policy.GetConnectSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if directive == nil {
		t.Fatal("expected connect-src directive")
	}
	sources := sourceStrings(directive.Sources)
	if len(sources) != 3 {
		t.Fatalf("expected 3 sources ('self' + 2 hosts), got %d: %v", len(sources), sources)
	}
}

// --- PatchCspFrameSrcWithHostSrc ---

func TestPatchCspFrameSrcWithHostSrc_NilCsp(t *testing.T) {
	t.Parallel()

	PatchCspFrameSrcWithHostSrc(nil, mustParseURL("https://example.com"))
}

func TestPatchCspFrameSrcWithHostSrc_NoUrls(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspFrameSrcWithHostSrc(policy)
	if len(policy.Directives) != 0 {
		t.Fatalf("expected no directives, got %d", len(policy.Directives))
	}
}

func TestPatchCspFrameSrcWithHostSrc_AddsNewDirective(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspFrameSrcWithHostSrc(policy, mustParseURL("https://frame.example.com"))

	directive := policy.GetFrameSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if directive == nil {
		t.Fatal("expected frame-src directive to be added")
	}

	sources := sourceStrings(directive.Sources)
	if !slices.Contains(sources, "'self'") {
		t.Error("expected 'self' source")
	}
	if !slices.Contains(sources, "https://frame.example.com") {
		t.Errorf("expected 'https://frame.example.com' source, got %v", sources)
	}
}

func TestPatchCspFrameSrcWithHostSrc_AppendsToExisting(t *testing.T) {
	t.Parallel()

	existingDirective := &csp.FrameSrcDirective{
		SourceDirective: csp.SourceDirective{
			Sources: []csp.SourceI{
				&csp.KeywordSource{Keyword: "self"},
			},
		},
	}
	policy := &csp.ContentSecurityPolicy{
		Directives: []csp.DirectiveI{existingDirective},
	}

	PatchCspFrameSrcWithHostSrc(policy, mustParseURL("https://frame.example.com"))

	directive := policy.GetFrameSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	sources := sourceStrings(directive.Sources)
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d: %v", len(sources), sources)
	}
}

func TestPatchCspFrameSrcWithHostSrc_NoDuplicates(t *testing.T) {
	t.Parallel()

	existingDirective := &csp.FrameSrcDirective{
		SourceDirective: csp.SourceDirective{
			Sources: []csp.SourceI{
				&csp.HostSource{Scheme: "https", Host: "frame.example.com"},
			},
		},
	}
	policy := &csp.ContentSecurityPolicy{
		Directives: []csp.DirectiveI{existingDirective},
	}

	PatchCspFrameSrcWithHostSrc(policy, mustParseURL("https://frame.example.com"))

	directive := policy.GetFrameSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if len(directive.Sources) != 1 {
		t.Fatalf("expected 1 source (no duplicate), got %d", len(directive.Sources))
	}
}

// --- PatchCspImageSrc ---

func TestPatchCspImageSrc_NilCsp(t *testing.T) {
	t.Parallel()

	PatchCspImageSrc(nil, mustParseURL("https://example.com"))
}

func TestPatchCspImageSrc_NoUrls(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspImageSrc(policy)
	if len(policy.Directives) != 0 {
		t.Fatalf("expected no directives, got %d", len(policy.Directives))
	}
}

func TestPatchCspImageSrc_AllNilUrls(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspImageSrc(policy, nil, nil)
	if len(policy.Directives) != 0 {
		t.Fatalf("expected no directives, got %d", len(policy.Directives))
	}
}

func TestPatchCspImageSrc_HostUrl(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspImageSrc(policy, mustParseURL("https://images.example.com"))

	directive := policy.GetImgSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if directive == nil {
		t.Fatal("expected img-src directive to be added")
	}

	sources := sourceStrings(directive.Sources)
	if !slices.Contains(sources, "'self'") {
		t.Error("expected 'self' source")
	}
	if !slices.Contains(sources, "https://images.example.com") {
		t.Errorf("expected host source, got %v", sources)
	}
}

func TestPatchCspImageSrc_DataUrl(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	dataUrl := &url.URL{Scheme: "data"}
	PatchCspImageSrc(policy, dataUrl)

	directive := policy.GetImgSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if directive == nil {
		t.Fatal("expected img-src directive to be added")
	}

	sources := sourceStrings(directive.Sources)
	if !slices.Contains(sources, "'self'") {
		t.Error("expected 'self' source")
	}
	if !slices.Contains(sources, "data:") {
		t.Errorf("expected 'data:' source, got %v", sources)
	}
}

func TestPatchCspImageSrc_MixedDataAndHost(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	dataUrl := &url.URL{Scheme: "data"}
	hostUrl := mustParseURL("https://cdn.example.com")
	PatchCspImageSrc(policy, dataUrl, hostUrl)

	directive := policy.GetImgSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if directive == nil {
		t.Fatal("expected img-src directive to be added")
	}

	sources := sourceStrings(directive.Sources)
	if !slices.Contains(sources, "data:") {
		t.Errorf("expected 'data:' source, got %v", sources)
	}
	if !slices.Contains(sources, "https://cdn.example.com") {
		t.Errorf("expected host source, got %v", sources)
	}
	if !slices.Contains(sources, "'self'") {
		t.Error("expected 'self' source")
	}
}

func TestPatchCspImageSrc_AppendsToExisting(t *testing.T) {
	t.Parallel()

	existingDirective := &csp.ImgSrcDirective{
		SourceDirective: csp.SourceDirective{
			Sources: []csp.SourceI{
				&csp.KeywordSource{Keyword: "self"},
			},
		},
	}
	policy := &csp.ContentSecurityPolicy{
		Directives: []csp.DirectiveI{existingDirective},
	}

	PatchCspImageSrc(policy, mustParseURL("https://images.example.com"))

	directive := policy.GetImgSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	sources := sourceStrings(directive.Sources)
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d: %v", len(sources), sources)
	}
}

func TestPatchCspImageSrc_NoDuplicates(t *testing.T) {
	t.Parallel()

	existingDirective := &csp.ImgSrcDirective{
		SourceDirective: csp.SourceDirective{
			Sources: []csp.SourceI{
				&csp.KeywordSource{Keyword: "self"},
				&csp.SchemeSource{Scheme: "data"},
			},
		},
	}
	policy := &csp.ContentSecurityPolicy{
		Directives: []csp.DirectiveI{existingDirective},
	}

	dataUrl := &url.URL{Scheme: "data"}
	PatchCspImageSrc(policy, dataUrl)

	directive := policy.GetImgSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if len(directive.Sources) != 2 {
		t.Fatalf("expected 2 sources (no duplicate), got %d: %v", len(directive.Sources), sourceStrings(directive.Sources))
	}
}

func TestPatchCspImageSrc_AppendsDataToExistingHost(t *testing.T) {
	t.Parallel()

	existingDirective := &csp.ImgSrcDirective{
		SourceDirective: csp.SourceDirective{
			Sources: []csp.SourceI{
				&csp.KeywordSource{Keyword: "self"},
				&csp.HostSource{Scheme: "https", Host: "images.example.com"},
			},
		},
	}
	policy := &csp.ContentSecurityPolicy{
		Directives: []csp.DirectiveI{existingDirective},
	}

	dataUrl := &url.URL{Scheme: "data"}
	PatchCspImageSrc(policy, dataUrl)

	directive := policy.GetImgSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	sources := sourceStrings(directive.Sources)
	if len(sources) != 3 {
		t.Fatalf("expected 3 sources, got %d: %v", len(sources), sources)
	}
	if !slices.Contains(sources, "data:") {
		t.Errorf("expected 'data:' source, got %v", sources)
	}
}

// --- PatchCspStyleSrcWithNonce ---

func TestPatchCspStyleSrcWithNonce_NilCsp(t *testing.T) {
	t.Parallel()

	PatchCspStyleSrcWithNonce(nil, "abc123")
}

func TestPatchCspStyleSrcWithNonce_NoNonces(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspStyleSrcWithNonce(policy)
	if len(policy.Directives) != 0 {
		t.Fatalf("expected no directives, got %d", len(policy.Directives))
	}
}

func TestPatchCspStyleSrcWithNonce_EmptyNonces(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspStyleSrcWithNonce(policy, "", "")
	if len(policy.Directives) != 0 {
		t.Fatalf("expected no directives, got %d", len(policy.Directives))
	}
}

func TestPatchCspStyleSrcWithNonce_AddsNewDirective(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspStyleSrcWithNonce(policy, "abc123")

	directive := policy.GetStyleSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if directive == nil {
		t.Fatal("expected style-src directive to be added")
	}

	sources := sourceStrings(directive.Sources)
	if !slices.Contains(sources, "'nonce-abc123'") {
		t.Errorf("expected nonce source, got %v", sources)
	}
}

func TestPatchCspStyleSrcWithNonce_MultipleNonces(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspStyleSrcWithNonce(policy, "nonce1", "nonce2")

	directive := policy.GetStyleSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if directive == nil {
		t.Fatal("expected style-src directive to be added")
	}

	sources := sourceStrings(directive.Sources)
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d: %v", len(sources), sources)
	}
}

func TestPatchCspStyleSrcWithNonce_AppendsToExisting(t *testing.T) {
	t.Parallel()

	existingDirective := &csp.StyleSrcDirective{
		SourceDirective: csp.SourceDirective{
			Sources: []csp.SourceI{
				&csp.KeywordSource{Keyword: "self"},
			},
		},
	}
	policy := &csp.ContentSecurityPolicy{
		Directives: []csp.DirectiveI{existingDirective},
	}

	PatchCspStyleSrcWithNonce(policy, "abc123")

	directive := policy.GetStyleSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	sources := sourceStrings(directive.Sources)
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d: %v", len(sources), sources)
	}
	if !slices.Contains(sources, "'nonce-abc123'") {
		t.Errorf("expected nonce source, got %v", sources)
	}
}

func TestPatchCspStyleSrcWithNonce_NoDuplicates(t *testing.T) {
	t.Parallel()

	existingDirective := &csp.StyleSrcDirective{
		SourceDirective: csp.SourceDirective{
			Sources: []csp.SourceI{
				&csp.NonceSource{Base64Value: "abc123"},
			},
		},
	}
	policy := &csp.ContentSecurityPolicy{
		Directives: []csp.DirectiveI{existingDirective},
	}

	PatchCspStyleSrcWithNonce(policy, "abc123")

	directive := policy.GetStyleSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if len(directive.Sources) != 1 {
		t.Fatalf("expected 1 source (no duplicate), got %d: %v", len(directive.Sources), sourceStrings(directive.Sources))
	}
}

// --- PatchCspStyleSrcWithHash ---

func TestPatchCspStyleSrcWithHash_NilCsp(t *testing.T) {
	t.Parallel()

	err := PatchCspStyleSrcWithHash(nil, "sha256-abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPatchCspStyleSrcWithHash_NoValues(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	err := PatchCspStyleSrcWithHash(policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(policy.Directives) != 0 {
		t.Fatalf("expected no directives, got %d", len(policy.Directives))
	}
}

func TestPatchCspStyleSrcWithHash_EmptyValues(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	err := PatchCspStyleSrcWithHash(policy, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(policy.Directives) != 0 {
		t.Fatalf("expected no directives, got %d", len(policy.Directives))
	}
}

func TestPatchCspStyleSrcWithHash_AddsNewDirective(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	err := PatchCspStyleSrcWithHash(policy, "sha256-abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	directive := policy.GetStyleSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if directive == nil {
		t.Fatal("expected style-src directive to be added")
	}

	sources := sourceStrings(directive.Sources)
	if !slices.Contains(sources, "'sha256-abc123'") {
		t.Errorf("expected hash source, got %v", sources)
	}
}

func TestPatchCspStyleSrcWithHash_MultipleHashes(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	err := PatchCspStyleSrcWithHash(policy, "sha256-abc", "sha384-def")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	directive := policy.GetStyleSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	sources := sourceStrings(directive.Sources)
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d: %v", len(sources), sources)
	}
}

func TestPatchCspStyleSrcWithHash_InvalidFormat(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	err := PatchCspStyleSrcWithHash(policy, "nohyphen")
	if err == nil {
		t.Fatal("expected error for value without hyphen separator")
	}
}

func TestPatchCspStyleSrcWithHash_AppendsToExisting(t *testing.T) {
	t.Parallel()

	existingDirective := &csp.StyleSrcDirective{
		SourceDirective: csp.SourceDirective{
			Sources: []csp.SourceI{
				&csp.KeywordSource{Keyword: "self"},
			},
		},
	}
	policy := &csp.ContentSecurityPolicy{
		Directives: []csp.DirectiveI{existingDirective},
	}

	err := PatchCspStyleSrcWithHash(policy, "sha256-abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	directive := policy.GetStyleSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	sources := sourceStrings(directive.Sources)
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d: %v", len(sources), sources)
	}
	if !slices.Contains(sources, "'sha256-abc123'") {
		t.Errorf("expected hash source, got %v", sources)
	}
}

func TestPatchCspStyleSrcWithHash_NoDuplicates(t *testing.T) {
	t.Parallel()

	existingDirective := &csp.StyleSrcDirective{
		SourceDirective: csp.SourceDirective{
			Sources: []csp.SourceI{
				&csp.HashSource{HashAlgorithm: "sha256", Base64Value: "abc123"},
			},
		},
	}
	policy := &csp.ContentSecurityPolicy{
		Directives: []csp.DirectiveI{existingDirective},
	}

	err := PatchCspStyleSrcWithHash(policy, "sha256-abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	directive := policy.GetStyleSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if len(directive.Sources) != 1 {
		t.Fatalf("expected 1 source (no duplicate), got %d: %v", len(directive.Sources), sourceStrings(directive.Sources))
	}
}

// --- PatchCspStyleSrcWithNonce and PatchCspStyleSrcWithHash interaction ---

func TestPatchCspStyleSrc_NonceAndHash_SharedDirective(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}

	PatchCspStyleSrcWithNonce(policy, "nonce1")
	err := PatchCspStyleSrcWithHash(policy, "sha256-hash1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	directive := policy.GetStyleSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if directive == nil {
		t.Fatal("expected style-src directive")
	}

	sources := sourceStrings(directive.Sources)
	if !slices.Contains(sources, "'nonce-nonce1'") {
		t.Errorf("expected nonce source, got %v", sources)
	}
	if !slices.Contains(sources, "'sha256-hash1'") {
		t.Errorf("expected hash source, got %v", sources)
	}
}

// --- Multiple directive types on same policy ---

func TestMultipleDirectives_OnSamePolicy(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}

	PatchCspConnectSrcWithHostSrc(policy, mustParseURL("https://api.example.com"))
	PatchCspFrameSrcWithHostSrc(policy, mustParseURL("https://frame.example.com"))
	PatchCspImageSrc(policy, mustParseURL("https://img.example.com"))
	PatchCspStyleSrcWithNonce(policy, "nonce1")

	if policy.GetConnectSrc() == nil {
		t.Error("expected connect-src directive")
	}
	if policy.GetFrameSrc() == nil {
		t.Error("expected frame-src directive")
	}
	if policy.GetImgSrc() == nil {
		t.Error("expected img-src directive")
	}
	if policy.GetStyleSrc() == nil {
		t.Error("expected style-src directive")
	}

	if len(policy.Directives) != 4 {
		t.Fatalf("expected 4 directives, got %d", len(policy.Directives))
	}
}

// --- PatchCspSourceDirective ---

func TestPatchCspSourceDirective_NilCsp(t *testing.T) {
	t.Parallel()

	PatchCspSourceDirective[csp.WorkerSrcDirective](nil, &csp.KeywordSource{Keyword: "self"})
	// Should not panic.
}

func TestPatchCspSourceDirective_NoSources(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspSourceDirective[csp.WorkerSrcDirective](policy)
	if len(policy.Directives) != 0 {
		t.Fatalf("expected no directives, got %d", len(policy.Directives))
	}
}

func TestPatchCspSourceDirective_AddsNewDirective(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspSourceDirective[csp.WorkerSrcDirective](
		policy,
		&csp.KeywordSource{Keyword: "self"},
		&csp.SchemeSource{Scheme: "blob"},
	)

	directive := policy.GetWorkerSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if directive == nil {
		t.Fatal("expected worker-src directive to be added")
	}
	if got := directive.String(); got != "worker-src 'self' blob:" {
		t.Fatalf("expected \"worker-src 'self' blob:\", got %q", got)
	}
}

func TestPatchCspSourceDirective_ExtendsExisting(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{
		Directives: []csp.DirectiveI{
			&csp.WorkerSrcDirective{
				SourceDirective: csp.SourceDirective{
					Sources: []csp.SourceI{&csp.KeywordSource{Keyword: "self"}},
				},
			},
		},
	}

	PatchCspSourceDirective[csp.WorkerSrcDirective](policy, &csp.SchemeSource{Scheme: "blob"})

	if len(policy.Directives) != 1 {
		t.Fatalf("expected the existing directive to be reused, got %d directives", len(policy.Directives))
	}
	directive := policy.GetWorkerSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if directive == nil {
		t.Fatal("expected worker-src directive")
	}
	if got := directive.String(); got != "worker-src 'self' blob:" {
		t.Fatalf("expected \"worker-src 'self' blob:\", got %q", got)
	}
}

func TestPatchCspSourceDirective_DeduplicatesSources(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspSourceDirective[csp.WorkerSrcDirective](policy, &csp.KeywordSource{Keyword: "self"})
	PatchCspSourceDirective[csp.WorkerSrcDirective](
		policy,
		&csp.KeywordSource{Keyword: "self"},
		&csp.SchemeSource{Scheme: "blob"},
	)

	directive := policy.GetWorkerSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if directive == nil {
		t.Fatal("expected worker-src directive")
	}
	sources := sourceStrings(directive.Sources)
	if len(sources) != 2 {
		t.Fatalf("expected 2 deduplicated sources, got %v", sources)
	}
}

// The helper is generic over the directive type, not specific to worker-src.
func TestPatchCspSourceDirective_OtherDirectiveType(t *testing.T) {
	t.Parallel()

	policy := &csp.ContentSecurityPolicy{}
	PatchCspSourceDirective[csp.ConnectSrcDirective](
		policy,
		&csp.KeywordSource{Keyword: "self"},
	)

	directive := policy.GetConnectSrc()
	if directive == nil {
		t.Fatal("nil directive")
	}
	if directive == nil {
		t.Fatal("expected connect-src directive to be added")
	}
	if !slices.Contains(sourceStrings(directive.Sources), "'self'") {
		t.Fatalf("expected 'self' in connect-src, got %v", sourceStrings(directive.Sources))
	}
}

func TestPatchCspStyleSrcWithKeyword(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		policy   string
		keywords []string
		expected string
	}{
		{
			name:     "no style-src",
			policy:   "default-src 'self'",
			keywords: []string{"self", "unsafe-hashes"},
			expected: "default-src 'self'; style-src 'self' 'unsafe-hashes'",
		},
		{
			name:     "an existing style-src is added to",
			policy:   "style-src 'self'",
			keywords: []string{"unsafe-hashes"},
			expected: "style-src 'self' 'unsafe-hashes'",
		},
		{
			name:     "what is already there is not repeated",
			policy:   "style-src 'self' 'unsafe-hashes'",
			keywords: []string{"self", "unsafe-hashes"},
			expected: "style-src 'self' 'unsafe-hashes'",
		},
		{
			name:     "empty keywords are skipped",
			policy:   "style-src 'self'",
			keywords: []string{""},
			expected: "style-src 'self'",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			policy, err := csp.Parse([]byte(testCase.policy))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			PatchCspStyleSrcWithKeyword(policy, testCase.keywords...)

			if got := policy.String(); got != testCase.expected {
				t.Errorf("policy: got %q, want %q", got, testCase.expected)
			}
		})
	}
}

func TestPatchCspTrustedTypes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		policy   string
		policies []string
		expected string
	}{
		{
			name:     "no trusted types",
			policy:   "default-src 'self'",
			policies: []string{"default", "dompurify"},
			expected: "default-src 'self'; trusted-types default dompurify; require-trusted-types-for 'script'",
		},
		{
			name:     "an existing directive is added to, and the requirement is left as it is",
			policy:   "trusted-types default; require-trusted-types-for 'script'",
			policies: []string{"default", "dompurify"},
			expected: "trusted-types default dompurify; require-trusted-types-for 'script'",
		},
		{
			// Naming no policy says nothing, so nothing is required either.
			name:     "no policies",
			policy:   "default-src 'self'",
			policies: []string{""},
			expected: "default-src 'self'",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			policy, err := csp.Parse([]byte(testCase.policy))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			PatchCspTrustedTypes(policy, testCase.policies...)

			if got := policy.String(); got != testCase.expected {
				t.Errorf("policy: got %q, want %q", got, testCase.expected)
			}
		})
	}
}
