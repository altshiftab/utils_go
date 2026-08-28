// Package cloud_scheduler manages Cloud Scheduler jobs over the REST API.
//
// Only the HTTP target is modelled. The API also schedules App Engine requests and Pub/Sub
// messages; nothing here uses either, and a type for something no caller sends is a type nobody
// checks.
//
// A job's identity is its full resource name, projects/P/locations/L/jobs/J. Create takes the
// parent and the id separately because that is how the API takes them; everything else takes the
// name Create returned.
package cloud_scheduler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_scheduler/cloud_scheduler_config"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_scheduler/types/job"
	"github.com/altshiftab/utils_go/pkg/cloud/gcp/cloud_scheduler/types/list_jobs_response"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
)

const Domain = "cloudscheduler.googleapis.com"

// Scope is the OAuth scope a token must carry to manage jobs. The broader cloud-platform scope
// also works and is what a Cloud Run runtime identity usually has.
const Scope = "https://www.googleapis.com/auth/cloud-scheduler"

// maxPageSize is the largest page jobs.list serves. Asking for more is not an error there; it
// simply returns this many, so paging has to assume it.
const maxPageSize = 500

var defaultBaseUrl = &url.URL{
	Scheme: "https",
	Host:   Domain,
}

type Client struct {
	baseUrl *url.URL
	config  *cloud_scheduler_config.Config
}

func NewClient(options ...cloud_scheduler_config.Option) *Client {
	config := cloud_scheduler_config.New(options...)
	baseUrl := config.BaseUrl
	if baseUrl == nil {
		baseUrl = defaultBaseUrl
	}
	u := *baseUrl
	u.Path = "/v1/"

	return &Client{baseUrl: &u, config: config}
}

// Parent is the location a job belongs to, projects/P/locations/L. Cloud Scheduler is regional, so
// the location is part of a job's identity rather than a detail of where it happens to run.
func Parent(project string, location string) (string, error) {
	if project == "" {
		return "", altshiftErrors.NewWithTrace(empty_error.New("project"))
	}

	if location == "" {
		return "", altshiftErrors.NewWithTrace(empty_error.New("location"))
	}

	return "projects/" + project + "/locations/" + location, nil
}

// resourceUrl builds a URL for a resource path, with each segment escaped and any custom verb --
// the ":run" of a jobs.run call -- appended after it.
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
	segments := make([]string, 0, 6)
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

// CreateJob creates a job under parent with the id given, and returns it as the server recorded it
// -- with its full name, which is what every other call addresses it by.
func (client *Client) CreateJob(
	ctx context.Context,
	parent string,
	jobId string,
	newJob *job.Job,
	options ...fetch_config.Option,
) (*job.Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	if parent == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("parent"))
	}

	if jobId == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("job id"))
	}

	if newJob == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("job"))
	}

	urlString, err := client.resourceUrl(parent+"/jobs", "")
	if err != nil {
		return nil, fmt.Errorf("resource url: %w", err)
	}

	// The id is sent as the job's name relative to the parent, which is how the API takes it.
	created := *newJob
	created.Name = parent + "/jobs/" + jobId

	options = append(append(client.config.FetchOptions, options...), fetch_config.WithMethod(http.MethodPost))
	_, response, err := altshiftHttpUtils.FetchJsonWithBody[*job.Job](ctx, urlString, &created, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json with body: %w", err), urlString)
	}
	if response == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("job"), urlString)
	}

	return response, nil
}

// GetJob reads one job by its full resource name.
func (client *Client) GetJob(
	ctx context.Context,
	name string,
	options ...fetch_config.Option,
) (*job.Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	urlString, err := client.resourceUrl(name, "")
	if err != nil {
		return nil, fmt.Errorf("resource url: %w", err)
	}

	options = append(client.config.FetchOptions, options...)
	_, response, err := altshiftHttpUtils.FetchJson[*job.Job](ctx, urlString, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
	}
	if response == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("job"), urlString)
	}

	return response, nil
}

// ListJobs reads every job under parent, following the pages until there are none left.
//
// The caller gets the whole list rather than a page and a token: a location holds the schedules of
// one deployment, which is tens of jobs rather than thousands, and every caller here wants all of
// them.
func (client *Client) ListJobs(
	ctx context.Context,
	parent string,
	options ...fetch_config.Option,
) ([]*job.Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	if parent == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("parent"))
	}

	base, err := client.resourceUrl(parent+"/jobs", "")
	if err != nil {
		return nil, fmt.Errorf("resource url: %w", err)
	}

	fetchOptions := append(client.config.FetchOptions, options...)

	var jobs []*job.Job
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
		_, response, err := altshiftHttpUtils.FetchJson[*list_jobs_response.Response](ctx, urlString, fetchOptions...)
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
		}
		if response == nil {
			return nil, altshiftErrors.NewWithTrace(nil_error.New("list jobs response"), urlString)
		}

		jobs = append(jobs, response.Jobs...)

		// A page that comes back with no token is the last one. A token that does not move the
		// listing on would otherwise be asked for forever.
		if response.NextPageToken == "" || response.NextPageToken == pageToken {
			break
		}
		pageToken = response.NextPageToken
	}

	return jobs, nil
}

// PatchJob updates the fields named in updateMask and leaves the rest alone.
//
// The mask is required rather than optional: patching without one replaces the whole job with what
// was sent, so a caller that meant to change a schedule would silently drop its target.
func (client *Client) PatchJob(
	ctx context.Context,
	name string,
	updated *job.Job,
	updateMask []string,
	options ...fetch_config.Option,
) (*job.Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	if updated == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("job"))
	}

	if len(updateMask) == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("update mask"))
	}

	base, err := client.resourceUrl(name, "")
	if err != nil {
		return nil, fmt.Errorf("resource url: %w", err)
	}

	requestUrl, err := url.Parse(base)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("url parse: %w", err), base)
	}

	mask := ""
	for index, field := range updateMask {
		if index != 0 {
			mask += ","
		}
		mask += field
	}

	query := requestUrl.Query()
	query.Set("updateMask", mask)
	requestUrl.RawQuery = query.Encode()

	urlString := requestUrl.String()
	options = append(append(client.config.FetchOptions, options...), fetch_config.WithMethod(http.MethodPatch))
	_, response, err := altshiftHttpUtils.FetchJsonWithBody[*job.Job](ctx, urlString, updated, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json with body: %w", err), urlString)
	}
	if response == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("job"), urlString)
	}

	return response, nil
}

// DeleteJob removes a job.
func (client *Client) DeleteJob(ctx context.Context, name string, options ...fetch_config.Option) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context err: %w", err)
	}

	urlString, err := client.resourceUrl(name, "")
	if err != nil {
		return fmt.Errorf("resource url: %w", err)
	}

	options = append(
		append(client.config.FetchOptions, options...),
		fetch_config.WithMethod(http.MethodDelete),
		fetch_config.WithSkipReadResponseBody(true),
	)
	if _, _, err := altshiftHttpUtils.Fetch(ctx, urlString, options...); err != nil {
		return altshiftErrors.New(fmt.Errorf("fetch: %w", err), urlString)
	}

	return nil
}

// PauseJob stops a job running until it is resumed.
func (client *Client) PauseJob(ctx context.Context, name string, options ...fetch_config.Option) (*job.Job, error) {
	return client.act(ctx, name, ":pause", options...)
}

// ResumeJob starts a paused job running again.
func (client *Client) ResumeJob(ctx context.Context, name string, options ...fetch_config.Option) (*job.Job, error) {
	return client.act(ctx, name, ":resume", options...)
}

// RunJob runs a job now, without regard to its schedule and without disturbing it. It is what an
// administrator asking for a run on demand calls.
func (client *Client) RunJob(ctx context.Context, name string, options ...fetch_config.Option) (*job.Job, error) {
	return client.act(ctx, name, ":run", options...)
}

// act performs one of the custom verbs, each of which is a POST with an empty body that answers
// with the job.
func (client *Client) act(
	ctx context.Context,
	name string,
	verb string,
	options ...fetch_config.Option,
) (*job.Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context err: %w", err)
	}

	urlString, err := client.resourceUrl(name, verb)
	if err != nil {
		return nil, fmt.Errorf("resource url: %w", err)
	}

	options = append(
		append(client.config.FetchOptions, options...),
		fetch_config.WithMethod(http.MethodPost),
		fetch_config.WithBody([]byte("{}")),
	)
	_, response, err := altshiftHttpUtils.FetchJson[*job.Job](ctx, urlString, options...)
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("fetch json: %w", err), urlString)
	}
	if response == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("job"), urlString)
	}

	return response, nil
}
