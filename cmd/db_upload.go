package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// A dump is uploaded in chunks (POST /api/dbs/uploads, then one POST per
// chunk, in order) so that no request has to fit the whole file into the
// limits of nginx and PHP. The server appends the chunks to a file it deletes
// when the restore job ends, or after a day.

// dbUploadChunkTimeout replaces the default 30 s client timeout while chunks
// are sent: one chunk is up to 32 MB and has to make it through a slow uplink.
const dbUploadChunkTimeout = 10 * time.Minute

// dbUploadAttempts is how many times one chunk is sent before giving up.
const dbUploadAttempts = 3

// dbUploadRetryDelay is the pause before a chunk is sent again; it grows
// with every attempt.
var dbUploadRetryDelay = 2 * time.Second

var errDumpUploadInterrupted = errors.New("interrupted — the upload was cancelled on the server")

// chunkOutOfOrder reads the index the server expects from its 409:
// "Chunk 3 is out of order: the next one is 4".
var chunkOutOfOrder = regexp.MustCompile(`the next one is (\d+)`)

// dbUploadStart is the answer of POST /api/dbs/uploads.
type dbUploadStart struct {
	UploadID  string `json:"upload_id"`
	ChunkSize int64  `json:"chunk_size"`
}

var dbUploadLimitsCmd = &cobra.Command{
	Use:   "upload-limits",
	Short: "What the server accepts as an uploaded dump",
	Long: `Show the limits of "db restore --upload": the largest dump the server
accepts, the size of the chunks it is sent in and the accepted extensions
(read from GET /api/dbs/uploads/limits; set on the server in config/cipi.php,
db_upload). A dump over the limit is copied to the server and restored with
"db restore --file".

Requires Cipi 5.9.0+, cipi/api 1.39+ and the dbs-manage ability.

  cipi-cli db upload-limits
  cipi-cli prod db upload-limits --json`,
	Example: `  cipi-cli db upload-limits
  cipi-cli prod db upload-limits --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Get("/api/dbs/uploads/limits", &result); err != nil {
			output.Error("Failed to read the upload limits: %s", featureErrorHint(err, dbLinkMinAPI, "dbs-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
			return nil
		}

		var data struct {
			MaxSize    int64    `json:"max_size"`
			ChunkSize  int64    `json:"chunk_size"`
			Extensions []string `json:"extensions"`
		}
		if err := json.Unmarshal(result.Data, &data); err != nil {
			output.Error("Failed to parse the upload limits: %s", err)
			return err
		}

		output.Header("Dump upload limits")
		output.KeyValue(nil, "Largest dump", humanBytes(data.MaxSize))
		output.KeyValue(nil, "Chunk size", humanBytes(data.ChunkSize))
		output.KeyValue(nil, "Extensions", joinOrDash(data.Extensions))
		fmt.Println()
		return nil
	},
}

// checkDumpFile refuses what the server would refuse before anything is
// sent: a name that is not .sql or .sql.gz, or an empty file. It returns the
// size of the dump.
func checkDumpFile(path string) (int64, error) {
	lower := strings.ToLower(path)
	if !strings.HasSuffix(lower, ".sql") && !strings.HasSuffix(lower, ".sql.gz") {
		return 0, fmt.Errorf("%s: only .sql and .sql.gz dumps can be uploaded", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if info.IsDir() {
		return 0, fmt.Errorf("%s is a directory", path)
	}
	if info.Size() == 0 {
		return 0, fmt.Errorf("%s is empty", path)
	}
	return info.Size(), nil
}

// uploadDump sends a local dump to the server in chunks and returns the
// upload_id to restore it with. When it fails or is interrupted (Ctrl-C) the
// upload is cancelled on the server, so nothing is left waiting there.
func uploadDump(client *api.Client, path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var started struct {
		Data dbUploadStart `json:"data"`
	}
	body := map[string]interface{}{"filename": filepath.Base(path), "size": size}
	if err := client.Post("/api/dbs/uploads", body, &started); err != nil {
		return "", err
	}
	up := started.Data
	if up.UploadID == "" || up.ChunkSize <= 0 {
		return "", fmt.Errorf("the server did not start the upload (no upload_id or chunk_size in its answer)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	timeout := client.HTTPClient.Timeout
	client.HTTPClient.Timeout = dbUploadChunkTimeout

	err = sendDumpChunks(ctx, client, up, f, size)

	// Back to the defaults before anything else: a second Ctrl-C must not
	// have to wait for the cancel request.
	client.HTTPClient.Timeout = timeout
	stop()
	if err != nil {
		cancelDumpUpload(client, up.UploadID)
		return "", err
	}
	return up.UploadID, nil
}

func sendDumpChunks(ctx context.Context, client *api.Client, up dbUploadStart, f io.Reader, size int64) error {
	s := spinner.New(spinner.CharSets[14], 120*time.Millisecond)
	s.Suffix = dumpUploadProgress(0, size)
	s.Color("cyan")
	s.Start()
	defer s.Stop()

	buf := make([]byte, min(up.ChunkSize, size))
	var sent int64
	for index := 0; sent < size; index++ {
		n, err := io.ReadFull(f, buf[:min(up.ChunkSize, size-sent)])
		if err != nil {
			return fmt.Errorf("reading the dump: %w (did the file change while it was uploaded?)", err)
		}
		if err := sendDumpChunk(ctx, client, up.UploadID, index, buf[:n]); err != nil {
			return err
		}
		sent += int64(n)

		s.Lock()
		s.Suffix = dumpUploadProgress(sent, size)
		s.Unlock()
	}
	return nil
}

func dumpUploadProgress(sent, size int64) string {
	return fmt.Sprintf("  Uploading %s / %s (%d%%)", humanBytes(sent), humanBytes(size), sent*100/size)
}

// sendDumpChunk sends one chunk, again when the connection or the server
// failed. A request the server refused (4xx) or could not store (507) is not
// repeated: sending it again changes nothing.
func sendDumpChunk(ctx context.Context, client *api.Client, uploadID string, index int, data []byte) error {
	path := "/api/dbs/uploads/" + url.PathEscape(uploadID) + "/chunks"
	fields := map[string]string{"index": strconv.Itoa(index)}

	for attempt := 1; ; attempt++ {
		err := client.PostMultipart(ctx, path, fields, "chunk", "chunk", data, nil)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return errDumpUploadInterrupted
		}

		status := httpStatusOf(err)
		// The answer to an earlier attempt was lost: the server has the
		// chunk and asks for the one after it.
		if status == 409 && attempt > 1 && chunkAlreadyReceived(err, index) {
			return nil
		}
		if (status >= 400 && status < 500) || status == 507 || attempt == dbUploadAttempts {
			return fmt.Errorf("chunk %d: %w", index, err)
		}

		select {
		case <-ctx.Done():
			return errDumpUploadInterrupted
		case <-time.After(time.Duration(attempt) * dbUploadRetryDelay):
		}
	}
}

func chunkAlreadyReceived(err error, index int) bool {
	m := chunkOutOfOrder.FindStringSubmatch(err.Error())
	if m == nil {
		return false
	}
	next, convErr := strconv.Atoi(m[1])
	return convErr == nil && next == index+1
}

// cancelDumpUpload deletes an upload on the server. Best effort: what is not
// cancelled is swept there after a day.
func cancelDumpUpload(client *api.Client, uploadID string) {
	_ = client.Delete("/api/dbs/uploads/"+url.PathEscape(uploadID), nil, nil)
}

// httpStatusOf is the HTTP status behind an API error, 0 when the request
// never got an answer.
func httpStatusOf(err error) int {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status
	}
	// Answers that are not JSON: "API error (HTTP 413): …", possibly wrapped.
	if _, after, found := strings.Cut(err.Error(), "API error (HTTP "); found {
		if code, _, ok := strings.Cut(after, ")"); ok {
			if status, convErr := strconv.Atoi(code); convErr == nil {
				return status
			}
		}
	}
	return 0
}
