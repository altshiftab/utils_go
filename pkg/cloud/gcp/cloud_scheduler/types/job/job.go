package job

import (
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_scheduler/types/http_target"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_scheduler/types/retry_config"
)

// State is what Cloud Scheduler reports about whether a job will run.
type State string

const (
	StateUnspecified State = "STATE_UNSPECIFIED"
	StateEnabled     State = "ENABLED"
	StatePaused      State = "PAUSED"
	// StateDisabled is a job Cloud Scheduler stopped running of its own accord, which it does after
	// enough consecutive failures. It is not the same as paused, which is asked for.
	StateDisabled State = "DISABLED"
	// StateUpdateFailed is a job whose last update did not take. It goes on running as it was.
	StateUpdateFailed State = "UPDATE_FAILED"
)

// Status is the error of the job's last attempt, in the shape Google's APIs report errors.
type Status struct {
	Code    int    `json:"code,omitzero"`
	Message string `json:"message,omitzero"`
}

// Job is one schedule. Only HTTP targets are modelled: the App Engine and Pub/Sub targets exist in
// the API and nothing here uses them.
type Job struct {
	// Name is the job's full resource name, projects/P/locations/L/jobs/J. It is assigned on
	// create -- the id is the last segment -- and is what every other call addresses the job by.
	Name        string `json:"name,omitzero"`
	Description string `json:"description,omitzero"`
	// Schedule is a unix cron expression, e.g. "0 3 * * *".
	Schedule string `json:"schedule,omitzero"`
	// TimeZone is what the schedule is read in, e.g. "Europe/Stockholm". It defaults to UTC, which
	// is worth setting deliberately: a schedule meant for a working day drifts by an hour twice a
	// year otherwise.
	TimeZone   string              `json:"timeZone,omitzero"`
	HttpTarget *http_target.Target `json:"httpTarget,omitzero"`
	// AttemptDeadline bounds one attempt. A target that runs longer than this is cut off and
	// counted as failed, so it has to outlast the slowest run.
	AttemptDeadline string               `json:"attemptDeadline,omitzero"`
	RetryConfig     *retry_config.Config `json:"retryConfig,omitzero"`

	// Reported by the server; ignored on create and update.
	State           State   `json:"state,omitzero"`
	Status          *Status `json:"status,omitzero"`
	ScheduleTime    string  `json:"scheduleTime,omitzero"`
	LastAttemptTime string  `json:"lastAttemptTime,omitzero"`
	UserUpdateTime  string  `json:"userUpdateTime,omitzero"`
}
