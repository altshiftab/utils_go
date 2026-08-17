package token

import (
	"net/http"
	"time"
)

const expiryDelta = 10 * time.Second

type Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`

	ExpiresIn int64 `json:"expires_in,omitempty"`

	Raw map[string]any
}

func (t *Token) Type() string {
	if t.TokenType != "" {
		return t.TokenType
	}
	return "Bearer"
}

func (t *Token) SetAuthHeader(r *http.Request) {
	r.Header.Set("Authorization", t.Type()+" "+t.AccessToken)
}

func (t *Token) Valid() bool {
	return t != nil && t.AccessToken != "" && !t.expired()
}

func (t *Token) expired() bool {
	if t.Expiry.IsZero() {
		return false
	}
	return t.Expiry.Round(0).Add(-expiryDelta).Before(time.Now())
}

func (t *Token) Extra(key string) any {
	if t.Raw == nil {
		return nil
	}
	return t.Raw[key]
}
