package task

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_tasks/types/http_request"
)

// View is how much of a task the server returns. A task's body may be large and is not always
// wanted, so the basic view leaves it out.
type View string

const (
	ViewUnspecified View = "VIEW_UNSPECIFIED"
	// ViewBasic omits the body and the headers, and is what a read returns unless asked otherwise.
	ViewBasic View = "BASIC"
	// ViewFull includes them, and needs the fuller permission to read.
	ViewFull View = "FULL"
)

// Attempt is what became of one dispatch.
type Attempt struct {
	ScheduleTime   string  `json:"scheduleTime,omitzero"`
	DispatchTime   string  `json:"dispatchTime,omitzero"`
	ResponseTime   string  `json:"responseTime,omitzero"`
	ResponseStatus *Status `json:"responseStatus,omitzero"`
}

// Status is an error in the shape Google's APIs report errors.
type Status struct {
	Code    int    `json:"code,omitzero"`
	Message string `json:"message,omitzero"`
}

// Task is one unit of work a queue will dispatch. Only the HTTP request is modelled: the API also
// dispatches to App Engine, and a type for something no caller sends is a type nobody checks.
type Task struct {
	// Name is the task's full resource name, projects/P/locations/L/queues/Q/tasks/T.
	//
	// It is optional, and naming a task is how the queue is asked to refuse a duplicate: a name
	// that has been used recently is rejected rather than dispatched a second time. That makes the
	// name worth deriving from what the work is about -- an entity and a domain, say -- rather than
	// from a counter.
	//
	// Two things come with that. The refusal outlives the task: a name cannot be reused for about
	// an hour after the task it belonged to was executed or deleted, so a named task is not a way
	// to ask for the same work twice in quick succession. And the API's own guidance is that names
	// spread evenly do better than names sharing a prefix, because the queue indexes by name and a
	// run of neighbours lands on one part of it.
	Name        string                `json:"name,omitzero"`
	HttpRequest *http_request.Request `json:"httpRequest,omitzero"`
	// ScheduleTime is when the task becomes eligible to run, RFC 3339. Left out, it is now.
	ScheduleTime string `json:"scheduleTime,omitzero"`
	// DispatchDeadline bounds one attempt, as seconds with a suffix, e.g. "1800s". A dispatch that
	// runs longer is cut off and counted as failed, so it has to outlast the slowest run. The
	// queue's own retry settings decide what happens next.
	DispatchDeadline string `json:"dispatchDeadline,omitzero"`

	// Reported by the server; ignored on create.
	CreateTime    string   `json:"createTime,omitzero"`
	DispatchCount int      `json:"dispatchCount,omitzero"`
	ResponseCount int      `json:"responseCount,omitzero"`
	FirstAttempt  *Attempt `json:"firstAttempt,omitzero"`
	LastAttempt   *Attempt `json:"lastAttempt,omitzero"`
	View          View     `json:"view,omitzero"`
}
