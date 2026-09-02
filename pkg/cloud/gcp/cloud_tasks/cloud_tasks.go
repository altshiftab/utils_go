// Package cloud_tasks creates and reads Cloud Tasks tasks over the REST API.
//
// Only the HTTP request is modelled. The API also dispatches to App Engine; nothing here uses that,
// and a type for something no caller sends is a type nobody checks.
//
// The queue itself is not managed here. A queue is infrastructure -- its rate, its concurrency and
// its retry policy are the things an operator sets and a deployment declares -- so it belongs in
// whatever describes the rest of the project's resources, not in the code that fills it. What this
// does is put work into a queue that already exists.
//
// A task's identity is its full resource name, projects/P/locations/L/queues/Q/tasks/T. Create
// takes the parent and the id separately because that is how the API takes them; everything else
// takes the name Create returned.
package cloud_tasks

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_tasks/cloud_tasks_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_tasks/types/list_tasks_response"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_tasks/types/task"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftHttpErrors "github.com/altshiftab/utils_go/pkg/http/errors"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

const Domain = "cloudtasks.googleapis.com"

// Scope is the OAuth scope a token must carry to manage tasks. The broader cloud-platform scope
// also works and is what a Cloud Run runtime identity usually has.
const Scope = "https://www.googleapis.com/auth/cloud-tasks"

// maxPageSize is the largest page tasks.list serves. Asking for more is not an error there; it
// simply returns this many, so paging has to assume it.
const maxPageSize = 1000

// ErrTaskExists is a task whose name has been used recently.
//
// It is worth telling apart from any other failure, because it is the answer a caller naming its
// tasks is asking for: the work is already queued, or has just been done, and asking again is not
// something to report as broken. Cloud Tasks keeps a name reserved for about an hour after the task
// it belonged to ran or was deleted, so this is also what a caller sees when it asks for the same
// work twice in quick succession.
var ErrTaskExists = errors.New("a task with that name already exists")

var defaultBaseUrl = &url.URL{
	Scheme: "https",
	Host:   Domain,
}

type Client struct {
	baseUrl *url.URL
	config  *cloud_tasks_config.Config
}

func NewClient(options ...cloud_tasks_config.Option) *Client {
	config := cloud_tasks_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = defaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/v2/"

	return &Client{baseUrl: &u, config: config}
}

// Parent is the queue a task belongs to, projects/P/locations/L/queues/Q. Cloud Tasks is regional,
// so the location is part of a queue's identity rather than a detail of where it happens to run.
func Parent(project string, location string, queue string) (string, error) {
	if project == "" {
		return "", altshiftErrors.NewWithTrace(empty_error.New("project"))
	}

	if location == "" {
		return "", altshiftErrors.NewWithTrace(empty_error.New("location"))
	}

	if queue == "" {
		return "", altshiftErrors.NewWithTrace(empty_error.New("queue"))
	}

	return "projects/" + project + "/locations/" + location + "/queues/" + queue, nil
}

// resourceUrl builds a URL for a resource path, with each segment escaped and any custom verb
// appended after it.
//
// The path is set twice, once escaped and once not, because that is what net/url wants of a path
// whose segments needed escaping: Path is the decoded form and RawPath the encoded one.
func (client *Client) resourceUrl(resource string, verb string) (string, error) {
	if resource == "" {
		return "", altshiftErrors.NewWithTrace(empty_error.New("resource"))
	}

	escaped := ""
	for index, segment := range splitPath(resource) {
		if index != 0 {
			escaped += "/"
		}
		escaped += url.PathEscape(segment)
	}

	u := *client.baseUrl
	u.Path += resource + verb
	u.RawPath = u.EscapedPath()[:len(client.baseUrl.EscapedPath())] + escaped + verb

	return u.String(), nil
}

func splitPath(path string) []string {
	segments := make([]string, 0, 8)
	current := ""
	for _, character := range path {
		if character == '/' {
			segments = append(segments, current)
			current = ""

			continue
		}
		current += string(character)
	}

	return append(segments, current)
}

// createTaskRequest is the body tasks.create takes. Unlike Cloud Scheduler, which is posted the
// resource itself, Cloud Tasks wraps it.
type createTaskRequest struct {
	Task *task.Task `json:"task,omitzero"`
	// ResponseView is how much of the created task comes back.
	ResponseView task.View `json:"responseView,omitzero"`
}

// CreateTask puts a task into the queue and returns it as the server recorded it.
//
// taskId is optional. Given one, the queue will refuse a second task of the same name, which is how
// duplicate work is kept out; see task.Task.Name for what that costs. Left empty, the server names
// the task and every call queues new work.
//
// A refused duplicate comes back as ErrTaskExists rather than as a failure, because to a caller
// naming its tasks that is an answer rather than a fault.
func (client *Client) CreateTask(
	ctx context.Context,
	parent string,
	taskId string,
	newTask *task.Task,
	options ...fetch_config.Option,
) (*task.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	if parent == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("parent"))
	}

	if newTask == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("task"))
	}

	urlString, err := client.resourceUrl(parent+"/tasks", "")
	if err != nil {
		return nil, fmt.Errorf("resource url: %w", err)
	}

	// The id is sent as the task's name relative to the parent, which is how the API takes it.
	created := *newTask
	if taskId != "" {
		created.Name = parent + "/tasks/" + taskId
	}

	options = append(append(client.config.FetchOptions, options...), fetch_config.WithMethod(http.MethodPost))

	_, response, err := altshiftHttpUtils.FetchJsonWithBody[*task.Task](
		ctx,
		urlString,
		&createTaskRequest{Task: &created},
		options...,
	)
	if err != nil {
		// The API answers a name it has seen recently with 409.
		var non2xx *altshiftHttpErrors.Non2xxStatusCodeError
		if errors.As(err, &non2xx) && non2xx.StatusCode == http.StatusConflict {
			return nil, altshiftErrors.New(fmt.Errorf("%w: %w", ErrTaskExists, err), urlString, created.Name)
		}

		return nil, altshiftErrors.New(fmt.Errorf("fetch json with body: %w", err), urlString)
	}
	if response == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("task"), urlString)
	}

	return response, nil
}

// GetTask reads one task by its full resource name.
func (client *Client) GetTask(
	ctx context.Context,
	name string,
	options ...fetch_config.Option,
) (*task.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	urlString, err := client.resourceUrl(name, "")
	if err != nil {
		return nil, fmt.Errorf("resource url: %w", err)
	}

	options = append(client.config.FetchOptions, options...)

	_, response, err := altshiftHttpUtils.FetchJson[*task.Task](ctx, urlString, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
	}
	if response == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("task"), urlString)
	}

	return response, nil
}

// DeleteTask removes a task from the queue, whether or not it has run.
//
// It does not free the name: a deleted task's name stays reserved for about an hour, which is what
// keeps a delete-and-recreate from being a way around the duplicate check.
func (client *Client) DeleteTask(
	ctx context.Context,
	name string,
	options ...fetch_config.Option,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context err: %w", err)
	}

	urlString, err := client.resourceUrl(name, "")
	if err != nil {
		return fmt.Errorf("resource url: %w", err)
	}

	options = append(append(client.config.FetchOptions, options...), fetch_config.WithMethod(http.MethodDelete))

	if _, _, err := altshiftHttpUtils.FetchJson[struct{}](ctx, urlString, options...); err != nil {
		return altshiftErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
	}

	return nil
}

// ListTasks reads every task in a queue, following the pages until there are none left.
//
// It is for looking at a queue rather than for working through one: a task is dispatched by the
// queue, not by a reader, and a queue long enough for the paging to matter is one nothing should be
// listing. The basic view comes back, so the bodies are not included.
func (client *Client) ListTasks(
	ctx context.Context,
	parent string,
	options ...fetch_config.Option,
) ([]*task.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	if parent == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("parent"))
	}

	base, err := client.resourceUrl(parent+"/tasks", "")
	if err != nil {
		return nil, fmt.Errorf("resource url: %w", err)
	}

	fetchOptions := append(client.config.FetchOptions, options...)

	var tasks []*task.Task
	pageToken := ""

	for {
		requestUrl, err := url.Parse(base)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("url parse: %w", err), base)
		}

		query := requestUrl.Query()
		query.Set("pageSize", strconv.Itoa(maxPageSize))
		if pageToken != "" {
			query.Set("pageToken", pageToken)
		}
		requestUrl.RawQuery = query.Encode()

		urlString := requestUrl.String()

		_, response, err := altshiftHttpUtils.FetchJson[*list_tasks_response.Response](ctx, urlString, fetchOptions...)
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
		}
		if response == nil {
			return nil, altshiftErrors.NewWithTrace(nil_error.New("list tasks response"), urlString)
		}

		tasks = append(tasks, response.Tasks...)

		// A page that comes back with no token is the last one. A token that does not move the
		// listing on would page forever, so it ends the walk too.
		if response.NextPageToken == "" || response.NextPageToken == pageToken {
			break
		}

		pageToken = response.NextPageToken
	}

	return tasks, nil
}
