package openapi_config

import (
	"testing"

	openapiTypes "github.com/altshiftab/utils_go/pkg/http/openapi/types"
)

func TestNewDefaults(t *testing.T) {
	t.Parallel()

	config := New()

	if config.Version != Version32 {
		t.Errorf("the default version is %q, expected %q", config.Version, Version32)
	}
	if config.Path != DefaultPath {
		t.Errorf("the default path is %q, expected %q", config.Path, DefaultPath)
	}

	// A document naming every operation the service offers is not handed out unasked.
	if config.Public {
		t.Error("a document is public by default")
	}

	if config.Info != nil {
		t.Errorf("a document has info by default: %v", config.Info)
	}
	if len(config.SecuritySchemes) != 0 {
		t.Errorf("a document has security schemes by default: %v", config.SecuritySchemes)
	}
}

func TestOptions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		option Option
		assert func(t *testing.T, config *Config)
	}{
		{
			name:   "version",
			option: WithVersion(Version31),
			assert: func(t *testing.T, config *Config) {
				if config.Version != Version31 {
					t.Errorf("the version is %q", config.Version)
				}
			},
		},
		{
			name:   "info",
			option: WithInfo("Signals", "1.2.3"),
			assert: func(t *testing.T, config *Config) {
				if config.Info == nil || config.Info.Title != "Signals" || config.Info.Version != "1.2.3" {
					t.Errorf("unexpected info: %v", config.Info)
				}
			},
		},
		{
			name:   "info description",
			option: WithInfoDescription("What the API is for."),
			assert: func(t *testing.T, config *Config) {
				if config.Info == nil || config.Info.Description != "What the API is for." {
					t.Errorf("unexpected info: %v", config.Info)
				}
			},
		},
		{
			name:   "server",
			option: WithServer("https://example.test", "Production"),
			assert: func(t *testing.T, config *Config) {
				if len(config.Servers) != 1 || config.Servers[0].Url != "https://example.test" {
					t.Errorf("unexpected servers: %v", config.Servers)
				}
			},
		},
		{
			name:   "security scheme",
			option: WithSecurityScheme("session", &openapiTypes.SecurityScheme{Type: "apiKey"}),
			assert: func(t *testing.T, config *Config) {
				scheme, ok := config.SecuritySchemes["session"]
				if !ok || scheme.Type != "apiKey" {
					t.Errorf("unexpected security schemes: %v", config.SecuritySchemes)
				}
			},
		},
		{
			name:   "tag",
			option: WithTag("orders", "Everything about orders."),
			assert: func(t *testing.T, config *Config) {
				if len(config.Tags) != 1 || config.Tags[0].Name != "orders" {
					t.Errorf("unexpected tags: %v", config.Tags)
				}
			},
		},
		{
			name:   "path",
			option: WithPath("/api/openapi.json"),
			assert: func(t *testing.T, config *Config) {
				if config.Path != "/api/openapi.json" {
					t.Errorf("the path is %q", config.Path)
				}
			},
		},
		{
			name:   "public",
			option: WithPublic(true),
			assert: func(t *testing.T, config *Config) {
				if !config.Public {
					t.Error("the document is not public")
				}
			},
		},
		{
			name:   "cache control",
			option: WithCacheControl("public, max-age=60"),
			assert: func(t *testing.T, config *Config) {
				if config.CacheControl != "public, max-age=60" {
					t.Errorf("the cache control is %q", config.CacheControl)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			testCase.assert(t, New(testCase.option))
		})
	}
}

// TestWithInfoAndDescriptionCompose checks that the two options that both reach Info do not undo one
// another, whichever order they arrive in.
func TestWithInfoAndDescriptionCompose(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		options []Option
	}{
		{
			name:    "info first",
			options: []Option{WithInfo("Signals", "1.0.0"), WithInfoDescription("Prose.")},
		},
		{
			name:    "description first",
			options: []Option{WithInfoDescription("Prose."), WithInfo("Signals", "1.0.0")},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			config := New(testCase.options...)

			if config.Info == nil {
				t.Fatalf("%s: there is no info", testCase.name)
			}
			if config.Info.Title != "Signals" || config.Info.Version != "1.0.0" {
				t.Errorf("%s: the title or version was lost: %v", testCase.name, config.Info)
			}
			if config.Info.Description != "Prose." {
				t.Errorf("%s: the description was lost: %v", testCase.name, config.Info)
			}
		})
	}
}

// TestSecuritySchemesAccumulate checks that declaring a second scheme does not replace the first,
// since the schemes are alternatives offered together.
func TestSecuritySchemesAccumulate(t *testing.T) {
	t.Parallel()

	config := New(
		WithSecurityScheme("session", &openapiTypes.SecurityScheme{Type: "apiKey", In: "cookie", Name: "session"}),
		WithSecurityScheme("apiKey", &openapiTypes.SecurityScheme{Type: "http", Scheme: "bearer"}),
	)

	if len(config.SecuritySchemes) != 2 {
		t.Errorf("expected two schemes, got %v", config.SecuritySchemes)
	}
}
