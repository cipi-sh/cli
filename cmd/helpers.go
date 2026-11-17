package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
)

func mustClient() (*api.Client, error) {
	client, err := api.NewClient()
	if err != nil {
		output.Error("%s", err)
	}
	return client, err
}

func appAPIPath(app, suffix string) string {
	return fmt.Sprintf("/api/apps/%s%s", url.PathEscape(app), suffix)
}

func aliasAPIPath(app, domain string) string {
	return fmt.Sprintf("/api/apps/%s/aliases/%s", url.PathEscape(app), url.PathEscape(domain))
}

func parseKeyValuePairs(pairs []string) (map[string]string, error) {
	out := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key, val, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid key=value pair %q", pair)
		}
		out[key] = val
	}
	return out, nil
}

func readJSONFile(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	return doc, nil
}

func printDataWrapper(data interface{}) {
	if jsonFlag {
		output.PrintJSON(map[string]interface{}{"data": data})
		return
	}
	switch v := data.(type) {
	case map[string]interface{}:
		printMapFields(v)
	case []map[string]interface{}:
		printMapSlice(v)
	case []interface{}:
		rows := make([]map[string]interface{}, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				rows = append(rows, m)
			}
		}
		printMapSlice(rows)
	default:
		output.PrintJSON(map[string]interface{}{"data": data})
	}
}

func printMapFields(m map[string]interface{}) {
	if len(m) == 0 {
		output.Warn("No data returned")
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		output.KeyValue(nil, k, formatFieldValue(m[k]))
	}
	fmt.Println()
}

func printMapSlice(rows []map[string]interface{}) {
	if len(rows) == 0 {
		output.Warn("No items found")
		return
	}
	keys := collectKeys(rows)
	t := output.NewTable(keys...)
	for _, row := range rows {
		vals := make([]string, len(keys))
		for i, k := range keys {
			vals[i] = formatFieldValue(row[k])
		}
		t.Row(vals...)
	}
	t.Flush()
	fmt.Println()
}

func collectKeys(rows []map[string]interface{}) []string {
	seen := make(map[string]struct{})
	for _, row := range rows {
		for k := range row {
			seen[k] = struct{}{}
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func formatFieldValue(v interface{}) string {
	if v == nil {
		return "—"
	}
	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "yes"
		}
		return "no"
	case float64:
		if val == float64(int(val)) {
			return fmt.Sprintf("%.0f", val)
		}
		return fmt.Sprintf("%g", val)
	case map[string]interface{}, []interface{}:
		b, _ := json.Marshal(val)
		return string(b)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// colorStatus colors common service/runtime states (active, running, ok, …).
func colorStatus(s string) string {
	switch strings.ToLower(s) {
	case "active", "running", "ok", "up", "healthy", "installed", "enabled":
		return output.Green.Sprint(s)
	case "inactive", "failed", "dead", "down", "error", "unhealthy", "disabled":
		return output.Red.Sprint(s)
	case "", "—":
		return s
	default:
		return output.Yellow.Sprint(s)
	}
}

// jobResultMap returns the parsed result object of a finished job, if any.
func jobResultMap(job *api.JobStatus) (map[string]interface{}, bool) {
	if job == nil {
		return nil, false
	}
	m, ok := job.Result.(map[string]interface{})
	return m, ok
}

func apiErrorHint(err error, minVersion, feature string) string {
	if msg := api.RouteNotFoundHint(err, minVersion, feature); msg != err.Error() {
		return msg
	}
	return err.Error()
}

// featureErrorHint explains why an endpoint of a recent cipi/api refused: a
// 403 names the ability the token lacks (abilities added by a release are not
// on the tokens created before it), a missing route names the cipi/api release
// that added it. Anything else is passed through — a 503 from an older Cipi
// already says which version it needs.
func featureErrorHint(err error, minAPI, ability string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()

	var apiErr *api.APIError
	isAPIErr := errors.As(err, &apiErr)
	switch {
	case isHTTPStatus(err, 403):
		return fmt.Sprintf("%s — the API token lacks the %s ability (existing tokens do not get new abilities: create one on the server with cipi api token create, or a full-access one with --full-access on Cipi 5.9.0+)", msg, ability)
	case isAPIErr && apiErr.IsRouteNotFound(), !isAPIErr && strings.Contains(msg, "HTTP 404"):
		return fmt.Sprintf("%s — update cipi/api to %s+ on the server (cipi api update)", msg, minAPI)
	}
	return msg
}

// formatSeconds prints a duration in seconds as 45s, 1m35s, 1h2m3s.
func formatSeconds(seconds int64) string {
	if seconds < 0 {
		return "—"
	}
	return (time.Duration(seconds) * time.Second).String()
}

var ansiColors = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// cliOutputLines is what the Cipi CLI printed on the server, without colors
// and blank lines.
func cliOutputLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(ansiColors.ReplaceAllString(text, ""), "\n") {
		line = strings.TrimRight(line, " \t\r")
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// printCLIOutput prints the server-side CLI output of a job or a command.
func printCLIOutput(text string) {
	for _, line := range cliOutputLines(text) {
		fmt.Printf("  %s\n", line)
	}
}
