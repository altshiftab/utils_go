package reporting_api_test

import (
	jsonv2 "encoding/json/v2"
	"testing"

	"github.com/altshiftab/utils_go/pkg/http/types/integrity_policy"
	"github.com/altshiftab/utils_go/pkg/http/types/reporting_api"
	altshiftJsonSchema "github.com/altshiftab/utils_go/pkg/json/schema"
)

// A report is serialized by the browser, so the schema has to take what engines actually send.
func TestReportSchemaTakesWhatBrowsersSend(t *testing.T) {
	t.Parallel()

	schema, err := altshiftJsonSchema.NewFromType[[]*reporting_api.Report[*integrity_policy.IntegrityViolationReportBody]]()
	if err != nil {
		t.Fatalf("new from type: %v", err)
	}
	if err := schema.Resolve(nil); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	testCases := []struct {
		name        string
		payload     string
		expectValid bool
	}{
		{
			// What section 2.4 serializes, as Firefox sends it.
			name:        "the members the specification lists",
			payload:     `[{"age":64,"type":"integrity-violation","url":"https://example.com/","user_agent":"ua","body":{"documentURL":"https://example.com/","blockedURL":"https://example.com/a.js","destination":"script","reportOnly":false}}]`,
			expectValid: true,
		},
		{
			// What WebKit sends: the report struct's internal members leak into the batch.
			name:        "the internal members WebKit adds",
			payload:     `[{"age":0,"attempts":0,"destination":"integrity-endpoint","type":"integrity-violation","url":"https://example.com/","user_agent":"ua","body":{"documentURL":"https://example.com/","blockedURL":"https://example.com/a.js","destination":"script","reportOnly":false}}]`,
			expectValid: true,
		},
		{
			name:        "a member a future engine adds to the body",
			payload:     `[{"age":0,"type":"integrity-violation","url":"https://example.com/","user_agent":"ua","body":{"documentURL":"https://example.com/","blockedURL":"https://example.com/a.js","destination":"script","reportOnly":false,"somethingNew":1}}]`,
			expectValid: true,
		},
		{
			name:        "a batch missing a member the specification requires",
			payload:     `[{"age":0,"url":"https://example.com/","user_agent":"ua","body":{"documentURL":"https://example.com/","blockedURL":"https://example.com/a.js","destination":"script","reportOnly":false}}]`,
			expectValid: false,
		},
		{
			name:        "a batch that is not a batch",
			payload:     `{"age":0}`,
			expectValid: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var instance any
			if err := jsonv2.Unmarshal([]byte(testCase.payload), &instance); err != nil {
				t.Fatalf("json unmarshal: %v", err)
			}

			err := schema.Validate(instance)
			if testCase.expectValid && err != nil {
				t.Errorf("expected the batch to validate, got %v", err)
			}
			if !testCase.expectValid && err == nil {
				t.Error("expected the batch to be refused")
			}
		})
	}
}
