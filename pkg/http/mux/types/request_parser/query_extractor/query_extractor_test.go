package query_extractor

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/query_extractor/query_extractor_config"
)

type basicQuery struct {
	Name string `query:"name"`
	Age  int    `query:"age"`
}

type jsonFallbackQuery struct {
	Name string `json:"name"`
	Age  int    `json:"age,omitempty"`
}

type queryTagOverridesJson struct {
	Name string `query:"q_name" json:"j_name"`
}

type optionalQuery struct {
	Name     string `query:"name"`
	Nickname string `query:"nickname,omitempty"`
}

type skipQuery struct {
	Name    string `query:"name"`
	Skipped string `query:"-"`
}

type emailQuery struct {
	Email string `query:"email,format=email"`
}

type uuidQuery struct {
	ID string `query:"id,format=uuid"`
}

type urlQuery struct {
	Link string `query:"link,format=url"`
}

func makeRequest(rawQuery string) *http.Request {
	return &http.Request{
		URL: &url.URL{RawQuery: rawQuery},
	}
}

func TestParse_QueryTag(t *testing.T) {
	t.Parallel()

	parser := New[basicQuery]()
	result, respErr := parser.Parse(makeRequest("name=alice&age=30"))
	if respErr != nil {
		t.Fatalf("unexpected error: %v", respErr)
	}
	if result.Name != "alice" {
		t.Fatalf("expected name 'alice', got %q", result.Name)
	}
	if result.Age != 30 {
		t.Fatalf("expected age 30, got %d", result.Age)
	}
}

func TestParse_JsonFallback(t *testing.T) {
	t.Parallel()

	parser := New[jsonFallbackQuery]()
	result, respErr := parser.Parse(makeRequest("name=bob&age=25"))
	if respErr != nil {
		t.Fatalf("unexpected error: %v", respErr)
	}
	if result.Name != "bob" {
		t.Fatalf("expected name 'bob', got %q", result.Name)
	}
	if result.Age != 25 {
		t.Fatalf("expected age 25, got %d", result.Age)
	}
}

func TestParse_JsonFallbackOptional(t *testing.T) {
	t.Parallel()

	parser := New[jsonFallbackQuery](query_extractor_config.WithAllowAdditionalParameters(true))
	result, respErr := parser.Parse(makeRequest("name=bob"))
	if respErr != nil {
		t.Fatalf("unexpected error: %v", respErr)
	}
	if result.Name != "bob" {
		t.Fatalf("expected name 'bob', got %q", result.Name)
	}
	if result.Age != 0 {
		t.Fatalf("expected age 0, got %d", result.Age)
	}
}

func TestParse_QueryTagOverridesJson(t *testing.T) {
	t.Parallel()

	parser := New[queryTagOverridesJson](query_extractor_config.WithAllowAdditionalParameters(true))
	result, respErr := parser.Parse(makeRequest("q_name=alice"))
	if respErr != nil {
		t.Fatalf("unexpected error: %v", respErr)
	}
	if result.Name != "alice" {
		t.Fatalf("expected name 'alice', got %q", result.Name)
	}
}

func TestParse_QueryTagSkip(t *testing.T) {
	t.Parallel()

	parser := New[skipQuery](query_extractor_config.WithAllowAdditionalParameters(true))
	result, respErr := parser.Parse(makeRequest("name=alice"))
	if respErr != nil {
		t.Fatalf("unexpected error: %v", respErr)
	}
	if result.Name != "alice" {
		t.Fatalf("expected name 'alice', got %q", result.Name)
	}
	if result.Skipped != "" {
		t.Fatalf("expected empty Skipped, got %q", result.Skipped)
	}
}

func TestParse_OptionalQuery(t *testing.T) {
	t.Parallel()

	parser := New[optionalQuery](query_extractor_config.WithAllowAdditionalParameters(true))
	result, respErr := parser.Parse(makeRequest("name=alice"))
	if respErr != nil {
		t.Fatalf("unexpected error: %v", respErr)
	}
	if result.Name != "alice" {
		t.Fatalf("expected name 'alice', got %q", result.Name)
	}
	if result.Nickname != "" {
		t.Fatalf("expected empty Nickname, got %q", result.Nickname)
	}
}

func TestParse_EmailFormatValid(t *testing.T) {
	t.Parallel()

	parser := New[emailQuery](query_extractor_config.WithAllowAdditionalParameters(true))
	result, respErr := parser.Parse(makeRequest("email=user@example.com"))
	if respErr != nil {
		t.Fatalf("unexpected error: %v", respErr)
	}
	if result.Email != "user@example.com" {
		t.Fatalf("expected 'user@example.com', got %q", result.Email)
	}
}

func TestParse_EmailFormatInvalid(t *testing.T) {
	t.Parallel()

	parser := New[emailQuery](query_extractor_config.WithAllowAdditionalParameters(true))
	_, respErr := parser.Parse(makeRequest("email=not-an-email"))
	if respErr == nil {
		t.Fatal("expected error for invalid email format")
	}
}

func TestParse_UuidFormatValid(t *testing.T) {
	t.Parallel()

	parser := New[uuidQuery](query_extractor_config.WithAllowAdditionalParameters(true))
	result, respErr := parser.Parse(makeRequest("id=550e8400-e29b-41d4-a716-446655440000"))
	if respErr != nil {
		t.Fatalf("unexpected error: %v", respErr)
	}
	if result.ID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("unexpected id: %q", result.ID)
	}
}

func TestParse_UuidFormatInvalid(t *testing.T) {
	t.Parallel()

	parser := New[uuidQuery](query_extractor_config.WithAllowAdditionalParameters(true))
	_, respErr := parser.Parse(makeRequest("id=not-a-uuid"))
	if respErr == nil {
		t.Fatal("expected error for invalid uuid format")
	}
}

func TestParse_UrlFormatValid(t *testing.T) {
	t.Parallel()

	parser := New[urlQuery](query_extractor_config.WithAllowAdditionalParameters(true))
	result, respErr := parser.Parse(makeRequest("link=https%3A%2F%2Fexample.com%2Fpath"))
	if respErr != nil {
		t.Fatalf("unexpected error: %v", respErr)
	}
	if result.Link != "https://example.com/path" {
		t.Fatalf("expected 'https://example.com/path', got %q", result.Link)
	}
}

func TestParse_UrlFormatInvalid(t *testing.T) {
	t.Parallel()

	parser := New[urlQuery](query_extractor_config.WithAllowAdditionalParameters(true))
	_, respErr := parser.Parse(makeRequest("link=not-a-url"))
	if respErr == nil {
		t.Fatal("expected error for invalid url format")
	}
}

func TestEmpty_AllowsEmptyQuery(t *testing.T) {
	t.Parallel()

	if _, respErr := Empty.Parse(makeRequest("")); respErr != nil {
		t.Fatalf("unexpected error for empty query: %v", respErr)
	}
}

func TestEmpty_RejectsNonEmptyQuery(t *testing.T) {
	t.Parallel()

	_, respErr := Empty.Parse(makeRequest("foo=bar"))
	if respErr == nil {
		t.Fatal("expected error for non-empty query")
	}
	if respErr.ProblemDetail == nil {
		t.Fatalf("expected a problem detail, got %v", respErr)
	}
	if respErr.ProblemDetail.Status != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, respErr.ProblemDetail.Status)
	}
}

func TestEmpty_RejectsValuelessParameter(t *testing.T) {
	t.Parallel()

	if _, respErr := Empty.Parse(makeRequest("foo")); respErr == nil {
		t.Fatal("expected error for valueless query parameter")
	}
}

func TestEmpty_RejectsMultipleParameters(t *testing.T) {
	t.Parallel()

	if _, respErr := Empty.Parse(makeRequest("a=1&b=2&c=3")); respErr == nil {
		t.Fatal("expected error for multiple query parameters")
	}
}

type typedQuery struct {
	Flag  bool     `query:"flag"`
	Count int      `query:"count"`
	Small int8     `query:"small"`
	Big   uint64   `query:"big"`
	Ratio float64  `query:"ratio"`
	Tags  []string `query:"tags"`
	Pair  [2]int   `query:"pair"`
	Raw   []byte   `query:"raw"`
}

func TestParse_TypedFields(t *testing.T) {
	t.Parallel()

	result, respErr := New[typedQuery]().Parse(
		makeRequest("flag&count=-5&small=7&big=42&ratio=1.5&tags=a&tags=b&pair=1&pair=2&raw=hello"),
	)
	if respErr != nil {
		t.Fatalf("unexpected error: %v", respErr)
	}

	if !result.Flag {
		t.Error("expected Flag to be true for a valueless parameter")
	}
	if result.Count != -5 {
		t.Errorf("Count = %d, want -5", result.Count)
	}
	if result.Small != 7 {
		t.Errorf("Small = %d, want 7", result.Small)
	}
	if result.Big != 42 {
		t.Errorf("Big = %d, want 42", result.Big)
	}
	if result.Ratio != 1.5 {
		t.Errorf("Ratio = %v, want 1.5", result.Ratio)
	}
	if len(result.Tags) != 2 || result.Tags[0] != "a" || result.Tags[1] != "b" {
		t.Errorf("Tags = %#v", result.Tags)
	}
	if result.Pair != [2]int{1, 2} {
		t.Errorf("Pair = %#v", result.Pair)
	}
	if string(result.Raw) != "hello" {
		t.Errorf("Raw = %q, want hello", result.Raw)
	}
}

type (
	boolField struct {
		V bool `query:"v"`
	}
	intField struct {
		V int `query:"v"`
	}
	uintField struct {
		V uint `query:"v"`
	}
	floatField struct {
		V float64 `query:"v"`
	}
	arrayField struct {
		V [2]int `query:"v"`
	}
	bytesField struct {
		V []byte `query:"v"`
	}
	pointerField struct {
		V *int `query:"v"`
	}
)

func TestParse_ValueErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		hasError func() bool
	}{
		{"bad bool", func() bool { _, re := New[boolField]().Parse(makeRequest("v=maybe")); return re != nil }},
		{"bad int", func() bool { _, re := New[intField]().Parse(makeRequest("v=abc")); return re != nil }},
		{"bad uint", func() bool { _, re := New[uintField]().Parse(makeRequest("v=-1")); return re != nil }},
		{"bad float", func() bool { _, re := New[floatField]().Parse(makeRequest("v=xyz")); return re != nil }},
		{"wrong array length", func() bool { _, re := New[arrayField]().Parse(makeRequest("v=1")); return re != nil }},
		{"bad array element", func() bool { _, re := New[arrayField]().Parse(makeRequest("v=1&v=x")); return re != nil }},
		{"bytes expects single value", func() bool { _, re := New[bytesField]().Parse(makeRequest("v=a&v=b")); return re != nil }},
		{"pointer field", func() bool { _, re := New[pointerField]().Parse(makeRequest("v=1")); return re != nil }},
		{"missing parameter", func() bool { _, re := New[intField]().Parse(makeRequest("")); return re != nil }},
		{"multiple values for scalar", func() bool { _, re := New[intField]().Parse(makeRequest("v=1&v=2")); return re != nil }},
		{"unknown parameter", func() bool { _, re := New[intField]().Parse(makeRequest("v=1&extra=2")); return re != nil }},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if !testCase.hasError() {
				t.Fatal("expected an error")
			}
		})
	}
}
