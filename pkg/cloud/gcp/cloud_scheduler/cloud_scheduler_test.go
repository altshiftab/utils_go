package cloud_scheduler

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_scheduler/cloud_scheduler_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_scheduler/types/http_target"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_scheduler/types/job"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_scheduler/types/list_jobs_response"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_scheduler/types/oidc_token"
)

// recorder is a stand-in for the API that records what it was asked and answers with what the test
// gave it.
type recorder struct {
	method string
	path   string
	query  url.Values
	body   []byte
}

func newClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	baseUrl, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("could not parse the server url: %v", err)
	}

	return NewClient(cloud_scheduler_config.WithBaseUrl(baseUrl)), server
}

func writeJson(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("could not marshal the response: %v", err)
	}

	writer.Header().Set("Content-Type", "application/json")
	if _, err := writer.Write(data); err != nil {
		t.Fatalf("could not write the response: %v", err)
	}
}

func TestParent(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		project     string
		location    string
		expect      string
		expectError bool
	}{
		{
			name:     "a project and a location",
			project:  "a-project",
			location: "europe-north2",
			expect:   "projects/a-project/locations/europe-north2",
		},
		{name: "an empty project is an error", location: "europe-north2", expectError: true},
		{name: "an empty location is an error", project: "a-project", expectError: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := Parent(testCase.project, testCase.location)

			if testCase.expectError {
				if err == nil {
					t.Fatalf("%s: expected an error, got none", testCase.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", testCase.name, err)
			}

			if got != testCase.expect {
				t.Errorf("%s: expected %q, got %q", testCase.name, testCase.expect, got)
			}
		})
	}
}

func TestCreateJob(t *testing.T) {
	t.Parallel()

	var recorded recorder
	client, _ := newClient(t, func(writer http.ResponseWriter, request *http.Request) {
		recorded.method = request.Method
		recorded.path = request.URL.Path
		body := make([]byte, request.ContentLength)
		_, _ = request.Body.Read(body)
		recorded.body = body

		writeJson(t, writer, &job.Job{
			Name:     "projects/a-project/locations/europe-north2/jobs/fetch-leaks",
			Schedule: "0 3 * * *",
			State:    job.StateEnabled,
		})
	})

	created, err := client.CreateJob(
		t.Context(),
		"projects/a-project/locations/europe-north2",
		"fetch-leaks",
		&job.Job{
			Schedule: "0 3 * * *",
			TimeZone: "Europe/Stockholm",
			HttpTarget: &http_target.Target{
				Uri:        "https://monitor.example.com/api/fetch/leaks?target=example.com",
				HttpMethod: http.MethodPost,
				OidcToken: &oidc_token.Token{
					ServiceAccountEmail: "scheduler@a-project.iam.gserviceaccount.com",
					Audience:            "https://monitor.example.com",
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recorded.method != http.MethodPost {
		t.Errorf("expected a POST, got %s", recorded.method)
	}
	// The id goes in the body as the job's name, not in the path; the path is the parent's jobs
	// collection.
	if expected := "/v1/projects/a-project/locations/europe-north2/jobs"; recorded.path != expected {
		t.Errorf("expected the path %q, got %q", expected, recorded.path)
	}

	var sent job.Job
	if err := json.Unmarshal(recorded.body, &sent); err != nil {
		t.Fatalf("could not unmarshal what was sent: %v (%q)", err, recorded.body)
	}
	if expected := "projects/a-project/locations/europe-north2/jobs/fetch-leaks"; sent.Name != expected {
		t.Errorf("expected the job's name to carry the id, got %q", sent.Name)
	}
	if sent.HttpTarget == nil || sent.HttpTarget.OidcToken == nil {
		t.Fatalf("expected the OIDC token to be sent, got %+v", sent.HttpTarget)
	}
	// The audience is what the monitor checks the aud claim against, so it has to survive.
	if expected := "https://monitor.example.com"; sent.HttpTarget.OidcToken.Audience != expected {
		t.Errorf("expected the audience %q, got %q", expected, sent.HttpTarget.OidcToken.Audience)
	}

	if created.State != job.StateEnabled {
		t.Errorf("expected the state the server reported, got %q", created.State)
	}
}

func TestCreateJobArgumentChecks(t *testing.T) {
	t.Parallel()

	client, _ := newClient(t, func(http.ResponseWriter, *http.Request) {})

	testCases := []struct {
		name   string
		parent string
		jobId  string
		job    *job.Job
	}{
		{name: "an empty parent is an error", jobId: "a-job", job: &job.Job{}},
		{name: "an empty id is an error", parent: "projects/p/locations/l", job: &job.Job{}},
		{name: "a nil job is an error", parent: "projects/p/locations/l", jobId: "a-job"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := client.CreateJob(t.Context(), testCase.parent, testCase.jobId, testCase.job); err == nil {
				t.Errorf("%s: expected an error, got none", testCase.name)
			}
		})
	}
}

// TestListJobsFollowsPages holds the paging: a location with more jobs than fit in a page has to be
// read to the end rather than reported as whatever the first page held.
func TestListJobsFollowsPages(t *testing.T) {
	t.Parallel()

	pages := map[string]*list_jobs_response.Response{
		"": {
			Jobs:          []*job.Job{{Name: "jobs/one"}, {Name: "jobs/two"}},
			NextPageToken: "second",
		},
		"second": {Jobs: []*job.Job{{Name: "jobs/three"}}},
	}

	var sizes []string
	client, _ := newClient(t, func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		sizes = append(sizes, query.Get("pageSize"))

		page, ok := pages[query.Get("pageToken")]
		if !ok {
			t.Errorf("asked for an unknown page token %q", query.Get("pageToken"))
			return
		}
		writeJson(t, writer, page)
	})

	jobs, err := client.ListJobs(t.Context(), "projects/a-project/locations/europe-north2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(jobs) != 3 {
		t.Fatalf("expected the jobs of both pages, got %d", len(jobs))
	}
	if jobs[0].Name != "jobs/one" || jobs[2].Name != "jobs/three" {
		t.Errorf("expected the pages in order, got %q first and %q last", jobs[0].Name, jobs[2].Name)
	}
	for _, size := range sizes {
		if size != fmt.Sprintf("%d", maxPageSize) {
			t.Errorf("expected the largest page the API serves, got %q", size)
		}
	}
}

// TestListJobsStopsOnARepeatedToken holds the guard against an API that keeps handing back the same
// token: without it the listing would ask for that page forever.
func TestListJobsStopsOnARepeatedToken(t *testing.T) {
	t.Parallel()

	calls := 0
	client, _ := newClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		calls++
		if calls > 5 {
			t.Fatal("the listing did not stop on a repeated page token")
		}
		writeJson(t, writer, &list_jobs_response.Response{
			Jobs:          []*job.Job{{Name: "jobs/one"}},
			NextPageToken: "same",
		})
	})

	jobs, err := client.ListJobs(t.Context(), "projects/a-project/locations/europe-north2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The first page is kept, the repeat is not followed.
	if len(jobs) != 2 {
		t.Errorf("expected the two pages read before the repeat was noticed, got %d", len(jobs))
	}
}

func TestPatchJob(t *testing.T) {
	t.Parallel()

	var recorded recorder
	client, _ := newClient(t, func(writer http.ResponseWriter, request *http.Request) {
		recorded.method = request.Method
		recorded.path = request.URL.Path
		recorded.query = request.URL.Query()
		writeJson(t, writer, &job.Job{Name: "projects/p/locations/l/jobs/a-job", Schedule: "0 4 * * *"})
	})

	if _, err := client.PatchJob(
		t.Context(),
		"projects/p/locations/l/jobs/a-job",
		&job.Job{Schedule: "0 4 * * *"},
		[]string{"schedule", "timeZone"},
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recorded.method != http.MethodPatch {
		t.Errorf("expected a PATCH, got %s", recorded.method)
	}
	if expected := "/v1/projects/p/locations/l/jobs/a-job"; recorded.path != expected {
		t.Errorf("expected the path %q, got %q", expected, recorded.path)
	}
	// Without the mask the API replaces the whole job with what was sent, so it has to be there.
	if expected := "schedule,timeZone"; recorded.query.Get("updateMask") != expected {
		t.Errorf("expected the update mask %q, got %q", expected, recorded.query.Get("updateMask"))
	}
}

// TestPatchJobRequiresAMask holds the guard: patching without one silently replaces the job, so an
// empty mask is refused here rather than sent.
func TestPatchJobRequiresAMask(t *testing.T) {
	t.Parallel()

	client, _ := newClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a patch with no mask reached the API")
	})

	if _, err := client.PatchJob(
		t.Context(),
		"projects/p/locations/l/jobs/a-job",
		&job.Job{Schedule: "0 4 * * *"},
		nil,
	); err == nil {
		t.Error("expected an error for an empty update mask, got none")
	}
}

func TestCustomVerbs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		call       func(client *Client) (*job.Job, error)
		expectPath string
	}{
		{
			name:       "pause",
			call:       func(c *Client) (*job.Job, error) { return c.PauseJob(t.Context(), "projects/p/locations/l/jobs/a-job") },
			expectPath: "/v1/projects/p/locations/l/jobs/a-job:pause",
		},
		{
			name: "resume",
			call: func(c *Client) (*job.Job, error) {
				return c.ResumeJob(t.Context(), "projects/p/locations/l/jobs/a-job")
			},
			expectPath: "/v1/projects/p/locations/l/jobs/a-job:resume",
		},
		{
			name:       "run",
			call:       func(c *Client) (*job.Job, error) { return c.RunJob(t.Context(), "projects/p/locations/l/jobs/a-job") },
			expectPath: "/v1/projects/p/locations/l/jobs/a-job:run",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var recorded recorder
			client, _ := newClient(t, func(writer http.ResponseWriter, request *http.Request) {
				recorded.method = request.Method
				recorded.path = request.URL.Path
				writeJson(t, writer, &job.Job{Name: "projects/p/locations/l/jobs/a-job"})
			})

			if _, err := testCase.call(client); err != nil {
				t.Fatalf("%s: unexpected error: %v", testCase.name, err)
			}

			if recorded.method != http.MethodPost {
				t.Errorf("%s: expected a POST, got %s", testCase.name, recorded.method)
			}
			// The verb is appended to the resource name rather than being a path segment, which is
			// what would happen if it were escaped along with the name.
			if recorded.path != testCase.expectPath {
				t.Errorf("%s: expected the path %q, got %q", testCase.name, testCase.expectPath, recorded.path)
			}
		})
	}
}

func TestDeleteJob(t *testing.T) {
	t.Parallel()

	var recorded recorder
	client, _ := newClient(t, func(writer http.ResponseWriter, request *http.Request) {
		recorded.method = request.Method
		recorded.path = request.URL.Path
		writer.WriteHeader(http.StatusOK)
	})

	if err := client.DeleteJob(t.Context(), "projects/p/locations/l/jobs/a-job"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recorded.method != http.MethodDelete {
		t.Errorf("expected a DELETE, got %s", recorded.method)
	}
	if expected := "/v1/projects/p/locations/l/jobs/a-job"; recorded.path != expected {
		t.Errorf("expected the path %q, got %q", expected, recorded.path)
	}
}

func TestGetJob(t *testing.T) {
	t.Parallel()

	var recorded recorder
	client, _ := newClient(t, func(writer http.ResponseWriter, request *http.Request) {
		recorded.method = request.Method
		recorded.path = request.URL.Path
		writeJson(t, writer, &job.Job{Name: "projects/p/locations/l/jobs/a-job", State: job.StatePaused})
	})

	got, err := client.GetJob(t.Context(), "projects/p/locations/l/jobs/a-job")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recorded.method != http.MethodGet {
		t.Errorf("expected a GET, got %s", recorded.method)
	}
	if got.State != job.StatePaused {
		t.Errorf("expected the state the server reported, got %q", got.State)
	}
}

// TestEmptyResourceIsAnError holds that a call with no name does not reach the API as a request for
// the whole collection.
func TestEmptyResourceIsAnError(t *testing.T) {
	t.Parallel()

	client, _ := newClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call with an empty resource name reached the API")
	})

	if _, err := client.GetJob(t.Context(), ""); err == nil {
		t.Error("expected an error for an empty name, got none")
	}
	if err := client.DeleteJob(t.Context(), ""); err == nil {
		t.Error("expected an error for an empty name, got none")
	}
	if _, err := client.ListJobs(t.Context(), ""); err == nil {
		t.Error("expected an error for an empty parent, got none")
	}
}
