package log

import (
	"context"
	"crypto/tls"
	"log/slog"
	"testing"

	altshiftTlsContext "github.com/altshiftab/utils_go/pkg/tls/context"
	altshiftTlsTypes "github.com/altshiftab/utils_go/pkg/tls/types"
)

func TestParseTlsContextNil(t *testing.T) {
	t.Parallel()

	if got := ParseTlsContext(nil); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestParseTlsContextNilConnectionState(t *testing.T) {
	t.Parallel()

	if got := ParseTlsContext(&altshiftTlsTypes.TlsContext{}); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestParseTlsContextWithConnectionState(t *testing.T) {
	t.Parallel()

	tlsContext := &altshiftTlsTypes.TlsContext{
		ConnectionState: &tls.ConnectionState{
			Version:           tls.VersionTLS13,
			HandshakeComplete: true,
			ServerName:        "example.com",
		},
	}

	base := ParseTlsContext(tlsContext)
	if base == nil {
		t.Fatal("expected non-nil base")
	}
	if base.Tls == nil {
		t.Fatal("expected base.Tls to be populated")
	}
	if !base.Tls.Established {
		t.Fatal("expected Established to be true")
	}
	if base.Tls.TlsProtocol == nil || base.Tls.TlsProtocol.Version != "1.3" {
		t.Fatalf("expected TLS protocol version 1.3, got %+v", base.Tls.TlsProtocol)
	}
	if base.Tls.Client == nil || base.Tls.Client.ServerName != "example.com" {
		t.Fatalf("expected client server name example.com, got %+v", base.Tls.Client)
	}
}

func TestExtractTlsContextNoValue(t *testing.T) {
	t.Parallel()

	record := slog.Record{}
	if err := ExtractTlsContext(context.Background(), &record); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.NumAttrs() != 0 {
		t.Fatalf("expected no attrs, got %d", record.NumAttrs())
	}
}

func TestExtractTlsContextNilPointerValue(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(
		context.Background(),
		altshiftTlsContext.TlsContextKey,
		(*altshiftTlsTypes.TlsContext)(nil),
	)

	record := slog.Record{}
	if err := ExtractTlsContext(ctx, &record); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.NumAttrs() != 0 {
		t.Fatalf("expected no attrs, got %d", record.NumAttrs())
	}
}

func TestExtractTlsContextNilConnectionState(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(
		context.Background(),
		altshiftTlsContext.TlsContextKey,
		&altshiftTlsTypes.TlsContext{},
	)

	record := slog.Record{}
	if err := ExtractTlsContext(ctx, &record); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.NumAttrs() != 0 {
		t.Fatalf("expected no attrs, got %d", record.NumAttrs())
	}
}

func TestExtractTlsContextWithConnectionState(t *testing.T) {
	t.Parallel()

	ctx := context.WithValue(
		context.Background(),
		altshiftTlsContext.TlsContextKey,
		&altshiftTlsTypes.TlsContext{
			ConnectionState: &tls.ConnectionState{
				Version:           tls.VersionTLS13,
				HandshakeComplete: true,
				ServerName:        "example.com",
			},
		},
	)

	record := slog.Record{}
	if err := ExtractTlsContext(ctx, &record); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.NumAttrs() == 0 {
		t.Fatal("expected attrs to be added")
	}

	var foundTls bool
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "tls" {
			foundTls = true
			return false
		}
		return true
	})
	if !foundTls {
		t.Fatal("expected a \"tls\" attribute group to be added")
	}
}

func TestTlsContextExtractorHandle(t *testing.T) {
	t.Parallel()

	record := slog.Record{}
	if err := TlsContextExtractor.Handle(context.Background(), &record); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.NumAttrs() != 0 {
		t.Fatalf("expected no attrs, got %d", record.NumAttrs())
	}
}
