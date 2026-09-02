package cloud_tasks

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_tasks/cloud_tasks_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_tasks/types/http_request"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_tasks/types/list_tasks_response"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_tasks/types/oidc_token"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_tasks/types/task"
)

// recorder is a stand-in for the API that records what it was asked and answers with what the test
// gave it.
type recorder struct {
	method string
	path   string
	body   []byte
}

func newClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	baseUrl, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("could not parse the server url: %v", err)
	}

	return NewClient(cloud_tasks_config.WithBaseUrl(baseUrl))
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
		name      string
		project   string
		location  string
		queue     string
		expected  string
		expectErr bool
	}{
		{
			name:     "a queue's full parent",
			project:  "a-project",
			location: "europe-north2",
			queue:    "discovery",
			expected: "projects/a-project/locations/europe-north2/queues/discovery",
		},
		{name: "no project", location: "europe-north2", queue: "discovery", expectErr: true},
		{name: "no location", project: "a-project", queue: "discovery", expectErr: true},
		{name: "no queue", project: "a-project", location: "europe-north2", expectErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			parent, err := Parent(testCase.project, testCase.location, testCase.queue)
			if testCase.expectErr {
				if err == nil {
					t.Fatalf("expected an error, got %q", parent)
				}

				return
			}

			if err != nil {
				t.Fatalf("did not expect an error: %v", err)
			}

			if parent != testCase.expected {
				t.Errorf("expected %q, got %q", testCase.expected, parent)
			}
		})
	}
}

func TestCreateTask(t *testing.T) {
	t.Parallel()

	var got recorder

	client := newClient(t, func(writer http.ResponseWriter, request *http.Request) {
		got.method = request.Method
		got.path = request.URL.Path
		body := make([]byte, request.ContentLength)
		_, _ = request.Body.Read(body)
		got.body = body

		writeJson(t, writer, &task.Task{
			Name:       "projects/a-project/locations/europe-north2/queues/discovery/tasks/a-task",
			CreateTime: "2026-09-02T10:00:00Z",
		})
	})

	parent := "projects/a-project/locations/europe-north2/queues/discovery"

	created, err := client.CreateTask(t.Context(), parent, "a-task", &task.Task{
		HttpRequest: &http_request.Request{
			Url:        "https://monitor.example/api/fetch/assets?entity=an-entity",
			HttpMethod: http.MethodPost,
			OidcToken: &oidc_token.Token{
				ServiceAccountEmail: "scheduler@a-project.iam.gserviceaccount.com",
				Audience:            "https://monitor.example",
			},
		},
		DispatchDeadline: "1800s",
	})
	if err != nil {
		t.Fatalf("did not expect an error: %v", err)
	}

	if got.method != http.MethodPost {
		t.Errorf("expected a POST, got %s", got.method)
	}

	if got.path != "/v2/"+parent+"/tasks" {
		t.Errorf("expected the tasks collection, got %s", got.path)
	}

	// Cloud Tasks wraps the resource, unlike Cloud Scheduler which is posted the job itself. A
	// client that posted the bare task would be answered with a validation error rather than a
	// created task, so the wrapper is worth pinning.
	var sent createTaskRequest
	if err := json.Unmarshal(got.body, &sent); err != nil {
		t.Fatalf("could not read what was sent: %v", err)
	}

	if sent.Task == nil {
		t.Fatal("the task should be sent wrapped in a task field")
	}

	// The id is sent as the name relative to the parent, which is how the API takes it, and is what
	// makes the queue refuse a duplicate.
	if sent.Task.Name != parent+"/tasks/a-task" {
		t.Errorf("expected the id as a full name, got %q", sent.Task.Name)
	}

	if sent.Task.HttpRequest == nil || sent.Task.HttpRequest.OidcToken == nil {
		t.Fatal("the request and its token should survive the round trip")
	}

	if sent.Task.HttpRequest.OidcToken.Audience != "https://monitor.example" {
		t.Errorf("expected the audience to be sent, got %q", sent.Task.HttpRequest.OidcToken.Audience)
	}

	if created.Name != "projects/a-project/locations/europe-north2/queues/discovery/tasks/a-task" {
		t.Errorf("expected the created task back, got %q", created.Name)
	}
}

// TestCreateTaskWithoutAnId holds that a task may be unnamed: the server names it, and every call
// queues new work rather than being refused as a duplicate.
func TestCreateTaskWithoutAnId(t *testing.T) {
	t.Parallel()

	var got recorder

	client := newClient(t, func(writer http.ResponseWriter, request *http.Request) {
		body := make([]byte, request.ContentLength)
		_, _ = request.Body.Read(body)
		got.body = body

		writeJson(t, writer, &task.Task{Name: "projects/p/locations/l/queues/q/tasks/1"})
	})

	if _, err := client.CreateTask(
		t.Context(),
		"projects/p/locations/l/queues/q",
		"",
		&task.Task{HttpRequest: &http_request.Request{Url: "https://monitor.example/"}},
	); err != nil {
		t.Fatalf("did not expect an error: %v", err)
	}

	var sent createTaskRequest
	if err := json.Unmarshal(got.body, &sent); err != nil {
		t.Fatalf("could not read what was sent: %v", err)
	}

	if sent.Task.Name != "" {
		t.Errorf("an unnamed task should be sent without a name, got %q", sent.Task.Name)
	}
}

// TestCreateTaskDuplicate is the answer a caller naming its tasks is asking for.
//
// The queue refusing a name it has seen means the work is already queued or has just been done.
// Reported as an ordinary failure it would read as the queue being broken, and a caller would have
// no way to tell "already handled" from "could not reach the API".
func TestCreateTaskDuplicate(t *testing.T) {
	t.Parallel()

	client := newClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusConflict)
		writeJson(t, writer, map[string]any{
			"error": map[string]any{"code": 409, "status": "ALREADY_EXISTS", "message": "Requested entity already exists"},
		})
	})

	_, err := client.CreateTask(
		t.Context(),
		"projects/p/locations/l/queues/q",
		"a-task",
		&task.Task{HttpRequest: &http_request.Request{Url: "https://monitor.example/"}},
	)

	if err == nil {
		t.Fatal("expected an error")
	}

	if !errors.Is(err, ErrTaskExists) {
		t.Errorf("expected a duplicate to be told apart from any other failure, got %v", err)
	}
}

// TestCreateTaskOtherFailuresAreNotDuplicates keeps the check narrow: only a conflict is a
// duplicate, so a queue that does not exist or a token that is not allowed still reads as broken.
func TestCreateTaskOtherFailuresAreNotDuplicates(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		statusCode int
	}{
		{name: "the queue is not there", statusCode: http.StatusNotFound},
		{name: "the caller may not", statusCode: http.StatusForbidden},
		{name: "the api is unwell", statusCode: http.StatusInternalServerError},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := newClient(t, func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(testCase.statusCode)
			})

			_, err := client.CreateTask(
				t.Context(),
				"projects/p/locations/l/queues/q",
				"a-task",
				&task.Task{HttpRequest: &http_request.Request{Url: "https://monitor.example/"}},
			)

			if err == nil {
				t.Fatal("expected an error")
			}

			if errors.Is(err, ErrTaskExists) {
				t.Errorf("a %d is not a duplicate", testCase.statusCode)
			}
		})
	}
}

func TestCreateTaskRefusesWhatItCannotSend(t *testing.T) {
	t.Parallel()

	client := newClient(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("nothing should have been sent")
	})

	if _, err := client.CreateTask(t.Context(), "", "a-task", &task.Task{}); err == nil {
		t.Error("a task needs a queue to go into")
	}

	if _, err := client.CreateTask(t.Context(), "projects/p/locations/l/queues/q", "a-task", nil); err == nil {
		t.Error("a nil task is a mistake in the code")
	}
}

func TestGetTask(t *testing.T) {
	t.Parallel()

	name := "projects/p/locations/l/queues/q/tasks/a-task"

	var got recorder

	client := newClient(t, func(writer http.ResponseWriter, request *http.Request) {
		got.method = request.Method
		got.path = request.URL.Path

		writeJson(t, writer, &task.Task{Name: name, DispatchCount: 2})
	})

	found, err := client.GetTask(t.Context(), name)
	if err != nil {
		t.Fatalf("did not expect an error: %v", err)
	}

	if got.method != http.MethodGet {
		t.Errorf("expected a GET, got %s", got.method)
	}

	if got.path != "/v2/"+name {
		t.Errorf("expected the task's own path, got %s", got.path)
	}

	if found.DispatchCount != 2 {
		t.Errorf("expected what the server said, got %d", found.DispatchCount)
	}
}

func TestDeleteTask(t *testing.T) {
	t.Parallel()

	name := "projects/p/locations/l/queues/q/tasks/a-task"

	var got recorder

	client := newClient(t, func(writer http.ResponseWriter, request *http.Request) {
		got.method = request.Method
		got.path = request.URL.Path

		writeJson(t, writer, struct{}{})
	})

	if err := client.DeleteTask(t.Context(), name); err != nil {
		t.Fatalf("did not expect an error: %v", err)
	}

	if got.method != http.MethodDelete {
		t.Errorf("expected a DELETE, got %s", got.method)
	}

	if got.path != "/v2/"+name {
		t.Errorf("expected the task's own path, got %s", got.path)
	}
}

func TestListTasksFollowsThePages(t *testing.T) {
	t.Parallel()

	parent := "projects/p/locations/l/queues/q"
	pages := 0

	client := newClient(t, func(writer http.ResponseWriter, request *http.Request) {
		pages++

		if request.URL.Query().Get("pageToken") == "" {
			writeJson(t, writer, &list_tasks_response.Response{
				Tasks:         []*task.Task{{Name: parent + "/tasks/1"}},
				NextPageToken: "second",
			})

			return
		}

		writeJson(t, writer, &list_tasks_response.Response{Tasks: []*task.Task{{Name: parent + "/tasks/2"}}})
	})

	tasks, err := client.ListTasks(t.Context(), parent)
	if err != nil {
		t.Fatalf("did not expect an error: %v", err)
	}

	if pages != 2 {
		t.Errorf("expected both pages to be read, got %d", pages)
	}

	if len(tasks) != 2 {
		t.Fatalf("expected both tasks, got %d", len(tasks))
	}
}

// TestListTasksStopsOnARepeatedToken holds the guard against a server that keeps handing back the
// same token: without it the walk would page forever.
func TestListTasksStopsOnARepeatedToken(t *testing.T) {
	t.Parallel()

	pages := 0

	client := newClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		pages++
		if pages > 10 {
			t.Fatal("the listing did not stop")
		}

		writeJson(t, writer, &list_tasks_response.Response{
			Tasks:         []*task.Task{{Name: "projects/p/locations/l/queues/q/tasks/1"}},
			NextPageToken: "always-the-same",
		})
	})

	if _, err := client.ListTasks(t.Context(), "projects/p/locations/l/queues/q"); err != nil {
		t.Fatalf("did not expect an error: %v", err)
	}

	if pages != 2 {
		t.Errorf("expected the walk to stop once the token repeated, got %d pages", pages)
	}
}

func TestResourceUrlEscapesSegments(t *testing.T) {
	t.Parallel()

	client := NewClient()

	// A queue or task id is a caller's string. One carrying a character that means something in a
	// path must not change which resource is addressed.
	urlString, err := client.resourceUrl("projects/p/locations/l/queues/a queue/tasks/a/task", "")
	if err != nil {
		t.Fatalf("did not expect an error: %v", err)
	}

	if want := fmt.Sprintf("https://%s/v2/projects/p/locations/l/queues/a%%20queue/tasks/a/task", Domain); urlString != want {
		t.Errorf("expected %q, got %q", want, urlString)
	}

	if _, err := client.resourceUrl("", ""); err == nil {
		t.Error("an empty resource is a mistake in the code")
	}
}
