package generate_content_request

import (
	"encoding/json/v2"
	"testing"
)

func TestGenerateContentRequestJSON(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		request *GenerateContentRequest
		want    string
	}{
		{name: "store off", request: &GenerateContentRequest{Store: new(false)}, want: `{"contents":[],"store":false}`},
		{name: "store on", request: &GenerateContentRequest{Store: new(true)}, want: `{"contents":[],"store":true}`},
		{name: "store left to the project", request: &GenerateContentRequest{}, want: `{"contents":[]}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			raw, err := json.Marshal(testCase.request)
			if err != nil {
				t.Fatalf("json marshal: %v", err)
			}

			if string(raw) != testCase.want {
				t.Errorf("got %s, want %s", raw, testCase.want)
			}
		})
	}
}
