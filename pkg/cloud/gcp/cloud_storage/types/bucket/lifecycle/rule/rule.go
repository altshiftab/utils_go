package rule

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/types/bucket/lifecycle/rule/action"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_storage/types/bucket/lifecycle/rule/condition"
)

type Rule struct {
	Action    *action.Action       `json:"action,omitempty"`
	Condition *condition.Condition `json:"condition,omitempty"`
}
