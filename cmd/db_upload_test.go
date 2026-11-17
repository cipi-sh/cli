package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cipi-sh/cli/internal/api"
)

// fakeUploadServer plays the dump upload endpoints of cipi/api: start, one
// chunk per request in order, cancel. failChunk can make a chunk request fail
// after or before the chunk is stored.
type fakeUploadServer struct {
	t         *testing.T
	mu        sync.Mutex
	chunkSize int64
	filename  string
	size      int64
	received  bytes.Buffer
	next      int
	requests  int // chunk requests seen
	cancelled bool
	// failChunk decides, for the n-th chunk request (from 1), whether to
	// answer with this status instead of 200 — and whether the chunk is
	// stored first (a lost answer) or not.
	failChunk func(request int) (status int, store bool)
}

func (s *fakeUploadServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/dbs/uploads", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Filename string `json:"filename"`
			Size     int64  `json:"size"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			s.t.Errorf("start: %v", err)
		}
		s.mu.Lock()
		s.filename, s.size = body.Filename, body.Size
		s.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"data":{"upload_id":"11111111-2222-3333-4444-555555555555","size":%d,"chunk_size":%d}}`, body.Size, s.chunkSize)
	})
	mux.HandleFunc("POST /api/dbs/uploads/{id}/chunks", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.requests++

		if err := r.ParseMultipartForm(1 << 20); err != nil {
			s.t.Errorf("chunk: %v", err)
			return
		}
		index, _ := strconv.Atoi(r.FormValue("index"))
		file, _, err := r.FormFile("chunk")
		if err != nil {
			s.t.Errorf("chunk part: %v", err)
			return
		}
		data, _ := io.ReadAll(file)

		status, store := 0, true
		if s.failChunk != nil {
			status, store = s.failChunk(s.requests)
		}
		if status != 0 && !store {
			w.WriteHeader(status)
			io.WriteString(w, `{"error":"refused"}`)
			return
		}
		if index != s.next {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprintf(w, `{"error":"Chunk %d is out of order: the next one is %d"}`, index, s.next)
			return
		}
		s.received.Write(data)
		s.next++
		if status != 0 {
			w.WriteHeader(status)
			io.WriteString(w, `{"message":"Server Error"}`)
			return
		}
		fmt.Fprintf(w, `{"data":{"next":%d}}`, s.next)
	})
	mux.HandleFunc("DELETE /api/dbs/uploads/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.cancelled = true
		s.mu.Unlock()
		io.WriteString(w, `{"data":{"cancelled":true}}`)
	})
	return mux
}

func writeDump(t *testing.T, name string, size int) (string, []byte) {
	t.Helper()
	content := make([]byte, size)
	for i := range content {
		content[i] = byte('a' + i%26)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, content
}

func uploadTestClient(t *testing.T, s *fakeUploadServer) *api.Client {
	t.Helper()
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)
	return &api.Client{BaseURL: srv.URL, Token: "t", HTTPClient: &http.Client{Timeout: 30 * time.Second}}
}

func fastUploadRetries(t *testing.T) {
	t.Helper()
	old := dbUploadRetryDelay
	dbUploadRetryDelay = time.Millisecond
	t.Cleanup(func() { dbUploadRetryDelay = old })
}

func TestUploadDumpSendsEveryChunkInOrder(t *testing.T) {
	// 25 bytes in chunks of 10: two whole chunks and a last one of 5.
	path, content := writeDump(t, "shop.sql.gz", 25)
	s := &fakeUploadServer{t: t, chunkSize: 10}
	client := uploadTestClient(t, s)

	id, err := uploadDump(client, path, int64(len(content)))
	if err != nil {
		t.Fatalf("uploadDump: %v", err)
	}
	if id != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("upload id = %q", id)
	}
	if s.filename != "shop.sql.gz" || s.size != 25 {
		t.Fatalf("server was told filename=%q size=%d", s.filename, s.size)
	}
	if s.requests != 3 || !bytes.Equal(s.received.Bytes(), content) {
		t.Fatalf("server got %d chunk request(s), %d bytes; want 3 and the dump itself", s.requests, s.received.Len())
	}
	if s.cancelled {
		t.Fatal("a successful upload was cancelled")
	}
	if client.HTTPClient.Timeout != 30*time.Second {
		t.Fatalf("client timeout left at %s", client.HTTPClient.Timeout)
	}
}

func TestUploadDumpResendsAfterAServerError(t *testing.T) {
	fastUploadRetries(t)
	path, content := writeDump(t, "shop.sql", 25)
	// The 2nd request (chunk 1) fails before anything is stored.
	s := &fakeUploadServer{t: t, chunkSize: 10, failChunk: func(n int) (int, bool) {
		if n == 2 {
			return http.StatusBadGateway, false
		}
		return 0, true
	}}
	client := uploadTestClient(t, s)

	if _, err := uploadDump(client, path, int64(len(content))); err != nil {
		t.Fatalf("uploadDump: %v", err)
	}
	if s.requests != 4 || !bytes.Equal(s.received.Bytes(), content) {
		t.Fatalf("server got %d chunk request(s), %d bytes; want 4 and the dump itself", s.requests, s.received.Len())
	}
}

func TestUploadDumpGoesOnWhenTheAnswerWasLost(t *testing.T) {
	fastUploadRetries(t)
	path, content := writeDump(t, "shop.sql", 25)
	// The 2nd request (chunk 1) is stored, but its answer is a 500: the
	// retry gets "the next one is 2" and the upload must go on, not send
	// chunk 1 twice.
	s := &fakeUploadServer{t: t, chunkSize: 10, failChunk: func(n int) (int, bool) {
		if n == 2 {
			return http.StatusInternalServerError, true
		}
		return 0, true
	}}
	client := uploadTestClient(t, s)

	if _, err := uploadDump(client, path, int64(len(content))); err != nil {
		t.Fatalf("uploadDump: %v", err)
	}
	if !bytes.Equal(s.received.Bytes(), content) {
		t.Fatalf("server holds %q, want the dump itself", s.received.String())
	}
}

func TestUploadDumpCancelsWhenTheServerRefuses(t *testing.T) {
	fastUploadRetries(t)
	path, content := writeDump(t, "shop.sql", 25)
	s := &fakeUploadServer{t: t, chunkSize: 10, failChunk: func(n int) (int, bool) {
		if n == 2 {
			return http.StatusUnprocessableEntity, false
		}
		return 0, true
	}}
	client := uploadTestClient(t, s)

	_, err := uploadDump(client, path, int64(len(content)))
	if err == nil || !strings.Contains(err.Error(), "chunk 1") || httpStatusOf(err) != http.StatusUnprocessableEntity {
		t.Fatalf("err = %v, want the 422 of chunk 1", err)
	}
	if s.requests != 2 {
		t.Fatalf("a refused chunk was sent %d time(s) in all, want no retry", s.requests-1)
	}
	if !s.cancelled {
		t.Fatal("the failed upload was left on the server")
	}
}

func TestUploadDumpGivesUpAfterThreeAttempts(t *testing.T) {
	fastUploadRetries(t)
	path, content := writeDump(t, "shop.sql", 5)
	s := &fakeUploadServer{t: t, chunkSize: 10, failChunk: func(int) (int, bool) {
		return http.StatusBadGateway, false
	}}
	client := uploadTestClient(t, s)

	if _, err := uploadDump(client, path, int64(len(content))); err == nil {
		t.Fatal("expected an error")
	}
	if s.requests != dbUploadAttempts || !s.cancelled {
		t.Fatalf("requests = %d (want %d), cancelled = %v (want true)", s.requests, dbUploadAttempts, s.cancelled)
	}
}

func TestCheckDumpFile(t *testing.T) {
	good, _ := writeDump(t, "Shop.SQL.GZ", 12)
	if size, err := checkDumpFile(good); err != nil || size != 12 {
		t.Fatalf("checkDumpFile(%q) = %d, %v", good, size, err)
	}

	empty := filepath.Join(t.TempDir(), "empty.sql")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	zip, _ := writeDump(t, "shop.zip", 12)

	cases := map[string]string{
		zip:                   "only .sql and .sql.gz",
		empty:                 "is empty",
		t.TempDir() + "/.sql": "no such file",
	}
	for path, want := range cases {
		if _, err := checkDumpFile(path); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("checkDumpFile(%q) = %v, want an error with %q", path, err, want)
		}
	}
}

func TestHTTPStatusOf(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{&api.APIError{ErrorText: "nope", Status: 409}, 409},
		{fmt.Errorf("chunk 2: %w", &api.APIError{ErrorText: "nope", Status: 507}), 507},
		{errors.New("API error (HTTP 413): <html>Request Entity Too Large</html>"), 413},
		{fmt.Errorf("chunk 2: %w", errors.New("API error (HTTP 502): Bad Gateway")), 502},
		{errors.New("request failed: connection reset by peer"), 0},
	}
	for _, tc := range cases {
		if got := httpStatusOf(tc.err); got != tc.want {
			t.Errorf("httpStatusOf(%q) = %d, want %d", tc.err, got, tc.want)
		}
	}
}
