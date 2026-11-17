package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
