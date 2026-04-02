package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	Message string              `json:"message"`
	Errors  map[string][]string `json:"errors,omitempty"`
}

func (e *APIError) Error() string {
	if len(e.Errors) > 0 {
		var parts []string
		for field, msgs := range e.Errors {
			for _, msg := range msgs {
				parts = append(parts, fmt.Sprintf("%s: %s", field, msg))
			}
		}
		return fmt.Sprintf("%s (%s)", e.Message, strings.Join(parts, "; "))
	}
	return e.Message
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
}

func NewClient() (*Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimRight(cfg.Endpoint, "/")

	return &Client{
		BaseURL: endpoint,
		Token:   cfg.Token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
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
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
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

func (c *Client) WaitForJob(jobID string) (*JobStatus, error) {
	s := spinner.New(spinner.CharSets[14], 120*time.Millisecond)
	s.Suffix = "  Processing..."
	s.Color("cyan")
	s.Start()
	defer s.Stop()

	maxAttempts := 120
	for i := 0; i < maxAttempts; i++ {
		var job JobStatus
		if err := c.Get(fmt.Sprintf("/api/jobs/%s", jobID), &job); err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		switch job.Status {
		case "completed", "success", "finished":
			s.Stop()
			return &job, nil
		case "failed", "error":
			s.Stop()
			errMsg := job.Error
			if errMsg == "" {
				errMsg = "job failed"
			}
			return &job, fmt.Errorf("%s", errMsg)
		}

		interval := 2 * time.Second
		if i > 10 {
			interval = 3 * time.Second
		}
		if i > 30 {
			interval = 5 * time.Second
		}
		time.Sleep(interval)
	}

	return nil, fmt.Errorf("job %s timed out after polling", jobID)
}

func (c *Client) DoAsyncAndWait(method, path string, body interface{}) error {
	async, err := c.DoAsync(method, path, body)
	if err != nil {
		return err
	}

	if async == nil {
		return nil
	}

	output.Info("Job dispatched: %s", async.JobIDString())

	_, err = c.WaitForJob(async.JobIDString())
	return err
}
