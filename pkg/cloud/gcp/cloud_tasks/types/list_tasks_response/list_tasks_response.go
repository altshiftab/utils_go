package list_tasks_response

import "github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_tasks/types/task"

// Response is one page of tasks.
type Response struct {
	Tasks []*task.Task `json:"tasks,omitzero"`
	// NextPageToken is empty on the last page.
	NextPageToken string `json:"nextPageToken,omitzero"`
}
