package validator

import (
	"errors"
	"testing"
	"time"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	jwtErrors "github.com/altshiftab/utils_go/pkg/json/jose/jwt/errors"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/numeric_date"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/token"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwt/types/validator/registered_claims_validator"
)

var errStub = errors.New("stub validator error")

type stubMapValidator struct {
	err      error
	received map[string]any
	called   bool
}

func (s *stubMapValidator) Validate(fields map[string]any) error {
	s.called = true
	s.received = fields
	return s.err
}

func TestValidateNilToken(t *testing.T) {
	t.Parallel()

	v := &Validator{}
	err := v.Validate(nil)
	if err == nil {
		t.Fatal("expected error for nil token")
	}
	if !errors.Is(err, altshiftErrors.ErrValidationError) {
		t.Errorf("error = %v, want ErrValidationError", err)
	}
}

func TestValidateNoValidators(t *testing.T) {
	t.Parallel()

	v := &Validator{}
	tok := &token.Token{
		Header:  map[string]any{"alg": "HS256"},
		Payload: map[string]any{"sub": "user"},
	}
	if err := v.Validate(tok); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateHeaderValidatorCalled(t *testing.T) {
	t.Parallel()

	header := map[string]any{"alg": "HS256"}
	headerValidator := &stubMapValidator{}
	v := &Validator{HeaderValidator: headerValidator}
	tok := &token.Token{Header: header, Payload: map[string]any{"sub": "user"}}

	if err := v.Validate(tok); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !headerValidator.called {
		t.Fatal("header validator was not called")
	}
	if headerValidator.received["alg"] != "HS256" {
		t.Errorf("header validator received alg = %v, want HS256", headerValidator.received["alg"])
	}
}

func TestValidateHeaderValidatorError(t *testing.T) {
	t.Parallel()

	v := &Validator{HeaderValidator: &stubMapValidator{err: errStub}}
	tok := &token.Token{Header: map[string]any{"alg": "HS256"}, Payload: map[string]any{}}

	err := v.Validate(tok)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errStub) {
		t.Errorf("error = %v, want errStub", err)
	}
}

func TestValidatePayloadValidatorReceivesParsedClaims(t *testing.T) {
	t.Parallel()

	exp := float64(time.Now().Add(time.Hour).Unix())
	payloadValidator := &stubMapValidator{}
	v := &Validator{PayloadValidator: payloadValidator}
	tok := &token.Token{Payload: map[string]any{"exp": exp}}

	if err := v.Validate(tok); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !payloadValidator.called {
		t.Fatal("payload validator was not called")
	}
	parsedExp, ok := payloadValidator.received["exp"].(numeric_date.Date)
	if !ok {
		t.Fatalf("payload validator received exp = %T, want numeric_date.Date", payloadValidator.received["exp"])
	}
	if parsedExp.Unix() != int64(exp) {
		t.Errorf("parsed exp unix = %d, want %d", parsedExp.Unix(), int64(exp))
	}
}

func TestValidatePayloadValidatorError(t *testing.T) {
	t.Parallel()

	v := &Validator{PayloadValidator: &stubMapValidator{err: errStub}}
	tok := &token.Token{Payload: map[string]any{"sub": "user"}}

	err := v.Validate(tok)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errStub) {
		t.Errorf("error = %v, want errStub", err)
	}
}

func TestValidatePayloadParseError(t *testing.T) {
	t.Parallel()

	// A zero exp cannot be parsed into a numeric date, producing a parse error
	// before the payload validator is reached.
	payloadValidator := &stubMapValidator{}
	v := &Validator{PayloadValidator: payloadValidator}
	tok := &token.Token{Payload: map[string]any{"exp": float64(0)}}

	err := v.Validate(tok)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !errors.Is(err, altshiftErrors.ErrParseError) {
		t.Errorf("error = %v, want ErrParseError", err)
	}
	if payloadValidator.called {
		t.Error("payload validator should not have been called after parse error")
	}
}

func TestValidateIntegrationRegisteredClaimsValidator(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		exp       float64
		expectErr bool
	}{
		{name: "future exp passes", exp: float64(time.Now().Add(time.Hour).Unix())},
		{name: "past exp fails", exp: float64(time.Now().Add(-time.Hour).Unix()), expectErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			v := &Validator{PayloadValidator: &registered_claims_validator.Validator{}}
			tok := &token.Token{Payload: map[string]any{"exp": testCase.exp}}

			err := v.Validate(tok)
			if testCase.expectErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !errors.Is(err, jwtErrors.ErrExpExpired) {
					t.Errorf("error = %v, want ErrExpExpired", err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
