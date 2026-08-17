package token_source

import (
	"sync"

	oauth2Token "github.com/altshiftab/utils_go/pkg/oauth2/types/token"
)

type TokenSource interface {
	Token() (*oauth2Token.Token, error)
}

type StaticTokenSource struct {
	t *oauth2Token.Token
}

func (s *StaticTokenSource) Token() (*oauth2Token.Token, error) {
	return s.t, nil
}

// NewStatic returns a TokenSource that always returns the same oauth2Token.
func NewStatic(t *oauth2Token.Token) TokenSource {
	return &StaticTokenSource{t: t}
}

type ReusableTokenSource struct {
	TokenSource TokenSource
	mu          sync.Mutex
	token       *oauth2Token.Token
}

func (s *ReusableTokenSource) Token() (*oauth2Token.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.token != nil && s.token.Valid() {
		return s.token, nil
	}

	t, err := s.TokenSource.Token()
	if err != nil {
		return nil, err
	}

	s.token = t
	return t, nil
}

// NewReusable returns a TokenSource that caches the token from src
// and refreshes it when expired.
func NewReusable(t *oauth2Token.Token, src TokenSource) TokenSource {
	return &ReusableTokenSource{
		token:       t,
		TokenSource: src,
	}
}
