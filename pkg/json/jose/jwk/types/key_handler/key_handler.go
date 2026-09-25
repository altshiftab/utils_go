package key_handler

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	altshiftCryptoInterfaces "github.com/altshiftab/utils_go/pkg/crypto/interfaces"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
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
	keysFetchedAt *time.Time

	mu              sync.RWMutex
	keyIdToVerifier map[string]altshiftCryptoInterfaces.NamedVerifier
}

// GetNamedVerifier returns the verifier for the key the set names keyId, or nil when the set holds
// no such key. A miss sends the handler back for the set once more, no more often than the
// configured interval, since the key may be one rotated in since the cached copy was fetched.
func (h *Handler) GetNamedVerifier(ctx context.Context, keyId string) (altshiftCryptoInterfaces.NamedVerifier, error) {
	keys, err := h.getKeys(ctx, false)
	if err != nil {
		return nil, err
	}

	verifier, err := h.lookup(keys, keyId)
	if err != nil || verifier != nil {
		return verifier, err
	}

	keys, err = h.getKeys(ctx, true)
	if err != nil {
		return nil, err
	}

	return h.lookup(keys, keyId)
}

// getKeys returns the cached key set, fetching it first when it has expired or, for a key id it
// did not hold, when the last fetch is older than the refetch interval. The slice is read under the
// same lock a fetch writes it under.
func (h *Handler) getKeys(ctx context.Context, unknownKeyId bool) ([]map[string]any, error) {
	h.keysMutex.Lock()
	defer h.keysMutex.Unlock()

	now := time.Now()
	expired := h.keysExpiresAt == nil || h.keysExpiresAt.Before(now)
	refetch := unknownKeyId && (h.keysFetchedAt == nil || now.Sub(*h.keysFetchedAt) >= h.config.UnknownKeyIdRefetchInterval)
	if expired || refetch {
		if err := h.fetchKeys(ctx); err != nil {
			return nil, err
		}
	}

	return h.keys, nil
}

// fetchKeys fetches the key set and the time it may be cached until. The caller holds keysMutex.
func (h *Handler) fetchKeys(ctx context.Context) error {
	jwkUrl := h.JwkUrl
	if jwkUrl == nil {
		return altshiftErrors.NewWithTrace(nil_error.NewWithInstance("url", "jwk url"))
	}

	urlString := jwkUrl.String()
	response, keysResponseData, err := altshiftHttpUtils.FetchJson[map[string]any](ctx, urlString, h.config.FetchOptions...)
	if err != nil {
		return altshiftErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
	}

	keys, err := utils.MapGetConvertSlice[map[string]any](keysResponseData, "keys")
	if err != nil {
		return altshiftErrors.New(fmt.Errorf("map get convert: %w", err), keysResponseData)
	}

	if response == nil {
		return altshiftErrors.NewWithTrace(nil_error.New("http response"))
	}

	h.keys = keys

	responseHeader := response.Header

	// Prefer Cache-Control: max-age over Expires
	cacheControlValue, ccErr := altshiftHttpUtils.GetSingleHeader("Cache-Control", responseHeader)
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
		expiresValue, err := altshiftHttpUtils.GetSingleHeader("Expires", responseHeader)
		if err != nil {
			return altshiftErrors.New(fmt.Errorf("get expires header: %w", err), responseHeader)
		}

		headerValueExpiresAt, err := time.Parse(time.RFC1123, expiresValue)
		if err != nil {
			return altshiftErrors.NewWithTrace(fmt.Errorf("time parse (expires): %w", err), expiresValue)
		}
		h.keysExpiresAt = &headerValueExpiresAt
	}

	h.mu.Lock()
	clear(h.keyIdToVerifier)
	h.mu.Unlock()

	fetchedAt := time.Now()
	h.keysFetchedAt = &fetchedAt

	return nil
}

// lookup finds keyId in keys, building and caching its verifier; nil when keys holds no such key.
func (h *Handler) lookup(keys []map[string]any, keyId string) (altshiftCryptoInterfaces.NamedVerifier, error) {
	h.mu.RLock()
	if verifier, ok := h.keyIdToVerifier[keyId]; ok {
		h.mu.RUnlock()
		return verifier, nil
	}
	h.mu.RUnlock()

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
			return nil, altshiftErrors.New(fmt.Errorf("new key: %w", err), keyMap)
		}
		if key == nil {
			return nil, altshiftErrors.NewWithTrace(nil_error.New("key"))
		}

		namedVerifier, err := key.NamedVerifier()
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("key named verifier: %w", err), key)
		}
		if utils.IsNil(namedVerifier) {
			return nil, altshiftErrors.NewWithTrace(nil_error.New("verifier"))
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
		return nil, altshiftErrors.NewWithTrace(nil_error.NewWithInstance("url", "jwk url"))
	}

	return &Handler{
		JwkUrl:          jwkUrl,
		keyIdToVerifier: make(map[string]altshiftCryptoInterfaces.NamedVerifier),
		config:          key_handler_config.New(options...),
	}, nil
}
