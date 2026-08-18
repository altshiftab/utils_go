package parsing

import (
	"bufio"
	"bytes"
	"fmt"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"net/http"
)

// ParseHttpRequestData parses raw request bytes. Empty input is reported as an
// error rather than a nil request with no error, so that a caller can rely on a
// nil error meaning it received a request.
func ParseHttpRequestData(requestBytes []byte) (*http.Request, error) {
	if len(requestBytes) == 0 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrParseError, empty_error.New("request bytes")),
		)
	}

	reader := bufio.NewReader(bytes.NewReader(requestBytes))
	request, err := http.ReadRequest(reader)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("http read request: %w", err), reader)
	}

	return request, nil
}

// ParseHttpResponseData parses raw response bytes. Empty input is reported as an
// error rather than a nil response with no error, so that a caller can rely on a
// nil error meaning it received a response.
func ParseHttpResponseData(responseBytes []byte) (*http.Response, error) {
	if len(responseBytes) == 0 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrParseError, empty_error.New("response bytes")),
		)
	}

	reader := bufio.NewReader(bytes.NewReader(responseBytes))
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("http read response: %w", err), reader)
	}

	return response, nil
}
