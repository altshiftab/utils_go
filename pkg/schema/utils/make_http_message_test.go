package utils

import (
	"testing"

	"github.com/altshiftab/utils_go/pkg/schema"
)

func TestMakeHttpMessage_NilBase(t *testing.T) {
	t.Parallel()

	result := MakeHttpMessage(nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestMakeHttpMessage_FullBase(t *testing.T) {
	t.Parallel()

	base := &schema.Base{
		Source: &schema.Target{Ip: "127.0.0.1"},
		User:   &schema.User{Name: "frank"},
		Http: &schema.Http{
			Version: "1.1",
			Request: &schema.HttpRequest{
				Method:   "GET",
				Referrer: "http://example.com/",
			},
			Response: &schema.HttpResponse{
				StatusCode: 200,
				Body:       &schema.Body{Bytes: 2326},
			},
		},
		Url:       &schema.Url{Original: "/index.html"},
		UserAgent: &schema.UserAgent{Original: "Mozilla/5.0"},
	}

	expected := `127.0.0.1 - frank "GET /index.html HTTP/1.1" 200 2326 "http://example.com/" "Mozilla/5.0"`
	result := MakeHttpMessage(base)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestMakeHttpMessage_NoUser(t *testing.T) {
	t.Parallel()

	base := &schema.Base{
		Source: &schema.Target{Ip: "192.168.1.1"},
		Http: &schema.Http{
			Version: "1.1",
			Request: &schema.HttpRequest{
				Method: "POST",
			},
			Response: &schema.HttpResponse{
				StatusCode: 404,
			},
		},
		Url: &schema.Url{Original: "/api/data"},
	}

	expected := `192.168.1.1 - - "POST /api/data HTTP/1.1" 404 - "-" "-"`
	result := MakeHttpMessage(base)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestMakeHttpMessage_NoRequest(t *testing.T) {
	t.Parallel()

	base := &schema.Base{
		Http: &schema.Http{
			Response: &schema.HttpResponse{
				StatusCode: 500,
				Body:       &schema.Body{Bytes: 100},
			},
		},
	}

	expected := `- - - "-" 500 100 "-" "-"`
	result := MakeHttpMessage(base)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestMakeHttpMessage_NoResponse(t *testing.T) {
	t.Parallel()

	base := &schema.Base{
		Source: &schema.Target{Ip: "10.0.0.1"},
		Http: &schema.Http{
			Version: "1.1",
			Request: &schema.HttpRequest{
				Method: "GET",
			},
		},
		Url:       &schema.Url{Original: "/"},
		UserAgent: &schema.UserAgent{Original: "curl/7.68.0"},
	}

	expected := `10.0.0.1 - - "GET / HTTP/1.1" - - "-" "curl/7.68.0"`
	result := MakeHttpMessage(base)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestMakeHttpMessage_BodyBytesFromResponseBody(t *testing.T) {
	t.Parallel()

	base := &schema.Base{
		Source: &schema.Target{Ip: "127.0.0.1"},
		Http: &schema.Http{
			Version: "1.1",
			Request: &schema.HttpRequest{
				Method: "GET",
			},
			Response: &schema.HttpResponse{
				StatusCode: 200,
				Body:       &schema.Body{Bytes: 27},
			},
		},
		Url: &schema.Url{Original: "/page"},
	}

	expected := `127.0.0.1 - - "GET /page HTTP/1.1" 200 27 "-" "-"`
	result := MakeHttpMessage(base)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestMakeHttpMessage_PathAndQuery(t *testing.T) {
	t.Parallel()

	base := &schema.Base{
		Source: &schema.Target{Ip: "10.0.0.1"},
		Http: &schema.Http{
			Version: "1.1",
			Request: &schema.HttpRequest{
				Method: "GET",
			},
			Response: &schema.HttpResponse{
				StatusCode: 200,
				Body:       &schema.Body{Bytes: 512},
			},
		},
		Url: &schema.Url{Path: "/search", Query: "q=test"},
	}

	expected := `10.0.0.1 - - "GET /search?q=test HTTP/1.1" 200 512 "-" "-"`
	result := MakeHttpMessage(base)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestMakeHttpMessage_UserEmailFallback(t *testing.T) {
	t.Parallel()

	base := &schema.Base{
		Source: &schema.Target{Ip: "127.0.0.1"},
		User:   &schema.User{Email: "frank@example.com"},
		Http: &schema.Http{
			Version: "1.1",
			Request: &schema.HttpRequest{
				Method: "GET",
			},
			Response: &schema.HttpResponse{
				StatusCode: 200,
			},
		},
		Url: &schema.Url{Original: "/index.html"},
	}

	expected := `127.0.0.1 - frank@example.com "GET /index.html HTTP/1.1" 200 - "-" "-"`
	result := MakeHttpMessage(base)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestMakeHttpMessage_UserNameOverEmail(t *testing.T) {
	t.Parallel()

	base := &schema.Base{
		Source: &schema.Target{Ip: "127.0.0.1"},
		User:   &schema.User{Name: "frank", Email: "frank@example.com"},
		Http: &schema.Http{
			Version: "1.1",
			Request: &schema.HttpRequest{
				Method: "GET",
			},
			Response: &schema.HttpResponse{
				StatusCode: 200,
			},
		},
		Url: &schema.Url{Original: "/index.html"},
	}

	expected := `127.0.0.1 - frank "GET /index.html HTTP/1.1" 200 - "-" "-"`
	result := MakeHttpMessage(base)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestMakeHttpMessage_EmptyBase(t *testing.T) {
	t.Parallel()

	base := &schema.Base{}

	expected := `- - - "-" - - "-" "-"`
	result := MakeHttpMessage(base)
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}
