package body_loader

import (
	stdcontext "context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_loader/body_setting"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/body_parser"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
)

func TestLoader_Fields(t *testing.T) {
	t.Parallel()

	parser := body_parser.New(func(_ *http.Request, body []byte) (any, *response_error.ResponseError) {
		return string(body), nil
	})

	loader := &Loader{
		Parser:      parser,
		ContentType: "application/json",
		Setting:     body_setting.Optional,
		MaxBytes:    1024,
	}

	if loader.Parser == nil {
		t.Fatal("Parser is nil")
	}
	if loader.ContentType != "application/json" {
		t.Errorf("ContentType = %q, want %q", loader.ContentType, "application/json")
	}
	if loader.Setting != body_setting.Optional {
		t.Errorf("Setting = %d, want Optional (%d)", int(loader.Setting), int(body_setting.Optional))
	}
	if loader.MaxBytes != 1024 {
		t.Errorf("MaxBytes = %d, want 1024", loader.MaxBytes)
	}
}

func TestLoader_ParserRoundTrip(t *testing.T) {
	t.Parallel()

	loader := &Loader{
		Parser: body_parser.New(func(_ *http.Request, body []byte) (any, *response_error.ResponseError) {
			return string(body), nil
		}),
	}

	request := httptest.NewRequestWithContext(stdcontext.Background(), http.MethodPost, "/", nil)
	result, responseError := loader.Parser.Parse(request, []byte("payload"))
	if responseError != nil {
		t.Fatalf("unexpected response error: %+v", responseError)
	}
	if result != "payload" {
		t.Errorf("Parse result = %v, want %q", result, "payload")
	}
}

func TestLoader_ZeroValueSettingIsRequired(t *testing.T) {
	t.Parallel()

	var loader Loader
	if loader.Setting != body_setting.Required {
		t.Errorf("zero value Setting = %d, want Required (%d)", int(loader.Setting), int(body_setting.Required))
	}
	if loader.Parser != nil {
		t.Error("zero value Parser = non-nil, want nil")
	}
	if loader.MaxBytes != 0 {
		t.Errorf("zero value MaxBytes = %d, want 0", loader.MaxBytes)
	}
}
