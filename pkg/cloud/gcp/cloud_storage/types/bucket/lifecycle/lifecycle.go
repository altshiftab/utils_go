package lifecycle

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/types/bucket/lifecycle/rule"
)

type Lifecycle struct {
	Rule []*rule.Rule `json:"rule,omitempty"`
}
