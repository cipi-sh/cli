package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/cipi-sh/cli/internal/config"
	"github.com/cipi-sh/cli/internal/output"
)

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type APIError struct {
	Message   string              `json:"message"`
	ErrorText string              `json:"error,omitempty"`
	Errors    map[string][]string `json:"errors,omitempty"`
	// Reasons is what a refused change says line by line (nginx directives,
	// cipi/api 1.44): the first is ErrorText again.
	Reasons []string `json:"reasons,omitempty"`
	Status  int      `json:"-"`
}

// UnmarshalJSON accepts the two shapes of "errors" the API sends: Laravel
// validation's field → list of messages, which is kept, and the backup
// endpoints' field → message, which is dropped because "error" already
// carries those messages joined.
func (e *APIError) UnmarshalJSON(data []byte) error {
	var raw struct {
		Message   string          `json:"message"`
		ErrorText string          `json:"error"`
		Errors    json.RawMessage `json:"errors"`
		Reasons   []string        `json:"reasons"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Message, e.ErrorText, e.Errors, e.Reasons = raw.Message, raw.ErrorText, nil, raw.Reasons
	if len(raw.Errors) > 0 {
		var fields map[string][]string
		if json.Unmarshal(raw.Errors, &fields) == nil {
			e.Errors = fields
		}
	}
	return nil
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = e.ErrorText
	}
	if len(e.Errors) > 0 {
		var parts []string
		for field, msgs := range e.Errors {
			for _, msg := range msgs {
				parts = append(parts, fmt.Sprintf("%s: %s", field, msg))
			}
		}
		return fmt.Sprintf("%s (%s)", msg, strings.Join(parts, "; "))
	}
	if e.Status > 0 && msg != "" {
		return fmt.Sprintf("HTTP %d: %s", e.Status, msg)
	}
	return msg
}

func (e *APIError) IsRouteNotFound() bool {
	return strings.Contains(strings.ToLower(e.Message), "could not be found")
}

func RouteNotFoundHint(err error, minVersion, feature string) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.IsRouteNotFound() {
		return fmt.Sprintf(
			"%s — update cipi/api to %s+ on your server (%s)",
			apiErr.Message,
			minVersion,
			feature,
		)
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

type AsyncResponse struct {
	JobID interface{} `json:"job_id"`
}

func (a *AsyncResponse) JobIDString() string {
	switch v := a.JobID.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

type JobStatus struct {
	ID     interface{} `json:"id"`
	Status string      `json:"status"`
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
	// Output is what the Cipi CLI printed on the server (finished jobs only).
	Output string `json:"output,omitempty"`
}

// FailureMessage says why a job failed: its error, else the error the API
// parsed out of the CLI output (result.error), else a generic message.
func (j *JobStatus) FailureMessage() string {
	if j.Error != "" {
		return j.Error
	}
	if m, ok := j.Result.(map[string]interface{}); ok {
		if msg, _ := m["error"].(string); msg != "" {
			return msg
		}
	}
	return "job failed"
}

func NewClient() (*Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return NewClientFromProfile(cfg), nil
}

// NewClientForProfile builds a client for a named server profile.
func NewClientForProfile(name string) (*Client, error) {
	cfg, err := config.LoadNamed(name)
	if err != nil {
		return nil, err
	}
	return NewClientFromProfile(cfg), nil
}

func NewClientFromProfile(cfg *config.Profile) *Client {
	return &Client{
		BaseURL: strings.TrimRight(cfg.Endpoint, "/"),
		Token:   cfg.Token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) request(method, path string, body interface{}) (*http.Response, error) {
	url := c.BaseURL + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

func (c *Client) parseResponse(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr APIError
		if json.Unmarshal(data, &apiErr) == nil && (apiErr.Message != "" || apiErr.ErrorText != "") {
			apiErr.Status = resp.StatusCode
			return &apiErr
		}
		return fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(data))
	}

	if target != nil && len(data) > 0 {
		if err := json.Unmarshal(data, target); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
	}

	return nil
}

func (c *Client) Get(path string, result interface{}) error {
	resp, err := c.request("GET", path, nil)
	if err != nil {
		return err
	}
	return c.parseResponse(resp, result)
}

// GetJob fetches GET /api/jobs/{id}. The API returns either { "data": { job } } or a flat job object.
func (c *Client) GetJob(jobID string) (*JobStatus, error) {
	resp, err := c.request("GET", fmt.Sprintf("/api/jobs/%s", jobID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		var apiErr APIError
		if json.Unmarshal(data, &apiErr) == nil && (apiErr.Message != "" || apiErr.ErrorText != "") {
			apiErr.Status = resp.StatusCode
			return nil, &apiErr
		}
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(data))
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(data, &outer); err != nil {
		return nil, fmt.Errorf("parsing job response: %w", err)
	}
	if raw, ok := outer["data"]; ok {
		var job JobStatus
		if err := json.Unmarshal(raw, &job); err != nil {
			return nil, fmt.Errorf("parsing job data: %w", err)
		}
		return &job, nil
	}
	var job JobStatus
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("parsing job response: %w", err)
	}
	return &job, nil
}

// ListDatabases returns GET /api/dbs. The server may answer synchronously (200 + data) or
// asynchronously (202 + job_id); in the latter case the job is polled until completion.
// engine may be "mariadb" or "pgsql" to filter (Cipi 4.8+).
func (c *Client) ListDatabases(engine string) ([]map[string]interface{}, error) {
	path := "/api/dbs"
	if engine != "" {
		path += "?engine=" + url.QueryEscape(engine)
	}
	resp, err := c.request("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		var apiErr APIError
		if json.Unmarshal(body, &apiErr) == nil && (apiErr.Message != "" || apiErr.ErrorText != "") {
			apiErr.Status = resp.StatusCode
			return nil, &apiErr
		}
		return nil, fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	switch resp.StatusCode {
	case http.StatusOK:
		var v struct {
			Data []map[string]interface{} `json:"data"`
		}
		if err := json.Unmarshal(body, &v); err != nil {
			return nil, fmt.Errorf("parsing database list: %w", err)
		}
		if v.Data == nil {
			return []map[string]interface{}{}, nil
		}
		return v.Data, nil
	case http.StatusAccepted:
		var async AsyncResponse
		if err := json.Unmarshal(body, &async); err != nil {
			return nil, fmt.Errorf("parsing async response: %w", err)
		}
		if async.JobID == nil || async.JobIDString() == "" {
			return nil, fmt.Errorf("no job_id in database list response")
		}
		job, err := c.WaitForJob(async.JobIDString())
		if err != nil {
			return nil, err
		}
		return databasesFromJobResult(job.Result)
	default:
		return nil, fmt.Errorf("unexpected HTTP %d listing databases: %s", resp.StatusCode, string(body))
	}
}

func databasesFromJobResult(result interface{}) ([]map[string]interface{}, error) {
	if result == nil {
		return []map[string]interface{}{}, nil
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected job result format for database list")
	}
	raw, ok := m["databases"]
	if !ok || raw == nil {
		return []map[string]interface{}{}, nil
	}
	arr, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("job result \"databases\" is not an array")
	}
	out := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

func (c *Client) Post(path string, body interface{}, result interface{}) error {
	resp, err := c.request("POST", path, body)
	if err != nil {
		return err
	}
	return c.parseResponse(resp, result)
}

func (c *Client) Put(path string, body interface{}, result interface{}) error {
	resp, err := c.request("PUT", path, body)
	if err != nil {
		return err
	}
	return c.parseResponse(resp, result)
}

func (c *Client) Delete(path string, body interface{}, result interface{}) error {
	resp, err := c.request("DELETE", path, body)
	if err != nil {
		return err
	}
	return c.parseResponse(resp, result)
}

// PostMultipart sends a multipart/form-data POST: the plain fields, then one
// file part. ctx aborts the request (an interrupted upload, for instance).
func (c *Client) PostMultipart(ctx context.Context, path string, fields map[string]string, fileField, fileName string, data []byte, result interface{}) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
	}
	part, err := w.CreateFormFile(fileField, fileName)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+path, &buf)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	return c.parseResponse(resp, result)
}

func (c *Client) DoAsync(method, path string, body interface{}) (*AsyncResponse, error) {
	resp, err := c.request(method, path, body)
	if err != nil {
		return nil, err
	}

	var async AsyncResponse
	if err := c.parseResponse(resp, &async); err != nil {
		return nil, err
	}

	if async.JobID == nil || async.JobIDString() == "" {
		return nil, nil
	}

	return &async, nil
}

// How long a job is followed before the wait gives up. The API stops a job
// after ten minutes, a deploy after two hours (cipi/api 1.48.0: the frontend
// build of a release alone has 30 minutes, and up to an hour where the app
// asks for it), and closes it as failed — so the wait is a minute longer than
// that, and what ends it is the end of the job. The loop used to stop after
// 120 polls, under nine minutes: a deploy with a long build "timed out after
// polling" while it was still going.
const (
	JobWait       = 11 * time.Minute
	DeployJobWait = 121 * time.Minute
)

// The clock of the wait: time.Sleep and time.Now, replaced in tests.
var (
	jobSleep = time.Sleep
	jobNow   = time.Now
)

// jobPollInterval is the pause after the poll numbered n (from 0): often at
// first, when most jobs end, then every few seconds.
func jobPollInterval(n int) time.Duration {
	switch {
	case n > 30:
		return 5 * time.Second
	case n > 10:
		return 3 * time.Second
	default:
		return 2 * time.Second
	}
}

func (c *Client) WaitForJob(jobID string) (*JobStatus, error) {
	return c.WaitForJobFor(jobID, JobWait)
}

// WaitForJobFor polls a job until it ends, or until wait has passed.
func (c *Client) WaitForJobFor(jobID string, wait time.Duration) (*JobStatus, error) {
	s := spinner.New(spinner.CharSets[14], 120*time.Millisecond)
	s.Suffix = "  Processing..."
	s.Color("cyan")
	s.Start()
	defer s.Stop()

	deadline := jobNow().Add(wait)
	for i := 0; ; i++ {
		job, err := c.GetJob(jobID)
		if err == nil {
			switch job.Status {
			case "completed", "success", "finished":
				s.Stop()
				return job, nil
			case "failed", "error":
				s.Stop()
				return job, fmt.Errorf("%s", job.FailureMessage())
			}
		}
		if !jobNow().Before(deadline) {
			break
		}
		if err != nil {
			// The API did not answer this once: asked again soon.
			jobSleep(2 * time.Second)
			continue
		}
		jobSleep(jobPollInterval(i))
	}

	return nil, fmt.Errorf("job %s is still running after %d minutes: this command stops waiting, the job does not. Follow it with: cipi-cli jobs wait %s", jobID, int(wait.Minutes()), jobID)
}

func (c *Client) DoAsyncAndWait(method, path string, body interface{}) error {
	_, err := c.DoAsyncAndWaitResult(method, path, body)
	return err
}

// DoAsyncAndWaitResult is DoAsyncAndWait but also returns the finished job, so callers can
// read its parsed result (nil when the API answered synchronously without a job_id).
func (c *Client) DoAsyncAndWaitResult(method, path string, body interface{}) (*JobStatus, error) {
	return c.DoAsyncAndWaitResultFor(method, path, body, JobWait)
}

// DoAsyncAndWaitFor is DoAsyncAndWait for a job that may take longer than most: a deploy.
func (c *Client) DoAsyncAndWaitFor(method, path string, body interface{}, wait time.Duration) error {
	_, err := c.DoAsyncAndWaitResultFor(method, path, body, wait)
	return err
}

// DoAsyncAndWaitResultFor is DoAsyncAndWaitResult with the time the job is followed for.
func (c *Client) DoAsyncAndWaitResultFor(method, path string, body interface{}, wait time.Duration) (*JobStatus, error) {
	async, err := c.DoAsync(method, path, body)
	if err != nil {
		return nil, err
	}

	if async == nil {
		return nil, nil
	}

	output.Info("Job dispatched: %s", async.JobIDString())

	return c.WaitForJobFor(async.JobIDString(), wait)
}
