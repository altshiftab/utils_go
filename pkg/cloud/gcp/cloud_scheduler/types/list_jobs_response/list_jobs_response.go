package list_jobs_response

import "github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_scheduler/types/job"

// Response is one page of a jobs.list call.
type Response struct {
	Jobs []*job.Job `json:"jobs,omitzero"`
	// NextPageToken is empty on the last page. It is valid for two hours.
	NextPageToken string `json:"nextPageToken,omitzero"`
}
