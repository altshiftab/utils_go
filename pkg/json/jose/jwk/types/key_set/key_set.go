package key_set

import "github.com/altshiftab/utils_go/pkg/json/jose/jwk/types/key"

type KeySet struct {
	Keys []*key.Key `json:"keys"`
}
