package www_authenticate

import "testing"

func TestBearer(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		errorCode        string
		errorDescription string
		want             string
	}{
		{name: "no token", want: "Bearer"},
		{name: "description without code is dropped", errorDescription: "ignored", want: "Bearer"},
		{name: "code only", errorCode: ErrorInvalidToken, want: `Bearer error="invalid_token"`},
		{
			name:             "code and description",
			errorCode:        ErrorInvalidToken,
			errorDescription: "The token has expired.",
			want:             `Bearer error="invalid_token", error_description="The token has expired."`,
		},
		{
			name:             "characters RFC 6750 does not allow are dropped",
			errorCode:        ErrorInvalidRequest,
			errorDescription: "a \"quoted\" \\ välue\n",
			want:             `Bearer error="invalid_request", error_description="a quoted  vlue"`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := Bearer(testCase.errorCode, testCase.errorDescription); got != testCase.want {
				t.Errorf("got %s, want %s", got, testCase.want)
			}
		})
	}
}
