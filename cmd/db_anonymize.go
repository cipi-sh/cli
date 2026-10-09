package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// The database anonymizer is not part of the Cipi API: it is served by the
// cipi/agent package inside the Laravel app (POST /<prefix>/db). The CLI resolves
// the app URL and CIPI_ANONYMIZER_TOKEN through the Cipi API, then talks to the
// app directly.

const anonymizerDefaultPrefix = "cipi"

var dbAnonymizeCmd = &cobra.Command{
	Use:     "anonymize <app>",
	Aliases: []string{"anonymise", "anon"},
	Short:   "Request an anonymized dump of a Laravel app database (cipi/agent)",
	Long: `Ask the cipi/agent package installed in a Laravel app to build an anonymized
dump of its database. The agent runs the job in the app queue and emails a
signed download link (valid 15 minutes) to the address you pass.

The CLI resolves everything from the server: the app URL (APP_URL, falling
back to https://<primary domain>) and CIPI_ANONYMIZER_TOKEN from the app .env,
then calls POST /<CIPI_ROUTE_PREFIX>/db on the app itself. Requires the
apps-view and apps-env token abilities. Use --url / --token to override.

One-time setup on the app (all via the Cipi API):

  cipi-cli apps artisan myapp cipi:service anonymize --enable
  cipi-cli apps artisan myapp cipi:generate-token anonymize
  cipi-cli apps artisan myapp cipi:init-anonymize
  # then edit /home/<user>/.db/anonymization.json on the server

Download the dump from the link you receive:

  cipi-cli db anonymize download "<signed url>" -o myapp.sql

  cipi-cli db anonymize myapp --email dev@example.com
  cipi-cli prod db anonymize myapp --email dev@example.com`,
	Example: `  cipi-cli db anonymize myapp --email dev@example.com
  cipi-cli prod db anonymize myapp --email dev@example.com
  cipi-cli db anonymize myapp --email dev@example.com --url https://www.example.com
  cipi-cli db anonymize download "<signed url>" -o myapp.sql`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app := args[0]
		email, _ := cmd.Flags().GetString("email")
		baseURL, _ := cmd.Flags().GetString("url")
		token, _ := cmd.Flags().GetString("token")

		email = strings.TrimSpace(email)
		if email == "" && !jsonFlag {
			email = output.ReadInput("Email for the download link")
		}
		if email == "" || !strings.Contains(email, "@") {
			output.Error("A valid --email is required: the agent sends the download link there")
			return fmt.Errorf("email required")
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		target, err := resolveAnonymizerTarget(client, app, baseURL, token)
		if err != nil {
			return err
		}

		if !jsonFlag {
			output.Info("Requesting anonymized dump of '%s' via %s", app, target.endpoint)
		}
		status, header, body, err := agentRequest("POST", target.endpoint, target.token, map[string]string{"email": email})
		if err != nil {
			output.Error("Agent request failed: %s", err)
			return err
		}
		if status >= 300 {
			output.Error("Anonymization request failed: %s", anonymizerErrorHint(app, target.appUser, status, header, body))
			return fmt.Errorf("HTTP %d", status)
		}

		var resp map[string]interface{}
		_ = json.Unmarshal(body, &resp)

		if jsonFlag {
			output.PrintJSON(map[string]interface{}{"data": map[string]interface{}{
				"app":      app,
				"endpoint": target.endpoint,
				"email":    email,
				"status":   str(resp, "status"),
				"message":  str(resp, "message"),
			}})
			return nil
		}

		output.Success("Anonymization job queued for '%s'", app)
		fmt.Println()
		output.KeyValue(nil, "Email", email)
		output.KeyValue(nil, "Endpoint", target.endpoint)
		if msg := str(resp, "message"); msg != "" {
			output.Dim.Printf("  %s\n", msg)
		}
		fmt.Println()
		output.Dim.Println("  The link in the email expires after 15 minutes. Download with:")
		output.Dim.Printf("    cipi-cli db anonymize download \"<signed url>\" -o %s.sql\n", app)
		fmt.Println()
		return nil
	},
}

var dbAnonymizeDownloadCmd = &cobra.Command{
	Use:     "download <signed-url>",
	Aliases: []string{"get", "fetch"},
	Short:   "Download an anonymized dump from the signed link received by email",
	Long: `Download the anonymized SQL dump from the signed link that cipi/agent emails
when the job completes. Links expire after 15 minutes.

Without -o the file name comes from the server (anonymized_database_<date>.sql).
Pass -o - to stream the dump to stdout, e.g. straight into a local database.

  cipi-cli db anonymize download "https://example.com/cipi/db/<token>?expires=...&signature=..."
  cipi-cli db anonymize download "<signed url>" -o myapp.sql
  cipi-cli db anonymize download "<signed url>" -o - | mysql -u root myapp_local`,
	Example: `  cipi-cli db anonymize download "<signed url>" -o myapp.sql
  cipi-cli db anonymize download "<signed url>" -o - | mysql -u root myapp_local`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		link := strings.TrimSpace(args[0])
		out, _ := cmd.Flags().GetString("output")
		force, _ := cmd.Flags().GetBool("force")

		u, err := url.Parse(link)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			output.Error("Pass the full signed URL from the email (https://...)")
			return fmt.Errorf("invalid url")
		}

		req, err := http.NewRequest("GET", link, nil)
		if err != nil {
			output.Error("Invalid URL: %s", err)
			return err
		}
		req.Header.Set("Accept", "application/sql, application/json;q=0.9, */*;q=0.8")
		req.Header.Set("User-Agent", "cipi-cli")

		// No overall timeout: dumps can be large. Only the headers must arrive promptly.
		httpClient := &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 60 * time.Second}}

		if !jsonFlag && out != "-" {
			output.Info("Downloading anonymized dump from %s...", u.Host)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			output.Error("Download failed: %s", err)
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			output.Error("Download failed: %s", anonymizerDownloadHint(resp.StatusCode, body))
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}

		if out == "-" {
			if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
				output.Error("Download interrupted: %s", err)
				return err
			}
			return nil
		}

		if out == "" {
			out = anonymizerDumpFileName(resp.Header.Get("Content-Disposition"))
		}
		flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		if !force {
			flags |= os.O_EXCL
		}
		f, err := os.OpenFile(out, flags, 0o600)
		if err != nil {
			if os.IsExist(err) {
				output.Error("File '%s' already exists — pass --force to overwrite or -o <path>", out)
				return err
			}
			output.Error("Cannot write '%s': %s", out, err)
			return err
		}
		n, copyErr := io.Copy(f, resp.Body)
		closeErr := f.Close()
		if copyErr != nil {
			os.Remove(out)
			output.Error("Download interrupted: %s", copyErr)
			return copyErr
		}
		if closeErr != nil {
			output.Error("Cannot write '%s': %s", out, closeErr)
			return closeErr
		}

		if jsonFlag {
			output.PrintJSON(map[string]interface{}{"data": map[string]interface{}{
				"file":  out,
				"bytes": n,
			}})
			return nil
		}

		output.Success("Saved %s (%s)", out, humanBytes(n))
		fmt.Println()
		return nil
	},
}

type anonymizerTarget struct {
	endpoint string // e.g. https://example.com/cipi/db
	token    string
	appUser  string
}

// resolveAnonymizerTarget reads the app and its .env through the Cipi API and
// builds the agent endpoint + bearer token. baseURL / token override the
// values derived from the server.
func resolveAnonymizerTarget(client *api.Client, app, baseURL, token string) (*anonymizerTarget, error) {
	var appResult struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := client.Get(appAPIPath(app, ""), &appResult); err != nil {
		output.Error("Failed to get app: %s", err)
		return nil, err
	}
	info := appResult.Data
	if custom, _ := info["custom"].(bool); custom {
		output.Error("'%s' is a custom app: the anonymizer needs a Laravel app with cipi/agent installed", app)
		return nil, fmt.Errorf("custom app")
	}
	if node, _ := info["node"].(bool); node {
		output.Error("'%s' is a Node app: the anonymizer needs a Laravel app with cipi/agent installed", app)
		return nil, fmt.Errorf("node app")
	}

	vars, err := appEnvVars(client, app)
	if err != nil {
		output.Error("Failed to read .env of '%s': %s", app, apiErrorHint(err, "5.0.3", "apps-env"))
		return nil, err
	}

	if !envTruthy(vars["CIPI_ANONYMIZER"]) {
		output.Error("The anonymizer is disabled on '%s' (CIPI_ANONYMIZER is not true)", app)
		output.Dim.Printf("  Enable it with: cipi-cli apps artisan %s cipi:service anonymize --enable\n", app)
		return nil, fmt.Errorf("anonymizer disabled")
	}

	if token == "" {
		token = strings.TrimSpace(vars["CIPI_ANONYMIZER_TOKEN"])
	}
	if token == "" {
		output.Error("CIPI_ANONYMIZER_TOKEN is not set on '%s'", app)
		output.Dim.Printf("  Generate it with: cipi-cli apps artisan %s cipi:generate-token anonymize\n", app)
		return nil, fmt.Errorf("anonymizer token missing")
	}

	if baseURL == "" {
		if v := strings.TrimSpace(vars["APP_URL"]); strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			baseURL = v
		}
	}
	if baseURL == "" {
		domain := str(info, "domain")
		if domain == "" || strings.HasPrefix(domain, "*") {
			output.Error("Cannot derive the app URL for '%s' (domain %q, no APP_URL) — pass --url https://...", app, domain)
			return nil, fmt.Errorf("app url unknown")
		}
		baseURL = "https://" + domain
	}
	baseURL = strings.TrimRight(baseURL, "/")

	prefix := strings.Trim(strings.TrimSpace(vars["CIPI_ROUTE_PREFIX"]), "/")
	if prefix == "" {
		prefix = anonymizerDefaultPrefix
	}

	return &anonymizerTarget{
		endpoint: baseURL + "/" + prefix + "/db",
		token:    token,
		appUser:  str(info, "user"),
	}, nil
}

// appEnvVars returns the .env of a Laravel app as a flat string map
// (GET /api/apps/{name}/env → data.vars).
func appEnvVars(client *api.Client, app string) (map[string]string, error) {
	var result struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := client.Get(appAPIPath(app, "/env"), &result); err != nil {
		return nil, err
	}
	raw, ok := result.Data["vars"].(map[string]interface{})
	if !ok {
		raw, _ = result.Data["variables"].(map[string]interface{})
	}
	vars := make(map[string]string, len(raw))
	for k, v := range raw {
		switch val := v.(type) {
		case nil:
			vars[k] = ""
		case string:
			vars[k] = val
		default:
			vars[k] = fmt.Sprintf("%v", val)
		}
	}
	return vars, nil
}

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// agentRequest calls a cipi/agent endpoint on the app itself. Redirects are not
// followed: a 301 would turn the POST into a GET on the wrong host.
func agentRequest(method, endpoint, token string, body interface{}) (int, http.Header, []byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("encoding request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, endpoint, reqBody)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "cipi-cli")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	httpClient := &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, resp.Header, nil, fmt.Errorf("reading response: %w", err)
	}
	return resp.StatusCode, resp.Header, data, nil
}

// agentErrorDetail extracts a readable message from a cipi/agent (or Laravel)
// JSON error body: {"error": "...", "message": "...", "details": {...}}.
func agentErrorDetail(body []byte) string {
	var payload struct {
		Error   string              `json:"error"`
		Message string              `json:"message"`
		Details map[string][]string `json:"details"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	detail := payload.Error
	if payload.Message != "" {
		if detail != "" {
			detail += ": " + payload.Message
		} else {
			detail = payload.Message
		}
	}
	if len(payload.Details) > 0 {
		var parts []string
		for field, msgs := range payload.Details {
			for _, m := range msgs {
				parts = append(parts, fmt.Sprintf("%s: %s", field, m))
			}
		}
		detail = fmt.Sprintf("%s (%s)", detail, strings.Join(parts, "; "))
	}
	return detail
}

func anonymizerErrorHint(app, appUser string, status int, header http.Header, body []byte) string {
	detail := agentErrorDetail(body)
	if appUser == "" {
		appUser = "<user>"
	}

	switch {
	case status >= 300 && status < 400:
		return fmt.Sprintf("the app redirected (HTTP %d) to %s — pass --url with the app's canonical URL", status, header.Get("Location"))
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		if detail == "" {
			detail = fmt.Sprintf("HTTP %d", status)
		}
		return fmt.Sprintf("%s — regenerate it with: cipi-cli apps artisan %s cipi:generate-token anonymize (or pass --token)", detail, app)
	case status == http.StatusNotFound:
		if strings.Contains(strings.ToLower(detail), "configuration file") {
			return fmt.Sprintf("%s — create it with: cipi-cli apps artisan %s cipi:init-anonymize, then edit /home/%s/.db/anonymization.json on the server", detail, app, appUser)
		}
		return fmt.Sprintf("HTTP 404 — the anonymizer endpoint is not exposed by the app: install cipi/agent and enable it with: cipi-cli apps artisan %s cipi:service anonymize --enable", app)
	case status == http.StatusBadRequest:
		return fmt.Sprintf("%s — fix /home/%s/.db/anonymization.json on the server", detail, appUser)
	}

	if detail == "" {
		detail = strings.TrimSpace(string(body))
		if len(detail) > 200 {
			detail = detail[:200] + "…"
		}
	}
	if detail == "" {
		return fmt.Sprintf("HTTP %d", status)
	}
	return fmt.Sprintf("HTTP %d: %s", status, detail)
}

func anonymizerDownloadHint(status int, body []byte) string {
	detail := agentErrorDetail(body)
	switch status {
	case http.StatusGone:
		return "the download link has expired (links last 15 minutes) — request a new dump with: cipi-cli db anonymize <app>"
	case http.StatusNotFound:
		if detail == "" {
			detail = "download token not found or expired"
		}
		return fmt.Sprintf("%s — request a new dump with: cipi-cli db anonymize <app>", detail)
	case http.StatusForbidden:
		if detail == "" {
			detail = "anonymizer not enabled on the app"
		}
		return detail
	}
	if detail == "" {
		return fmt.Sprintf("HTTP %d", status)
	}
	return fmt.Sprintf("HTTP %d: %s", status, detail)
}

// anonymizerDumpFileName picks the local file name from Content-Disposition,
// falling back to the agent's own naming scheme.
func anonymizerDumpFileName(contentDisposition string) string {
	if contentDisposition != "" {
		if _, params, err := mime.ParseMediaType(contentDisposition); err == nil {
			if name := filepath.Base(strings.TrimSpace(params["filename"])); name != "" && name != "." && name != "/" {
				return name
			}
		}
	}
	return "anonymized_database_" + time.Now().Format("2006-01-02_15-04-05") + ".sql"
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func init() {
	dbAnonymizeCmd.Flags().StringP("email", "e", "", "Email that receives the signed download link (prompted if omitted)")
	dbAnonymizeCmd.Flags().String("url", "", "App base URL (default: APP_URL from .env, else https://<primary domain>)")
	dbAnonymizeCmd.Flags().String("token", "", "Anonymizer bearer token (default: CIPI_ANONYMIZER_TOKEN from .env)")

	dbAnonymizeDownloadCmd.Flags().StringP("output", "o", "", "Output file (default: name sent by the server; - for stdout)")
	dbAnonymizeDownloadCmd.Flags().BoolP("force", "f", false, "Overwrite the output file if it exists")

	dbAnonymizeCmd.AddCommand(dbAnonymizeDownloadCmd)
	dbCmd.AddCommand(dbAnonymizeCmd)
}
