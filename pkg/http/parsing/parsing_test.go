package parsing

import (
	"testing"
)

func TestParseHttpRequestData(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		input          []byte
		wantNil        bool
		wantErr        bool
		expectedMethod string
		expectedPath   string
	}{
		{
			name:    "empty input is a parse error, not a nil result",
			input:   nil,
			wantErr: true,
		},
		{
			name:           "valid request",
			input:          []byte("GET /path HTTP/1.1\r\nHost: example.com\r\n\r\n"),
			expectedMethod: "GET",
			expectedPath:   "/path",
		},
		{
			name:    "malformed request",
			input:   []byte("@@@ not a valid request line\r\n\r\n"),
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			request, err := ParseHttpRequestData(testCase.input)

			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if request != nil {
					t.Errorf("request = %v, want nil on error", request)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if testCase.wantNil {
				if request != nil {
					t.Errorf("request = %v, want nil", request)
				}
				return
			}

			if request == nil {
				t.Fatal("request is nil, want non-nil")
			}
			if request.Method != testCase.expectedMethod {
				t.Errorf("Method = %q, want %q", request.Method, testCase.expectedMethod)
			}
			if request.URL.Path != testCase.expectedPath {
				t.Errorf("URL.Path = %q, want %q", request.URL.Path, testCase.expectedPath)
			}
		})
	}
}

func TestParseHttpResponseData(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		input              []byte
		wantNil            bool
		wantErr            bool
		expectedStatusCode int
	}{
		{
			name:    "empty input is a parse error, not a nil result",
			input:   nil,
			wantErr: true,
		},
		{
			name:               "valid response",
			input:              []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"),
			expectedStatusCode: 200,
		},
		{
			name:    "malformed response",
			input:   []byte("garbage response\r\n\r\n"),
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			response, err := ParseHttpResponseData(testCase.input)

			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if response != nil {
					t.Errorf("response = %v, want nil on error", response)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if testCase.wantNil {
				if response != nil {
					t.Errorf("response = %v, want nil", response)
				}
				return
			}

			if response == nil {
				t.Fatal("response is nil, want non-nil")
			}
			if response.StatusCode != testCase.expectedStatusCode {
				t.Errorf("StatusCode = %d, want %d", response.StatusCode, testCase.expectedStatusCode)
			}
			if response.Body != nil {
				_ = response.Body.Close()
			}
		})
	}
}
