package cose_body_parser

import (
	"bytes"
	"crypto/ecdh"
	"errors"
	"fmt"
	"net/http"

	"github.com/altshiftab/utils_go/pkg/cose"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
)

type Option func(*Parser)

// WithKeyIdentifier requires messages to carry a matching recipient key identifier.
func WithKeyIdentifier(keyIdentifier []byte) Option {
	return func(parser *Parser) {
		parser.keyIdentifier = keyIdentifier
	}
}

// WithPlaintextContentType requires the content protected header's content type to match.
func WithPlaintextContentType(contentType string) Option {
	return func(parser *Parser) {
		parser.plaintextContentType = contentType
	}
}

// Parser decrypts COSE_Encrypt request bodies, returning the plaintext.
type Parser struct {
	privateKey           *ecdh.PrivateKey
	keyIdentifier        []byte
	plaintextContentType string
}

func (p *Parser) Parse(_ *http.Request, body []byte) ([]byte, *response_error.ResponseError) {
	privateKey := p.privateKey
	if privateKey == nil {
		return nil, &response_error.ResponseError{
			ServerError: motmedelErrors.NewWithTrace(nil_error.New("private key")),
		}
	}

	result, err := cose.Decrypt(body, privateKey, nil)
	if err != nil {
		wrappedErr := motmedelErrors.NewWithTrace(fmt.Errorf("cose decrypt: %w", err))

		switch {
		case errors.Is(err, cose.ErrMalformedMessage), errors.Is(err, cose.ErrUnsupportedAlgorithm):
			return nil, &response_error.ResponseError{
				ClientError: wrappedErr,
				ProblemDetail: problem_detail.New(
					http.StatusBadRequest,
					problem_detail_config.WithDetail("Malformed COSE message."),
				),
			}
		case errors.Is(err, cose.ErrNoUsableRecipient):
			return nil, &response_error.ResponseError{
				ClientError: wrappedErr,
				ProblemDetail: problem_detail.New(
					http.StatusBadRequest,
					problem_detail_config.WithDetail("The request body could not be decrypted."),
				),
			}
		default:
			return nil, &response_error.ResponseError{ServerError: wrappedErr}
		}
	}

	if keyIdentifier := p.keyIdentifier; len(keyIdentifier) > 0 {
		if !bytes.Equal(result.KeyIdentifier, keyIdentifier) {
			return nil, &response_error.ResponseError{
				ProblemDetail: problem_detail.New(
					http.StatusBadRequest,
					problem_detail_config.WithDetail(
						"The key identifier in the message does not match the identifier of the key in use.",
					),
					problem_detail_config.WithExtension(
						map[string]any{"kid": string(result.KeyIdentifier)},
					),
				),
			}
		}
	}

	if plaintextContentType := p.plaintextContentType; plaintextContentType != "" {
		contentType, ok := result.ContentType.(string)
		if !ok || contentType != plaintextContentType {
			return nil, &response_error.ResponseError{
				ProblemDetail: problem_detail.New(
					http.StatusBadRequest,
					problem_detail_config.WithDetail("Unexpected plaintext content type."),
					problem_detail_config.WithExtension(
						map[string]any{"content_type": fmt.Sprintf("%v", result.ContentType)},
					),
				),
			}
		}
	}

	return result.Plaintext, nil
}

func New(privateKey *ecdh.PrivateKey, options ...Option) (*Parser, error) {
	if privateKey == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.New("private key"))
	}

	parser := &Parser{privateKey: privateKey}
	for _, option := range options {
		if option != nil {
			option(parser)
		}
	}

	return parser, nil
}
