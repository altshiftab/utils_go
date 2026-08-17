package parsing

import (
	"bufio"
	"bytes"
	"fmt"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"net/http"
)

func ParseHttpRequestData(requestBytes []byte) (*http.Request, error) {
	if len(requestBytes) == 0 {
		return nil, nil
	}

	reader := bufio.NewReader(bytes.NewReader(requestBytes))
	request, err := http.ReadRequest(reader)
	if err != nil {
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("http read request: %w", err), reader)
	}

	return request, nil
}

func ParseHttpResponseData(responseBytes []byte) (*http.Response, error) {
	if len(responseBytes) == 0 {
		return nil, nil
	}

	reader := bufio.NewReader(bytes.NewReader(responseBytes))
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("http read response: %w", err), reader)
	}

	return response, nil
}
