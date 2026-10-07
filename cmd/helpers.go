package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

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
