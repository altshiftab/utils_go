package client_side_encryption

import (
	"crypto/ecdsa"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/request_parser/header_extractor"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/response_error"
	"github.com/altshiftab/utils_go/pkg/http/mux/utils/client_side_encryption/body_parser_config"
	"github.com/altshiftab/utils_go/pkg/http/mux/utils/client_side_encryption/header_parser_config"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail"
	"github.com/altshiftab/utils_go/pkg/http/types/problem_detail/problem_detail_config"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwe"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type HeaderParser struct {
	headerExtractor   *header_extractor.Parser
	KeyAlgorithm      jwe.KeyAlgorithm
	ContentEncryption jwe.ContentEncryption
	ContentType       string
}

func (p *HeaderParser) Parse(request *http.Request) (*jwe.Encrypter, *response_error.ResponseError) {
	clientJwkRaw, responseError := p.headerExtractor.Parse(request)
	if responseError != nil {
		return nil, responseError
	}

	var clientJwkMap map[string]any
	if err := json.Unmarshal([]byte(clientJwkRaw), &clientJwkMap); err != nil {
		return nil, &response_error.ResponseError{
			ClientError: altshiftErrors.NewWithTrace(
				fmt.Errorf("json unmarshal (client jwk): %w", err),
				clientJwkRaw,
			),
			ProblemDetail: problem_detail.New(
				http.StatusBadRequest,
				problem_detail_config.WithDetail("Invalid JWK header."),
			),
		}
	}

	clientJwk, err := key.New(clientJwkMap)
	if err != nil {
		return nil, &response_error.ResponseError{
			ClientError: altshiftErrors.New(fmt.Errorf("key new (client jwk): %w", err), clientJwkMap),
			ProblemDetail: problem_detail.New(
				http.StatusBadRequest,
				problem_detail_config.WithDetail("Invalid JWK header."),
			),
		}
	}
	if clientJwk == nil || utils.IsNil(clientJwk.Material) {
		return nil, &response_error.ResponseError{
			ProblemDetail: problem_detail.New(
				http.StatusBadRequest,
				problem_detail_config.WithDetail("Missing client public JWK key."),
			),
		}
	}

	clientPublicKey, err := clientJwk.Material.PublicKey()
	if err != nil {
		return nil, &response_error.ResponseError{
			ClientError: altshiftErrors.New(fmt.Errorf("public key (client jwk): %w", err), clientJwk),
			ProblemDetail: problem_detail.New(
				http.StatusBadRequest,
				problem_detail_config.WithDetail("Malformed JWK key."),
			),
		}
	}

	clientEcdsaPublicKey, ok := clientPublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, &response_error.ResponseError{
			ClientError: altshiftErrors.NewWithTrace(
				fmt.Errorf("%w (client jwk): %T", jwe.ErrUnsupportedKeyType, clientPublicKey),
			),
			ProblemDetail: problem_detail.New(
				http.StatusBadRequest,
				problem_detail_config.WithDetail("Malformed JWK key."),
			),
		}
	}

	responseEncrypter, err := jwe.NewEncrypter(p.KeyAlgorithm, p.ContentEncryption, clientEcdsaPublicKey)
	if err != nil {
		return nil, &response_error.ResponseError{
			ClientError: altshiftErrors.New(
				fmt.Errorf("jwe new encrypter: %w", err),
				clientEcdsaPublicKey,
			),
			ProblemDetail: problem_detail.New(
				http.StatusBadRequest,
				problem_detail_config.WithDetail("Malformed JWK key."),
			),
		}
	}

	responseEncrypter.KeyId = clientJwk.Kid
	responseEncrypter.ContentType = p.ContentType

	return responseEncrypter, nil
}

func NewHeaderParser(options ...header_parser_config.Option) (*HeaderParser, error) {
	config := header_parser_config.New(options...)

	headerExtractor, err := header_extractor.New(config.HeaderName)
	if err != nil {
		return nil, fmt.Errorf("header extractor new: %w", err)
	}

	return &HeaderParser{
		headerExtractor:   headerExtractor,
		KeyAlgorithm:      config.KeyAlgorithm,
		ContentEncryption: config.ContentEncryption,
		ContentType:       config.ContentType,
	}, nil
}

type BodyParser struct {
	PrivateKey        any
	KeyIdentifier     string
	KeyAlgorithm      jwe.KeyAlgorithm
	ContentEncryption jwe.ContentEncryption
}

func (p *BodyParser) Parse(_ *http.Request, body []byte) ([]byte, *response_error.ResponseError) {
	encryption, err := jwe.ParseCompact(
		string(body),
		[]jwe.KeyAlgorithm{p.KeyAlgorithm},
		[]jwe.ContentEncryption{p.ContentEncryption},
	)
	if err != nil {
		return nil, &response_error.ResponseError{
			ClientError: altshiftErrors.New(
				fmt.Errorf("jwe parse compact: %w", err),
				body, p.KeyAlgorithm, p.ContentEncryption,
			),
		}
	}

	if keyIdentifier := p.KeyIdentifier; keyIdentifier != "" {
		jweKeyIdentifier := encryption.Header.KeyId
		if jweKeyIdentifier != keyIdentifier {
			return nil, &response_error.ResponseError{
				ProblemDetail: problem_detail.New(
					http.StatusBadRequest,
					problem_detail_config.WithDetail("The key identifier in the JWE header does not match the identifier of the key in use."),
					problem_detail_config.WithExtension(map[string]any{"kid": jweKeyIdentifier}),
				),
			}
		}
	}

	plaintext, err := encryption.Decrypt(p.PrivateKey)
	if err != nil {
		wrappedErr := altshiftErrors.New(fmt.Errorf("jwe decrypt: %w", err))

		if errors.Is(err, altshiftErrors.ErrVerificationError) || errors.Is(err, altshiftErrors.ErrValidationError) {
			return nil, &response_error.ResponseError{
				ClientError: wrappedErr,
				ProblemDetail: problem_detail.New(
					http.StatusBadRequest,
					problem_detail_config.WithDetail("The request body could not be decrypted."),
				),
			}
		}

		return nil, &response_error.ResponseError{ServerError: wrappedErr}
	}

	return plaintext, nil
}

func NewBodyParser(privateKey any, options ...body_parser_config.Option) (*BodyParser, error) {
	if utils.IsNil(privateKey) {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("private key"))
	}

	config := body_parser_config.New(options...)

	return &BodyParser{
		PrivateKey:        privateKey,
		KeyIdentifier:     config.KeyIdentifier,
		KeyAlgorithm:      config.KeyAlgorithm,
		ContentEncryption: config.ContentEncryption,
	}, nil
}
