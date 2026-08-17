package header_parser_config

import (
	"testing"

	"github.com/altshiftab/utils_go/pkg/json/jose/jwe"
)

func TestNew(t *testing.T) {
	t.Parallel()

	defaults := New()
	if defaults.HeaderName != DefaultHeaderName {
		t.Errorf("default header name = %q, want %q", defaults.HeaderName, DefaultHeaderName)
	}
	if defaults.KeyAlgorithm != DefaultKeyAlgorithm {
		t.Errorf("default key algorithm = %q, want %q", defaults.KeyAlgorithm, DefaultKeyAlgorithm)
	}
	if defaults.ContentEncryption != DefaultContentEncryption {
		t.Errorf("default content encryption = %q, want %q", defaults.ContentEncryption, DefaultContentEncryption)
	}
	if defaults.ContentType != DefaultContentType {
		t.Errorf("default content type = %q, want %q", defaults.ContentType, DefaultContentType)
	}

	config := New(
		WithHeaderName("X-Custom-Jwk"),
		WithKeyAlgorithm(jwe.KeyAlgorithm("ECDH-ES+A256KW")),
		WithContentEncryption(jwe.ContentEncryption("A128GCM")),
		WithContentType("application/octet-stream"),
	)
	if config.HeaderName != "X-Custom-Jwk" {
		t.Errorf("header name = %q, want %q", config.HeaderName, "X-Custom-Jwk")
	}
	if config.KeyAlgorithm != "ECDH-ES+A256KW" {
		t.Errorf("key algorithm = %q, want %q", config.KeyAlgorithm, "ECDH-ES+A256KW")
	}
	if config.ContentEncryption != "A128GCM" {
		t.Errorf("content encryption = %q, want %q", config.ContentEncryption, "A128GCM")
	}
	if config.ContentType != "application/octet-stream" {
		t.Errorf("content type = %q, want %q", config.ContentType, "application/octet-stream")
	}
}
