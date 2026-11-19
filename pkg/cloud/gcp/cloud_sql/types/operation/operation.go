// Package operation is a Cloud SQL Admin API operation: a long-running change to an instance, as
// an export, and how it went.
package operation

import (
	"strings"
)

const (
	StatusPending = "PENDING"
	StatusRunning = "RUNNING"
	StatusDone    = "DONE"

	TypeExport = "EXPORT"
)

type OperationError struct {
	Kind    string `json:"kind,omitzero"`
	Code    string `json:"code,omitzero"`
	Message string `json:"message,omitzero"`
}

type OperationErrors struct {
	Kind   string            `json:"kind,omitzero"`
	Errors []*OperationError `json:"errors,omitzero"`
}

type Operation struct {
	Kind string `json:"kind,omitzero"`
	// Name identifies the operation within its project.
	Name          string `json:"name"`
	Status        string `json:"status,omitzero"`
	OperationType string `json:"operationType,omitzero"`
	TargetId      string `json:"targetId,omitzero"`
	TargetProject string `json:"targetProject,omitzero"`
	InsertTime    string `json:"insertTime,omitzero"`
	StartTime     string `json:"startTime,omitzero"`
	EndTime       string `json:"endTime,omitzero"`
	// Error is set on an operation that is done but failed.
	Error *OperationErrors `json:"error,omitzero"`
}

// Done reports whether the operation has finished, successfully or not.
func (o *Operation) Done() bool {
	return o != nil && o.Status == StatusDone
}

// Failure describes why a finished operation failed, or is empty for one that did not.
func (o *Operation) Failure() string {
	if o == nil || o.Error == nil || len(o.Error.Errors) == 0 {
		return ""
	}

	descriptions := make([]string, 0, len(o.Error.Errors))
	for _, operationError := range o.Error.Errors {
		if operationError != nil {
			descriptions = append(descriptions, operationError.Code+": "+operationError.Message)
		}
	}

	return strings.Join(descriptions, "; ")
}
