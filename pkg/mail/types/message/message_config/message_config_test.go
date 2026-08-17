package message_config

import (
	"net/mail"
	"reflect"
	"testing"

	"github.com/altshiftab/utils_go/pkg/mail/types/message/message_header"
)

func TestNew_NoOptions(t *testing.T) {
	t.Parallel()

	config := New()
	if config == nil {
		t.Fatal("New() = nil, want non-nil")
	}
	if !reflect.DeepEqual(config, &Config{}) {
		t.Errorf("config = %+v, want zero-valued Config", config)
	}
}

func TestNew_AppliesAllOptions(t *testing.T) {
	t.Parallel()

	cc := []*mail.Address{{Address: "cc@example.com"}}
	bcc := []*mail.Address{{Address: "bcc@example.com"}}
	replyTo := []*mail.Address{{Address: "reply@example.com"}}
	headers := []*message_header.Header{
		{Name: "X-Foo", Value: "bar"},
		{Name: "X-Baz", Value: "qux"},
	}

	config := New(
		WithCc(cc),
		WithBcc(bcc),
		WithReplyTo(replyTo),
		WithDomain("custom.example"),
		WithHeaders(headers...),
	)

	if !reflect.DeepEqual(config.Cc, cc) {
		t.Errorf("Cc = %v, want %v", config.Cc, cc)
	}
	if !reflect.DeepEqual(config.Bcc, bcc) {
		t.Errorf("Bcc = %v, want %v", config.Bcc, bcc)
	}
	if !reflect.DeepEqual(config.ReplyTo, replyTo) {
		t.Errorf("ReplyTo = %v, want %v", config.ReplyTo, replyTo)
	}
	if config.Domain != "custom.example" {
		t.Errorf("Domain = %q, want %q", config.Domain, "custom.example")
	}
	if !reflect.DeepEqual(config.Headers, headers) {
		t.Errorf("Headers = %v, want %v", config.Headers, headers)
	}
}

func TestNew_NilOptionIgnored(t *testing.T) {
	t.Parallel()

	config := New(nil, WithDomain("example.com"), nil)
	if config.Domain != "example.com" {
		t.Errorf("Domain = %q, want %q", config.Domain, "example.com")
	}
}

func TestNew_LastOptionWins(t *testing.T) {
	t.Parallel()

	config := New(WithDomain("first.example"), WithDomain("second.example"))
	if config.Domain != "second.example" {
		t.Errorf("Domain = %q, want %q", config.Domain, "second.example")
	}
}

func TestWithHeaders_NoArgs(t *testing.T) {
	t.Parallel()

	config := New(WithHeaders())
	if len(config.Headers) != 0 {
		t.Errorf("Headers = %v, want empty/nil", config.Headers)
	}
}

func TestWithHeaders_Variadic(t *testing.T) {
	t.Parallel()

	h1 := &message_header.Header{Name: "X-One", Value: "1"}
	h2 := &message_header.Header{Name: "X-Two", Value: "2"}

	config := New(WithHeaders(h1, h2))
	if len(config.Headers) != 2 || config.Headers[0] != h1 || config.Headers[1] != h2 {
		t.Errorf("Headers = %v, want [%v %v]", config.Headers, h1, h2)
	}
}
