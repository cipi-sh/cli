package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPIErrorUnmarshal(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string // Error() with Status 422
	}{
		{
			name: "Laravel validation: field → list of messages",
			body: `{"message":"The major field is required.","errors":{"major":["The major field is required."]}}`,
			want: "The major field is required. (major: The major field is required.)",
		},
		{
			name: "backup profile: field → message, already joined in error",
			body: `{"error":"scope must be one of: all, files, db","errors":{"scope":"scope must be one of: all, files, db"}}`,
			want: "HTTP 422: scope must be one of: all, files, db",
		},
		{
			name: "plain error",
			body: `{"error":"App 'nope' not found"}`,
			want: "HTTP 422: App 'nope' not found",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var apiErr APIError
			if err := json.Unmarshal([]byte(tc.body), &apiErr); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			apiErr.Status = 422
			if got := apiErr.Error(); got != tc.want {
				t.Fatalf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestJobFailureMessage(t *testing.T) {
	cases := []struct {
		name string
		job  JobStatus
		want string
	}{
		{"its own error", JobStatus{Error: "boom", Result: map[string]interface{}{"error": "other"}}, "boom"},
		{"the error parsed out of the CLI output", JobStatus{Result: map[string]interface{}{"error": "Node 22 is used by app shop"}}, "Node 22 is used by app shop"},
		{"a result without an error", JobStatus{Result: map[string]interface{}{"outcome": "blocked"}}, "job failed"},
		{"no result", JobStatus{}, "job failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.job.FailureMessage(); got != tc.want {
				t.Fatalf("FailureMessage() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPostMultipart(t *testing.T) {
	var gotIndex, gotName, gotAuth string
	var gotChunk []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("not a multipart request: %v", err)
			return
		}
		gotIndex = r.FormValue("index")
		file, header, err := r.FormFile("chunk")
		if err != nil {
			t.Errorf("no chunk part: %v", err)
			return
		}
		defer file.Close()
		gotName = header.Filename
		gotChunk, _ = io.ReadAll(file)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":{"next":4}}`)
	}))
	defer srv.Close()

	client := &Client{BaseURL: srv.URL, Token: "1|secret", HTTPClient: srv.Client()}
	var result struct {
		Data struct {
			Next int `json:"next"`
		} `json:"data"`
	}
	err := client.PostMultipart(context.Background(), "/api/dbs/uploads/x/chunks", map[string]string{"index": "3"}, "chunk", "chunk", []byte("dump bytes"), &result)
	if err != nil {
		t.Fatalf("PostMultipart: %v", err)
	}
	if gotAuth != "Bearer 1|secret" || gotIndex != "3" || gotName != "chunk" || string(gotChunk) != "dump bytes" {
		t.Fatalf("server got auth=%q index=%q name=%q chunk=%q", gotAuth, gotIndex, gotName, gotChunk)
	}
	if result.Data.Next != 4 {
		t.Fatalf("next = %d, want 4", result.Data.Next)
	}
}

func TestPostMultipartError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		io.WriteString(w, `{"error":"Chunk 3 is out of order: the next one is 4"}`)
	}))
	defer srv.Close()

	client := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	err := client.PostMultipart(context.Background(), "/x", nil, "chunk", "chunk", []byte("x"), nil)
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != http.StatusConflict || !strings.Contains(apiErr.Error(), "the next one is 4") {
		t.Fatalf("err = %v, want the API's 409", err)
	}
}

// fakeJobClock replaces the clock of WaitForJobFor: a sleep moves the time
// instead of waiting. It returns what was slept, and puts the clock back.
func fakeJobClock(t *testing.T) *[]time.Duration {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	slept := []time.Duration{}
	jobNow = func() time.Time { return now }
	jobSleep = func(d time.Duration) { slept = append(slept, d); now = now.Add(d) }
	t.Cleanup(func() { jobNow, jobSleep = time.Now, time.Sleep })
	return &slept
}

func total(ds []time.Duration) time.Duration {
	var sum time.Duration
	for _, d := range ds {
		sum += d
	}
	return sum
}

// jobServer answers GET /api/jobs/{id} with what answer(poll) returns (poll counts from 1).
func jobServer(t *testing.T, answer func(poll int) (int, string)) (*Client, *int) {
	t.Helper()
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/api/jobs/") {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		polls++
		status, body := answer(polls)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, Token: "1|secret", HTTPClient: srv.Client()}, &polls
}

func TestJobWaits(t *testing.T) {
	if JobWait <= 10*time.Minute || JobWait > 15*time.Minute {
		t.Fatalf("JobWait = %s: a job has ten minutes on the server, the wait a little more", JobWait)
	}
	if DeployJobWait <= 60*time.Minute || DeployJobWait > 70*time.Minute {
		t.Fatalf("DeployJobWait = %s: a deploy has an hour on the server, the wait a little more", DeployJobWait)
	}
	for n, want := range map[int]time.Duration{0: 2 * time.Second, 10: 2 * time.Second, 11: 3 * time.Second, 30: 3 * time.Second, 31: 5 * time.Second, 900: 5 * time.Second} {
		if got := jobPollInterval(n); got != want {
			t.Errorf("jobPollInterval(%d) = %s, want %s", n, got, want)
		}
	}
}

// A deploy whose frontend build takes half an hour: followed to its end. The
// wait used to stop after 120 polls, under nine minutes.
func TestWaitForJobFollowsALongDeployToItsEnd(t *testing.T) {
	slept := fakeJobClock(t)
	client, polls := jobServer(t, func(poll int) (int, string) {
		if poll < 400 {
			return 200, `{"data":{"id":"j1","status":"running","output":"task cipi:node_build"}}`
		}
		return 200, `{"data":{"id":"j1","status":"completed","output":"Successfully deployed!"}}`
	})

	job, err := client.WaitForJobFor("j1", DeployJobWait)
	if err != nil {
		t.Fatalf("WaitForJobFor: %v", err)
	}
	if job.Status != "completed" || *polls != 400 {
		t.Fatalf("status=%q after %d polls, want completed after 400", job.Status, *polls)
	}
	if waited := total(*slept); waited < 30*time.Minute || waited > 35*time.Minute {
		t.Fatalf("followed for %s, want about 32 minutes (11 polls 2 s apart, 20 at 3 s, the rest at 5 s)", waited)
	}
}

func TestWaitForJobGivesUpWhenItsTimeIsOver(t *testing.T) {
	slept := fakeJobClock(t)
	client, polls := jobServer(t, func(int) (int, string) { return 200, `{"data":{"id":"j2","status":"running"}}` })

	job, err := client.WaitForJob("j2")
	if job != nil || err == nil {
		t.Fatalf("job=%v err=%v, want no job and an error", job, err)
	}
	for _, want := range []string{"job j2 is still running after 11 minutes", "the job does not", "cipi-cli jobs wait j2"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not say %q", err, want)
		}
	}
	if waited := total(*slept); waited < JobWait || waited >= JobWait+5*time.Second {
		t.Fatalf("waited %s, want %s and not a poll more", waited, JobWait)
	}
	if *polls < 121 {
		t.Fatalf("%d polls: no more than the 120 it used to stop at", *polls)
	}
}

func TestWaitForJobReturnsAFailedJobAtOnce(t *testing.T) {
	slept := fakeJobClock(t)
	client, polls := jobServer(t, func(poll int) (int, string) {
		if poll == 1 {
			return 200, `{"data":{"id":"j3","status":"running"}}`
		}
		return 200, `{"data":{"id":"j3","status":"failed","result":{"error":"The build (cipi:node_build) ran for 30 minutes"}}}`
	})

	job, err := client.WaitForJobFor("j3", DeployJobWait)
	if err == nil || err.Error() != "The build (cipi:node_build) ran for 30 minutes" || job == nil || job.Status != "failed" {
		t.Fatalf("job=%v err=%v, want the failed job and its error", job, err)
	}
	if *polls != 2 || total(*slept) != 2*time.Second {
		t.Fatalf("%d polls, %s slept: want 2 polls 2 s apart", *polls, total(*slept))
	}
}

// The API not answering once (a restart, a gateway error) is not the end of the job.
func TestWaitForJobAsksAgainWhenTheAPIDoesNotAnswer(t *testing.T) {
	slept := fakeJobClock(t)
	client, polls := jobServer(t, func(poll int) (int, string) {
		switch poll {
		case 1, 2:
			return 502, `<html>Bad Gateway</html>`
		case 3:
			return 200, `{"data":{"id":"j4","status":"running"}}`
		}
		return 200, `{"data":{"id":"j4","status":"completed"}}`
	})

	job, err := client.WaitForJob("j4")
	if err != nil || job == nil || job.Status != "completed" {
		t.Fatalf("job=%v err=%v, want the completed job", job, err)
	}
	if *polls != 4 || total(*slept) != 6*time.Second {
		t.Fatalf("%d polls, %s slept: want 4 polls 2 s apart", *polls, total(*slept))
	}
}
