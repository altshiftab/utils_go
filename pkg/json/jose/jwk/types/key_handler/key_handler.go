package key_handler

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	motmedelCryptoInterfaces "github.com/altshiftab/utils_go/pkg/crypto/interfaces"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	motmedelHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
	jwkKey "github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key"
	"github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key_handler/key_handler_config"
	"github.com/altshiftab/utils_go/pkg/utils"
)

type Handler struct {
	JwkUrl *url.URL
	config *key_handler_config.Config

	keysMutex     sync.Mutex
	keys          []map[string]any
	keysExpiresAt *time.Time

	mu              sync.RWMutex
	keyIdToVerifier map[string]motmedelCryptoInterfaces.NamedVerifier
}

func (h *Handler) GetNamedVerifier(ctx context.Context, keyId string) (motmedelCryptoInterfaces.NamedVerifier, error) {
	h.keysMutex.Lock()
	err := func() error {
		defer h.keysMutex.Unlock()
		if expiresAt := h.keysExpiresAt; expiresAt == nil || expiresAt.Before(time.Now()) {
			jwkUrl := h.JwkUrl
			if jwkUrl == nil {
				return motmedelErrors.NewWithTrace(nil_error.NewWithInstance("url", "jwk url"))
			}

			urlString := jwkUrl.String()
			response, keysResponseData, err := motmedelHttpUtils.FetchJson[map[string]any](ctx, urlString, h.config.FetchOptions...)
			if err != nil {
				return motmedelErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
			}

			keys, err := utils.MapGetConvertSlice[map[string]any](keysResponseData, "keys")
			if err != nil {
				return motmedelErrors.New(fmt.Errorf("map get convert: %w", err), keysResponseData)
			}

			h.keys = keys

			responseHeader := response.Header

			// Prefer Cache-Control: max-age over Expires
			cacheControlValue, ccErr := motmedelHttpUtils.GetSingleHeader("Cache-Control", responseHeader)
			usedCacheControl := false
			if ccErr == nil && cacheControlValue != "" {
				directives := strings.Split(cacheControlValue, ",")
				for _, d := range directives {
					d = strings.TrimSpace(strings.ToLower(d))
					if strings.HasPrefix(d, "max-age=") {
						v := strings.TrimSpace(strings.TrimPrefix(d, "max-age="))
						if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
							maxAgeExpiresAt := time.Now().Add(time.Duration(secs) * time.Second)
							h.keysExpiresAt = &maxAgeExpiresAt
							usedCacheControl = true
						}
						break
					}
				}
			}

			if !usedCacheControl {
				// Fallback to Expires header (RFC1123)
				expiresValue, err := motmedelHttpUtils.GetSingleHeader("Expires", responseHeader)
				if err != nil {
					return motmedelErrors.New(fmt.Errorf("get expires header: %w", err), responseHeader)
				}

				headerValueExpiresAt, err := time.Parse(time.RFC1123, expiresValue)
				if err != nil {
					return motmedelErrors.NewWithTrace(fmt.Errorf("time parse (expires): %w", err), expiresValue)
				}
				h.keysExpiresAt = &headerValueExpiresAt
			}

			h.mu.Lock()
			clear(h.keyIdToVerifier)
			h.mu.Unlock()
		}

		return nil
	}()
	if err != nil {
		return nil, err
	}

	h.mu.RLock()
	if verifier, ok := h.keyIdToVerifier[keyId]; ok {
		h.mu.RUnlock()
		return verifier, nil
	}
	h.mu.RUnlock()

	keys := h.keys
	for _, keyMap := range keys {
		if keyMap == nil {
			continue
		}

		keyMapKeyId := keyMap["kid"]
		if keyMapKeyId != keyId {
			continue
		}

		key, err := jwkKey.New(keyMap)
		if err != nil {
			return nil, motmedelErrors.New(fmt.Errorf("new key: %w", err), keyMap)
		}
		if key == nil {
			return nil, motmedelErrors.NewWithTrace(nil_error.New("key"))
		}

		namedVerifier, err := key.NamedVerifier()
		if err != nil {
			return nil, motmedelErrors.New(fmt.Errorf("key named verifier: %w", err), key)
		}
		if utils.IsNil(namedVerifier) {
			return nil, motmedelErrors.NewWithTrace(nil_error.New("verifier"))
		}

		h.mu.Lock()
		h.keyIdToVerifier[keyId] = namedVerifier
		h.mu.Unlock()

		return namedVerifier, nil
	}

	return nil, nil
}

func New(jwkUrl *url.URL, options ...key_handler_config.Option) (*Handler, error) {
	if jwkUrl == nil {
		return nil, motmedelErrors.NewWithTrace(nil_error.NewWithInstance("url", "jwk url"))
	}

	return &Handler{
		JwkUrl:          jwkUrl,
		keyIdToVerifier: make(map[string]motmedelCryptoInterfaces.NamedVerifier),
		config:          key_handler_config.New(options...),
	}, nil
}
