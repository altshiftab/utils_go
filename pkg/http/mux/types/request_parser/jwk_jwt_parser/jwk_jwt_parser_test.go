package jwk_jwt_parser_test

import (
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/jwk_jwt_parser"
	"github.com/altshiftab/utils_go/pkg/interfaces/validator"
)

func jwkUrl(t *testing.T) *url.URL {
	t.Helper()

	parsed, err := url.Parse("https://www.googleapis.com/oauth2/v3/certs")
	if err != nil {
		t.Fatalf("url parse: %v", err)
	}

	return parsed
}

func acceptAll() validator.Validator[map[string]any] {
	return validator.New(func(_ map[string]any) error { return nil })
}

func TestNew(t *testing.T) {
	testCases := []struct {
		name              string
		headerName        string
		headerValuePrefix string
	}{
		{
			name:              "a bearer credential",
			headerName:        "Authorization",
			headerValuePrefix: "Bearer ",
		},
		{
			// An assertion in a header of its own is the whole value, so an
			// empty prefix has to be accepted rather than read as missing.
			name:              "an assertion in its own header",
			headerName:        "x-goog-iap-jwt-assertion",
			headerValuePrefix: "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			parser, err := jwk_jwt_parser.New(
				jwkUrl(t),
				testCase.headerName,
				testCase.headerValuePrefix,
				acceptAll(),
			)
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", testCase.name, err)
			}
			if parser == nil {
				t.Errorf("%s: no parser was returned", testCase.name)
			}
		})
	}
}

// TestNewRefusesANilClaimsValidator is the reason this package exists.
//
// A JWT validator skips a nil payload validator silently, so a parser
// assembled without one accepts every token the key set signs — for Google's
// OAuth2 keys, a token minted for anybody, by anybody who asked. Refusing it
// here makes the omission impossible rather than merely unlikely.
func TestNewRefusesANilClaimsValidator(t *testing.T) {
	if _, err := jwk_jwt_parser.New(jwkUrl(t), "Authorization", "Bearer ", nil); err == nil {
		t.Fatal("a parser was built with nothing checking its claims")
	}
}

// TestNewRefusesATypedNilClaimsValidator covers the shape a nil interface
// actually arrives in: a nil pointer in a non-nil interface, which == nil does
// not catch.
func TestNewRefusesATypedNilClaimsValidator(t *testing.T) {
	var typedNil validator.Validator[map[string]any] = (*nilValidator)(nil)

	if _, err := jwk_jwt_parser.New(jwkUrl(t), "Authorization", "Bearer ", typedNil); err == nil {
		t.Fatal("a parser was built with a typed-nil claims validator")
	}
}

type nilValidator struct{}

func (*nilValidator) Validate(_ map[string]any) error { return nil }

func TestNewRefusesIncompleteInput(t *testing.T) {
	testCases := []struct {
		name       string
		jwkUrl     *url.URL
		headerName string
	}{
		{name: "no jwk url", jwkUrl: nil, headerName: "Authorization"},
		{name: "no header name", jwkUrl: jwkUrl(t), headerName: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := jwk_jwt_parser.New(testCase.jwkUrl, testCase.headerName, "Bearer ", acceptAll())
			if err == nil {
				t.Errorf("%s: was accepted", testCase.name)
			}
		})
	}
}
