package token_response

import (
	"encoding/json/v2"
	"fmt"
	"time"

	"github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/oauth2/types/token"
)

// TODO: Add more fields?

type Response struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func (r *Response) Token() *token.Token {
	tok := &token.Token{
		AccessToken: r.AccessToken,
		TokenType:   r.TokenType,
	}
	if r.ExpiresIn > 0 {
		tok.Expiry = time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)
	}

	return tok
}

func Parse(data []byte) (*Response, error) {
	var response Response
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, errors.NewWithTrace(
			fmt.Errorf("json unmarshal (token response): %w", err),
			data,
		)
	}

	if response.AccessToken == "" {
		return nil, errors.NewWithTrace(empty_error.New("access token"))
	}

	return &response, nil
}
